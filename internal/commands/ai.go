package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/spf13/cobra"
)

var (
	aiInput   string
	aiNoWait  bool
	aiBatch   bool
	aiTimeout time.Duration
	aiJSON    bool
	aiAgent   string
	aiLimit   int
)

// vulnRankAgent is the vuln-rank agent's name, for its table output.
const vulnRankAgent = "vuln-rank"

var aiCmd = &cobra.Command{
	Use:   "ai",
	Short: "Run jman-api's AI agents",
	Long: `Runs jman-api's AI agents and shows their results. Agents are advisory:
they read jman's data and answer, and never change anything.

Running an agent needs the execute level. See docs/ai-gateway.md.`,
	Args: usageArgs,
}

var aiStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show whether the AI gateway is on, and today's spend",
	Args:  usageArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := apiClient()
		if err != nil {
			return err
		}
		var s models.AIStatus
		if err := c.Get(cmd.Context(), "/ai/status", &s); err != nil {
			return err
		}
		switch {
		case !s.Enabled:
			verb.Printf(verb.Normal, "%s AI gateway disabled: %s\n", verb.Yellow("!"), s.Reason)
		case s.Fake:
			verb.Printf(verb.Normal, "%s AI gateway on, with fake answers (local development)\n", verb.Green("✓"))
		default:
			verb.Printf(verb.Normal, "%s AI gateway on\n", verb.Green("✓"))
		}
		verb.Printf(verb.Normal, "Spent today (UTC, estimated): $%.2f of $%.2f\n", s.SpentTodayUSD, s.DailyBudgetUSD)
		return nil
	},
}

var aiAgentsCmd = &cobra.Command{
	Use:   "agents",
	Short: "List the AI agents",
	Args:  usageArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := apiClient()
		if err != nil {
			return err
		}
		var agents []models.AIAgentInfo
		if err := c.Get(cmd.Context(), "/ai/agents", &agents); err != nil {
			return err
		}
		if aiJSON {
			return printJSON(agents)
		}
		for _, a := range agents {
			schedule := "on request"
			if a.ScheduleHours > 0 {
				schedule = fmt.Sprintf("every %dh", a.ScheduleHours)
			}
			if a.Batch {
				schedule += ", batchable"
			}
			verb.Printf(verb.Normal, "%s (v%s, %s, effort %s, %s)\n  %s\n", verb.Bold(a.Name), a.Version, a.Model,
				a.Effort, schedule, a.Description)
		}
		return nil
	},
}

var aiRunCmd = &cobra.Command{
	Use:   "run <agent>",
	Short: "Run an AI agent and show its result",
	Long: `Runs an agent and waits for its result. --input is the run input as JSON,
for example for vuln-rank: --input '{"keys":["plugin:akismet"]}'.

--batch sends the run through the Message Batches API, for agents that allow
it: half price, but the result can take up to 24 hours, so it doesn't wait.`,
	Args: usageArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := apiClient()
		if err != nil {
			return err
		}
		req := models.AIRunRequest{}
		if aiInput != "" {
			if !json.Valid([]byte(aiInput)) {
				return fmt.Errorf("--input isn't valid JSON")
			}
			req.Input = json.RawMessage(aiInput)
		}
		req.Batch = aiBatch
		noWait := aiNoWait || aiBatch
		path := "/ai/agents/" + url.PathEscape(args[0]) + "/runs"
		if !noWait {
			path += "?wait=60"
		}
		var run models.AIRun
		if err := c.Post(cmd.Context(), path, req, &run); err != nil {
			return err
		}
		if noWait {
			verb.Printf(verb.Normal, "Queued run %s. See it with: jman ai show %s\n", run.ID, run.ID)
			return nil
		}

		ctx, cancel := context.WithTimeout(cmd.Context(), aiTimeout)
		defer cancel()
		for !models.AIRunFinished(run.Status) {
			verb.PrintErrorf(verb.Verbose, "Run %s is %s...\n", run.ID, run.Status)
			select {
			case <-ctx.Done():
				return fmt.Errorf("run %s is still %s; see it later with jman ai show %s", run.ID, run.Status, run.ID)
			case <-time.After(3 * time.Second):
			}
			if err := c.Get(ctx, "/ai/runs/"+url.PathEscape(run.ID), &run); err != nil {
				return err
			}
		}
		return printAIRun(run)
	},
}

var aiRunsCmd = &cobra.Command{
	Use:   "runs",
	Short: "List recent AI runs",
	Args:  usageArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := apiClient()
		if err != nil {
			return err
		}
		q := url.Values{"limit": {fmt.Sprint(aiLimit)}}
		if aiAgent != "" {
			q.Set("agent", aiAgent)
		}
		var runs []models.AIRun
		if err := c.Get(cmd.Context(), "/ai/runs?"+q.Encode(), &runs); err != nil {
			return err
		}
		if aiJSON {
			return printJSON(runs)
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tAGENT\tSTATUS\tCREATED\tBY\tCOST")
		for _, r := range runs {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t$%.3f\n", r.ID, r.Agent, r.Status,
				r.CreatedAt.Local().Format("2006-01-02 15:04"), r.RequestedBy, float64(r.CostMicros)/1e6)
		}
		return w.Flush()
	},
}

