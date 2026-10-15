package commands

import (
	"errors"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/JCO-Digital/jman/internal/client"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/mirror"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/JCO-Digital/jman/internal/vulnview"
	"github.com/spf13/cobra"
)

var (
	vulnSlack bool
	vulnCVSS  float64
)

// errVulnSlack replaces the removed --slack flag.
var errVulnSlack = errors.New("jman no longer sends vulnerability reports to Slack; jman-api sends them daily")

var vulnCmd = &cobra.Command{
	Use:   "vuln",
	Short: "Show plugin vulnerabilities across all sites.",
	Long:  `Shows the vulnerabilities jman-api has found in installed plugins and WordPress core. Default operation is 'list'.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if err := rootCmd.PersistentPreRunE(cmd, args); err != nil {
			return err
		}
		if vulnSlack {
			return errVulnSlack
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Default to 'list' if no subcommand is provided.
		// Note: This only runs if no subcommand matches.
		return vulnListCmd.RunE(cmd, args)
	},
}

var vulnListCmd = &cobra.Command{
	Use:   "list [site-search]",
	Short: "List vulnerabilities (vulnerability-centric)",
	Args:  usageArgs,
	Long:  `Lists vulnerabilities and the sites they affect. If site-search is provided, only sites whose domain contains it are shown, and --cvss is ignored.`,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return getSiteCompletions()
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		siteSearch := ""
		if len(args) > 0 {
			siteSearch = args[0]
		}
		reports, m, err := fetchVulnReports(cmd)
		if err != nil {
			return err
		}
		for _, report := range vulnview.FilterReports(reports, siteSearch, vulnCVSS, m.SiteName) {
			fmt.Println(vulnview.FormatReport(report, true, m.SiteName))
		}
		return nil
	},
}

var vulnSitesCmd = &cobra.Command{
	Use:   "sites",
	Short: "List vulnerabilities per site (site-centric)",
	Long:  `Lists sites and their vulnerable plugins: the sites whose highest CVSS reaches --cvss (default cvssThreshold in the config) or whose number of vulnerabilities reaches vulnThreshold.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		reports, m, err := fetchVulnReports(cmd)
		if err != nil {
			return err
		}
		threshold := vulnCVSS
		if threshold <= 0 {
			threshold = config.Cfg.CVSSThreshold
		}
		sites := vulnview.SitesOverThreshold(reports, threshold, config.Cfg.VulnThreshold)
		for _, sv := range sites {
			name := m.SiteName(sv.SiteID)
			if name == "" {
				name = fmt.Sprintf("Site ID: %s", sv.SiteID)
			}
			fmt.Println(vulnview.FormatSiteReport(fmt.Sprintf("%s (%d Vulnerabilities)", name, sv.TotalVulns), sv.Plugins, false))
		}
		verb.Printf(verb.Verbose, "%d sites match criteria\n", len(sites))
		return nil
	},
}

var vulnCoreCmd = &cobra.Command{
	Use:   "core",
	Short: "List WordPress core vulnerabilities",
	Long:  `Lists WordPress core vulnerabilities, grouped by installed core version, and the sites they affect.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := loadMirror(cmd)
		if err != nil {
			return err
		}
		c, err := apiClient()
		if err != nil {
			return err
		}
		var reports []models.CoreVulnReport
		if err := c.Get(cmd.Context(), "/vulns/core", &reports); err != nil {
			return vulnAPIError(err)
		}
		for _, report := range vulnview.FilterCoreReports(reports, vulnCVSS) {
			fmt.Println(vulnview.FormatCoreReport(report, m.SiteName))
		}
		return nil
	},
}

var vulnBackportsCmd = &cobra.Command{
	Use:   "backports",
	Short: "List backport rules",
	Long:  `Lists rules marking vulnerabilities as fixed by backport releases, with the number of sites each rule currently matches. Rules are managed in the web UI.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := apiClient()
		if err != nil {
			return err
		}
		var listings []models.VulnBackportListing
		if err := c.Get(cmd.Context(), "/vuln-backports", &listings); err != nil {
			return vulnAPIError(err)
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

// fetchVulnReports returns jman-api's plugin vulnerability reports, and the
// local copy of its data for site names.
func fetchVulnReports(cmd *cobra.Command) ([]models.VulnReport, *mirror.Mirror, error) {
	m, err := loadMirror(cmd)
	if err != nil {
		return nil, nil, err
	}
	c, err := apiClient()
	if err != nil {
		return nil, nil, err
	}
	var reports []models.VulnReport
	if err := c.Get(cmd.Context(), "/vulns", &reports); err != nil {
		return nil, nil, vulnAPIError(err)
	}
	return reports, m, nil
}

// vulnAPIError explains that vulnerability data needs jman-api, which the
// local copy doesn't cover.
func vulnAPIError(err error) error {
	var apiErr *client.APIError
	if errors.Is(err, client.ErrUnauthorized) || errors.As(err, &apiErr) {
		return err
	}
	return fmt.Errorf("vulnerability data comes from jman-api, which can't be reached: %w", err)
}

func init() {
	vulnCmd.PersistentFlags().BoolVarP(&vulnSlack, "slack", "s", false, "no longer supported: jman-api sends the Slack reports")
	_ = vulnCmd.PersistentFlags().MarkHidden("slack")
	vulnCmd.PersistentFlags().Float64VarP(&vulnCVSS, "cvss", "c", 0, "Filter by CVSS threshold")

	vulnCmd.AddCommand(vulnListCmd)
	vulnCmd.AddCommand(vulnSitesCmd)
	vulnCmd.AddCommand(vulnCoreCmd)
	vulnCmd.AddCommand(vulnBackportsCmd)
	rootCmd.AddCommand(vulnCmd)
}
