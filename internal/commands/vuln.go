package commands

import (
	"fmt"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/vuln"
	"github.com/spf13/cobra"
)

var (
	vulnSlack bool
	vulnCVSS  float64
)

var vulnCmd = &cobra.Command{
	Use:   "vuln",
	Short: "Scan for plugin vulnerabilities across all sites.",
	Long:  `Scan for plugin vulnerabilities. Default operation is 'list'.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Default to 'list' if no subcommand is provided.
		// Note: This only runs if no subcommand matches.
		return vulnListCmd.RunE(vulnListCmd, args)
	},
}

var vulnListCmd = &cobra.Command{
	Use:   "list [site-search]",
	Short: "List vulnerabilities (vulnerability-centric)",
	Long:  `Lists vulnerabilities and the sites they affect. If site-search is provided, it switches to a site-centric view for that site and ignores thresholds.`,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return getSiteCompletions()
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := vuln.ScanOptions{
			Mode:          "list",
			Slack:         vulnSlack,
			CVSSThreshold: vulnCVSS,
		}
		if len(args) > 0 {
			opts.SiteSearch = args[0]
		}

		err := vuln.ScanVulnerabilities(opts)
		if err != nil {
			return fmt.Errorf("error scanning vulnerabilities: %w", err)
		}

		return nil
	},
}

var vulnSitesCmd = &cobra.Command{
	Use:   "sites",
	Short: "List vulnerabilities per site (site-centric)",
	Long:  `Lists sites and the vulnerable plugins they have, filtered by configured thresholds.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := vuln.ScanOptions{
			Mode:          "sites",
			Slack:         vulnSlack,
			CVSSThreshold: vulnCVSS,
		}

		err := vuln.ScanVulnerabilities(opts)
		if err != nil {
			return fmt.Errorf("error scanning vulnerabilities: %w", err)
		}

		return nil
	},
}

func init() {
	vulnCmd.PersistentFlags().BoolVarP(&vulnSlack, "slack", "s", false, "Send reports to Slack")
	vulnCmd.PersistentFlags().Float64VarP(&vulnCVSS, "cvss", "c", 0, "Filter by CVSS threshold")

	vulnCmd.AddCommand(vulnListCmd)
	vulnCmd.AddCommand(vulnSitesCmd)
	vulnCmd.AddCommand(vulnIgnoreCmd)
	vulnIgnoreCmd.AddCommand(vulnIgnoreListCmd)
	vulnIgnoreCmd.AddCommand(vulnIgnoreAddCmd)
	vulnIgnoreCmd.AddCommand(vulnIgnoreRemoveCmd)
	rootCmd.AddCommand(vulnCmd)
}

var vulnIgnoreCmd = &cobra.Command{
	Use:   "ignore",
	Short: "Manage the vulnerability ignore list",
	Long:  `Add, remove, or list vulnerability UUIDs that should be suppressed from all scan output.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return vulnIgnoreListCmd.RunE(vulnIgnoreListCmd, args)
	},
}

var vulnIgnoreListCmd = &cobra.Command{
	Use:   "list",
	Short: "List ignored vulnerability UUIDs",
	RunE: func(cmd *cobra.Command, args []string) error {
		ignored, err := db.GetIgnoredVulns()
		if err != nil {
			return fmt.Errorf("error fetching ignore list: %w", err)
		}
		if len(ignored) == 0 {
			fmt.Println("No vulnerabilities are currently ignored.")
			return nil
		}
		for _, v := range ignored {
			if v.Reason != "" {
				fmt.Printf("%s  # %s\n", v.UUID, v.Reason)
			} else {
				fmt.Println(v.UUID)
			}
		}
		return nil
	},
}

var vulnIgnoreAddCmd = &cobra.Command{
	Use:   "add <uuid> [reason]",
	Short: "Add a vulnerability UUID to the ignore list",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		uuid := args[0]
		reason := ""
		if len(args) > 1 {
			reason = args[1]
		}
		if err := db.IgnoreVuln(uuid, reason); err != nil {
			return fmt.Errorf("error adding to ignore list: %w", err)
		}
		fmt.Printf("Ignored vulnerability: %s\n", uuid)
		return nil
	},
}

var vulnIgnoreRemoveCmd = &cobra.Command{
	Use:   "remove <uuid>",
	Short: "Remove a vulnerability UUID from the ignore list",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		uuid := args[0]
		if err := db.UnignoreVuln(uuid); err != nil {
			return fmt.Errorf("error removing from ignore list: %w", err)
		}
		fmt.Printf("Removed vulnerability from ignore list: %s\n", uuid)
		return nil
	},
}
