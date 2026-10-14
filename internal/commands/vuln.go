package commands

import (
	"fmt"
	"os"
	"text/tabwriter"

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

var vulnCoreCmd = &cobra.Command{
	Use:   "core",
	Short: "List WordPress core vulnerabilities",
	Long:  `Lists WordPress core vulnerabilities, grouped by installed core version, and the sites they affect.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := vuln.ScanOptions{
			Slack:         vulnSlack,
			CVSSThreshold: vulnCVSS,
		}

		err := vuln.ScanCoreVulnerabilities(opts)
		if err != nil {
			return fmt.Errorf("error scanning core vulnerabilities: %w", err)
		}

		return nil
	},
}

var vulnBackportsCmd = &cobra.Command{
	Use:   "backports",
	Short: "List backport rules",
	Long:  `Lists rules marking vulnerabilities as fixed by backport releases, with the number of sites each rule currently matches. Rules are managed in the web UI.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		listings, err := vuln.ListVulnBackports()
		if err != nil {
			return err
		}

		if len(listings) == 0 {
			fmt.Println("No backport rules found.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "ID\tTYPE\tPLUGIN\tUUID\tBRANCH\tMIN\tSITES\tREASON\tCREATED_BY")
		for _, l := range listings {
			slug := l.Slug
			if slug == "" {
				slug = "-"
			}
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\n", l.ID, l.Type, slug, l.VulnUUID, l.Branch, l.MinVersion, l.MatchedSites, l.Reason, l.CreatedBy)
		}
		w.Flush()
		return nil
	},
}

func init() {
	vulnCmd.PersistentFlags().BoolVarP(&vulnSlack, "slack", "s", false, "Send reports to Slack")
	vulnCmd.PersistentFlags().Float64VarP(&vulnCVSS, "cvss", "c", 0, "Filter by CVSS threshold")

	vulnCmd.AddCommand(vulnListCmd)
	vulnCmd.AddCommand(vulnSitesCmd)
	vulnCmd.AddCommand(vulnCoreCmd)
	vulnCmd.AddCommand(vulnBackportsCmd)
	rootCmd.AddCommand(vulnCmd)
}
