package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/anthropics/anthropic-sdk-go/v2"
)

// maxToolOutput bounds what one tool call returns to the model.
const maxToolOutput = 100_000

// buildParams builds a run's first request. The system prompt and tools
// come first and never change between runs, so they stay in the prompt
// cache; with tools, the top-level cache_control also caches the growing
// conversation between turns.
func buildParams(spec Spec, set settings, prompt Prompt) anthropic.MessageNewParams {
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(set.Model),
		MaxTokens: set.MaxTokens,
		System: []anthropic.TextBlockParam{{
			Text:         spec.System,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt.Text)),
		},
		OutputConfig: anthropic.OutputConfigParam{
			Effort: anthropic.OutputConfigEffort(set.Effort),
			Format: anthropic.JSONOutputFormatParam{Schema: spec.OutputSchema},
		},
	}
	for _, t := range spec.Tools {
		tool := anthropic.ToolParam{
			Name:        t.Name,
			Description: anthropic.String(t.Description),
			Strict:      anthropic.Bool(true),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties:  t.Properties,
				Required:    t.Required,
				ExtraFields: map[string]any{"additionalProperties": false},
			},
		}
		params.Tools = append(params.Tools, anthropic.ToolUnionParam{OfTool: &tool})
	}
	if len(spec.Tools) > 0 {
		params.CacheControl = anthropic.NewCacheControlEphemeralParam()
	}
	return params
}

// requestHash identifies a first request, for reusing results.
func requestHash(version string, params anthropic.MessageNewParams) (string, error) {
	raw, err := json.Marshal(params)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(append([]byte(version+"\x00"), raw...))
	return hex.EncodeToString(sum[:]), nil
}

// execute runs a claimed run to its end and stores the result. A batched
// run ends when its batch does (see collectBatches).
func (g *Gateway) execute(ctx context.Context, run *models.AIRun) {
	g.runRun(ctx, run)
	if run.Status == models.AIRunRunning && run.BatchID != "" {
		ok, err := db.SetAIRunBatch(run)
		if err == nil && ok {
			return
		}
		if err != nil {
			run.Status, run.Error = models.AIRunFailed, fmt.Sprintf("failed to store the batch: %v", err)
		}
		// Canceled meanwhile, or not stored: don't leave the batch running.
		if cerr := g.batcherFor(nil).Cancel(context.WithoutCancel(ctx), run.BatchID); cerr != nil {
			g.logf("AI run %s: failed to cancel batch %s: %v", run.ID, run.BatchID, cerr)
		}
	}
	g.finish(run)
}

// finish stores a run's result and wakes its waiters.
func (g *Gateway) finish(run *models.AIRun) {
	now := g.now()
	run.FinishedAt = &now
	if _, err := db.FinishAIRun(run); err != nil {
		g.logf("AI run %s: failed to store the result: %v", run.ID, err)
	}
	g.notify(run.ID)
}

