package commands

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/monitor"
	"github.com/spf13/cobra"
)

var monitorCmd = &cobra.Command{
	Use:           "monitor",
	Short:         "Manage site monitoring",
	Long:          `Manage site monitoring, including the list of ignored domains.`,
	SilenceErrors: true,
	SilenceUsage:  true,
}

var monitorListIgnoredCmd = &cobra.Command{
	Use:   "list",
	Short: "List all currently ignored sites",
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		sites, err := db.GetIgnoredSites()
		if err != nil {
			return err
		}

		if len(sites) == 0 {
			fmt.Println("No sites are currently ignored.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "DOMAIN\tREASON\tIGNORED AT")
		for _, s := range sites {
			fmt.Fprintf(w, "%s\t%s\t%s\n", s.Domain, s.Reason, s.CreatedAt)
		}
		w.Flush()
		return nil
	},
}

var monitorIgnoreCmd = &cobra.Command{
	Use:           "ignore <domain> [reason]",
	Short:         "Add a site to the ignore list",
	Args:          cobra.MinimumNArgs(1),
	SilenceErrors: true,
	SilenceUsage:  true,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return getSiteCompletions()
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		domain := args[0]
		reason := "No reason provided"
		if len(args) > 1 {
			reason = args[1]
		}

		// Check if the site is currently alerting and notify Slack
		monitor.NotifyIfAlertingSiteIgnored(domain, reason)

		if err := db.IgnoreSite(domain, reason); err != nil {
			return err
		}

		fmt.Printf("Site %s is now ignored.\n", domain)
		return nil
	},
}

var monitorUnignoreCmd = &cobra.Command{
	Use:           "unignore <domain>",
	Short:         "Remove a site from the ignore list",
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	SilenceUsage:  true,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			sites, err := db.GetIgnoredSites()
			if err != nil {
				return nil, cobra.ShellCompDirectiveError
			}
			var completions []string
			for _, s := range sites {
				completions = append(completions, fmt.Sprintf("%s\t%s", s.Domain, s.Reason))
			}
			return completions, cobra.ShellCompDirectiveNoFileComp
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		domain := args[0]

		if err := db.UnignoreSite(domain); err != nil {
			return err
		}

		fmt.Printf("Site %s has been removed from the ignore list.\n", domain)
		return nil
	},
}

func init() {
	monitorCmd.AddCommand(monitorListIgnoredCmd)
	monitorCmd.AddCommand(monitorIgnoreCmd)
	monitorCmd.AddCommand(monitorUnignoreCmd)
	rootCmd.AddCommand(monitorCmd)
}
