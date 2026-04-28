package commands

import (
	"fmt"

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
			return getSiteCompletions(toComplete)
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
	rootCmd.AddCommand(vulnCmd)
}