// runRun fills in run's status, output, usage and cost.
func (g *Gateway) runRun(ctx context.Context, run *models.AIRun) {
	fail := func(format string, args ...any) {
		run.Status = models.AIRunFailed
		run.Error = fmt.Sprintf(format, args...)
	}

	a, ok := g.agents[run.Agent]
	if !ok {
		fail("unknown agent %q", run.Agent)
		return
	}
	spec := a.Spec()
	set := g.settings(spec)
	run.Model = set.Model
	if err := checkModel(set.Model, set.Effort); err != nil {
		fail("%v", err)
		return
	}

	ctx, cancel := context.WithTimeout(ctx, set.Timeout)
	defer cancel()

	prompt, err := a.Prompt(ctx, run.Input)
	if err != nil {
		fail("failed to prepare the agent's data: %v", err)
		return
	}
	if prompt.Skip != nil {
		// Nothing was sent, through a batch or otherwise.
		run.Status = models.AIRunSucceeded
		run.Output = prompt.Skip
		run.Batch = false
		return
	}
	params := buildParams(spec, set, prompt)
	if err := checkZDR(params); err != nil {
		fail("refused to send the request: %v", err)
		return
	}
	if run.RequestHash, err = requestHash(spec.Version, params); err != nil {
		fail("failed to hash the request: %v", err)
		return
	}

	if spec.ReuseFor > 0 {
		prev, err := db.FindReusableAIRun(run.Agent, spec.Version, run.RequestHash, g.now().Add(-spec.ReuseFor))
		if err != nil {
			g.logf("AI run %s: failed to look for a reusable run: %v", run.ID, err)
		} else if prev != nil {
			run.Status = models.AIRunSucceeded
			run.Output = prev.Output
			run.ReusedFrom = prev.ID
			run.StopReason = prev.StopReason
			return
		}
	}

	if run.Batch {
		if hits := findPersonalData(params); len(hits) > 0 {
			g.logf("AI run %s: not batched: the request looks like it holds personal data (%s); sending it through the regular API",
				run.ID, strings.Join(hits, ", "))
			run.Batch = false
		}
	}

	msgr := g.messenger
	if g.fake {
		faker, ok := a.(Faker)
		if !ok {
			fail("agent %s has no fake answer for local development", run.Agent)
			return
		}
		answer, err := faker.FakeAnswer(ctx, prompt)
		if err != nil {
			fail("fake answer: %v", err)
			return
		}
		msgr = fakeMessenger{answer: answer}
	}

	if run.Batch {
		g.submitBatch(ctx, run, msgr, params, prompt)
		return
	}

	stepSeq := 0
	for turn := 1; ; turn++ {
		if turn > set.MaxTurns {
			fail("no answer after %d model calls", set.MaxTurns)
			return
		}
		if err := g.checkBudget(run); err != nil {
			fail("%v", err)
			return
		}

		resp, err := msgr.New(ctx, params)
		if err != nil {
			if ctx.Err() != nil {
				g.endOnContext(ctx, run)
				return
			}
			fail("Claude API: %s", truncate(err.Error(), 500))
			return
		}
		run.Turns = turn
		addUsage(run, set.Model, resp, false)

		if resp.StopReason == anthropic.StopReasonToolUse && len(spec.Tools) > 0 {
			params.Messages = append(params.Messages, resp.ToParam())
			results := g.runTools(ctx, run, spec.Tools, turn, &stepSeq, resp)
			params.Messages = append(params.Messages, anthropic.NewUserMessage(results...))
			if spec.MaxCostUSD > 0 && MicrosToUSD(run.CostMicros) > spec.MaxCostUSD {
				fail("stopped: the run's estimated cost passed its $%.2f limit", spec.MaxCostUSD)
				return
			}
			continue
		}
		applyAnswer(ctx, run, a, prompt, resp)
		return
	}
}

// addUsage adds a response's usage and estimated cost to run. Batched
// requests cost half.
func addUsage(run *models.AIRun, model string, resp *anthropic.Message, batch bool) {
	usage := models.AIUsage{
		InputTokens:              resp.Usage.InputTokens,
		OutputTokens:             resp.Usage.OutputTokens,
		CacheReadInputTokens:     resp.Usage.CacheReadInputTokens,
		CacheCreationInputTokens: resp.Usage.CacheCreationInputTokens,
	}
	run.Usage.Add(usage)
	cost := costMicros(model, usage)
	if batch {
		cost /= 2
	}
	run.CostMicros += cost
	run.StopReason = string(resp.StopReason)
}