var aiShowCmd = &cobra.Command{
	Use:   "show <run-id>",
	Short: "Show an AI run and its result",
	Args:  usageArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := apiClient()
		if err != nil {
			return err
		}
		var run models.AIRun
		if err := c.Get(cmd.Context(), "/ai/runs/"+url.PathEscape(args[0]), &run); err != nil {
			return err
		}
		return printAIRun(run)
	},
}

var aiCancelCmd = &cobra.Command{
	Use:   "cancel <run-id>",
	Short: "Cancel a queued or running AI run",
	Args:  usageArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := apiClient()
		if err != nil {
			return err
		}
		var run models.AIRun
		if err := c.Post(cmd.Context(), "/ai/runs/"+url.PathEscape(args[0])+"/cancel", nil, &run); err != nil {
			return err
		}
		verb.Printf(verb.Normal, "%s Run %s canceled.\n", verb.Green("✓"), run.ID)
		return nil
	},
}

func printAIRun(run models.AIRun) error {
	if aiJSON {
		return printJSON(run)
	}
	mode := ""
	if run.Batch {
		mode = " (batch)"
	}
	verb.Printf(verb.Normal, "Run %s: %s %s on %s%s\n", run.ID, verb.Bold(run.Agent), run.Status, run.Model, mode)
	switch {
	case run.ReusedFrom != "":
		verb.Printf(verb.Normal, "Reused the result of run %s (same request).\n", run.ReusedFrom)
	case run.Turns > 0:
		verb.Printf(verb.Normal, "%d model calls, %d input + %d cached + %d output tokens, about $%.3f\n", run.Turns,
			run.Usage.InputTokens+run.Usage.CacheCreationInputTokens, run.Usage.CacheReadInputTokens,
			run.Usage.OutputTokens, float64(run.CostMicros)/1e6)
	}
	if run.Error != "" {
		msg := run.Error
		if run.RefusalCategory != "" {
			msg += " (" + run.RefusalCategory + ")"
		}
		verb.Printf(verb.Normal, "%s %s\n", verb.Red("✗"), msg)
	}
	if len(run.Output) == 0 {
		if run.Status != models.AIRunSucceeded {
			return fmt.Errorf("run %s", run.Status)
		}
		return nil
	}
	fmt.Println()
	if run.Agent == vulnRankAgent {
		var r models.AIVulnRanking
		if err := json.Unmarshal(run.Output, &r); err == nil {
			return printVulnRanking(r)
		}
	}
	return printJSON(run.Output)
}

func printVulnRanking(r models.AIVulnRanking) error {
	if len(r.Rankings) == 0 && len(r.Unranked) == 0 {
		verb.Printf(verb.Normal, "No open vulnerability exposures to rank.\n")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "#\tPRIO\tSCORE\tGROUP\tACTION\tCONF\tRATIONALE")
	for _, x := range r.Rankings {
		prio := x.Priority
		switch prio {
		case models.AIPriorityP1:
			prio = verb.Red(prio)
		case models.AIPriorityP2:
			prio = verb.Yellow(prio)
		}
		fmt.Fprintf(w, "%d\t%s\t%d\t%s\t%s\t%s\t%s\n", x.Rank, prio, x.Score, x.Key,
			strings.ReplaceAll(x.Action, "_", " "), x.Confidence, x.Rationale)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if len(r.Unranked) > 0 {
		verb.Printf(verb.Normal, "\nNot ranked: %s\n", strings.Join(r.Unranked, ", "))
	}
	return nil
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func init() {
	aiCmd.PersistentFlags().BoolVar(&aiJSON, "json", false, "print JSON")
	aiRunCmd.Flags().StringVar(&aiInput, "input", "", "run input as JSON")
	aiRunCmd.Flags().BoolVar(&aiNoWait, "no-wait", false, "queue the run and return at once")
	aiRunCmd.Flags().BoolVar(&aiBatch, "batch", false, "run through the Message Batches API (half price, up to 24 hours)")
	aiRunCmd.Flags().DurationVar(&aiTimeout, "timeout", 15*time.Minute, "how long to wait for the result")
	aiRunsCmd.Flags().StringVar(&aiAgent, "agent", "", "only this agent's runs")
	aiRunsCmd.Flags().IntVar(&aiLimit, "limit", 20, "how many runs to list")
	aiCmd.AddCommand(aiStatusCmd, aiAgentsCmd, aiRunCmd, aiRunsCmd, aiShowCmd, aiCancelCmd)
	rootCmd.AddCommand(aiCmd)
}