// applyAnswer ends run with the model's final response.
func applyAnswer(ctx context.Context, run *models.AIRun, a Agent, prompt Prompt, resp *anthropic.Message) {
	fail := func(format string, args ...any) {
		run.Status = models.AIRunFailed
		run.Error = fmt.Sprintf(format, args...)
	}
	switch resp.StopReason {
	case anthropic.StopReasonRefusal:
		run.Status = models.AIRunRefused
		run.RefusalCategory = string(resp.StopDetails.Category)
		run.Error = "the model declined the request"
		if resp.StopDetails.Explanation != "" {
			run.Error += ": " + resp.StopDetails.Explanation
		}

	case anthropic.StopReasonMaxTokens, anthropic.StopReasonModelContextWindowExceeded:
		fail("the answer was cut off (%s); raise the agent's MaxTokens or give it less data", resp.StopReason)

	case anthropic.StopReasonEndTurn, anthropic.StopReasonStopSequence:
		answer := answerText(resp)
		if !json.Valid([]byte(answer)) {
			fail("the model's answer isn't JSON")
			return
		}
		out, err := a.Finalize(ctx, prompt, json.RawMessage(answer))
		if err != nil {
			fail("invalid answer: %v", err)
			return
		}
		run.Output = out
		run.Status = models.AIRunSucceeded

	default:
		fail("unexpected stop reason %q", resp.StopReason)
	}
}

// endOnContext ends a run whose context ended: canceled by a caller, or
// past its timeout.
func (g *Gateway) endOnContext(ctx context.Context, run *models.AIRun) {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		run.Status = models.AIRunFailed
		run.Error = "the run timed out"
		return
	}
	run.Status = models.AIRunCanceled
}

// runTools runs the tool calls of one response and returns their results,
// all for one user message. A failing or unknown tool is reported to the
// model as an error result rather than failing the run.
func (g *Gateway) runTools(ctx context.Context, run *models.AIRun, tools []Tool, turn int, seq *int,
	resp *anthropic.Message) []anthropic.ContentBlockParamUnion {
	byName := make(map[string]Tool, len(tools))
	for _, t := range tools {
		byName[t.Name] = t
	}
	var results []anthropic.ContentBlockParamUnion
	for _, block := range resp.Content {
		use, ok := block.AsAny().(anthropic.ToolUseBlock)
		if !ok {
			continue
		}
		input := json.RawMessage(use.JSON.Input.Raw())
		if !json.Valid(input) {
			input = json.RawMessage("{}")
		}
		output, isError := "", false
		if t, ok := byName[use.Name]; !ok {
			output, isError = fmt.Sprintf("unknown tool %q", use.Name), true
		} else if out, err := t.Run(ctx, input); err != nil {
			output, isError = "error: "+err.Error(), true
		} else {
			output = truncate(out, maxToolOutput)
		}
		*seq++
		step := models.AIRunStep{Seq: *seq, Turn: turn, Tool: use.Name, Input: input, Output: output,
			IsError: isError, CreatedAt: g.now()}
		if err := db.InsertAIRunStep(run.ID, step); err != nil {
			g.logf("AI run %s: failed to store a tool call: %v", run.ID, err)
		}
		results = append(results, anthropic.NewToolResultBlock(use.ID, output, isError))
	}
	return results
}

// answerText joins the text blocks of a response.
func answerText(resp *anthropic.Message) string {
	var b strings.Builder
	for _, block := range resp.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			b.WriteString(t.Text)
		}
	}
	return strings.TrimSpace(b.String())
}

// checkBudget fails once today's estimated spend (UTC) reaches the daily
// budget. Runs going on at the same time can each pass it, so the budget
// can be overshot by about one run.
func (g *Gateway) checkBudget(run *models.AIRun) error {
	if g.dailyBudgetMicros <= 0 {
		return nil
	}
	spent, err := db.SumAICostSince(startOfDayUTC(g.now()))
	if err != nil {
		return fmt.Errorf("failed to check the daily AI budget: %w", err)
	}
	if spent+run.CostMicros >= g.dailyBudgetMicros {
		return fmt.Errorf("the daily AI budget ($%.2f) is used up; runs resume at 00:00 UTC", MicrosToUSD(g.dailyBudgetMicros))
	}
	return nil
}

func startOfDayUTC(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + fmt.Sprintf("\n[truncated: %d more bytes]", len(s)-n)
}
