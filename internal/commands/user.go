package commands

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/JCO-Digital/jman/internal/mirror"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/JCO-Digital/jman/internal/wpcli"
	"github.com/spf13/cobra"
)

var flagUserLive bool

var userCmd = &cobra.Command{
	Use:   "user",
	Short: "WordPress users on target sites.",
	Long: `List or find the WordPress users of the sites. Subscribers and WooCommerce customers aren't listed, as some sites have thousands; they are only counted.

To add an administrator, see 'jman admin'.`,
}

var listUserCmd = &cobra.Command{
	Use:   "list <target>",
	Short: "List the users of target sites.",
	Long: `Lists the WordPress users of target sites from jman-api's data, without subscribers and WooCommerce customers, which are counted.

With --live, the users are read from the sites over SSH instead.`,
	Args: usageArgs,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return getSiteCompletions()
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		sites, m, err := promptSites(cmd, args[0], readOnly)
		if err != nil {
			return err
		}
		if len(sites) == 0 {
			fmt.Println("Operation cancelled or no sites matched.")
			return nil
		}

		for _, site := range sites {
			users, found := mirrorSiteUsers(m, site.ID)
			if flagUserLive {
				verb.Printf(verb.Verbose, "Listing users on %s (%s)...\n", verb.Blue(site.Name), site.ServerName)
				if users, err = wpcli.GetSiteUsers(site); err != nil {
					verb.Printf(verb.Normal, "Error listing users on %s:\n%v\n", verb.Blue(site.Name), verb.Red(err))
					continue
				}
				found = true
			}
			if !found {
				verb.Printf(verb.Normal, "No users known for %s yet.\n", verb.Blue(site.Name))
				continue
			}
			printSiteUsers(site, users)
		}
		return nil
	},
}

var findUserCmd = &cobra.Command{
	Use:   "find <query>",
	Short: "Find a user across all sites.",
	Long: `Finds WordPress users whose login, email or display name contains the query (ignoring case), on every site in jman-api's data.

Subscribers and WooCommerce customers aren't searched.`,
	Args: usageArgs,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := loadMirror(cmd)
		if err != nil {
			return err
		}
		matches := findUsers(m, args[0])
		if len(matches) == 0 {
			verb.Printf(verb.Normal, "No users found matching '%s'.\n", args[0])
			return nil
		}

		verb.Printf(verb.Normal, "Found %d users matching '%s':\n", len(matches), args[0])
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "SITE\tLOGIN\tEMAIL\tNAME\tROLES")
		for _, u := range matches {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", siteDomain(m, u.SiteID), u.Login, u.Email, u.DisplayName, u.Roles)
		}
		return w.Flush()
	},
}

func init() {
	listUserCmd.Flags().BoolVar(&flagUserLive, "live", false, "read the users from the sites over SSH")
	userCmd.AddCommand(listUserCmd)
	userCmd.AddCommand(findUserCmd)
	rootCmd.AddCommand(userCmd)
}

// mirrorSiteUsers returns a site's users from the mirror, and false if its
// users were never read.
func mirrorSiteUsers(m *mirror.Mirror, siteID string) (models.SiteUsers, bool) {
	summary, found := m.SiteUserSummary(siteID)
	if !found {
		return models.SiteUsers{}, false
	}
	return models.SiteUsers{Users: m.SiteUsers(siteID), ExcludedCounts: summary.ExcludedCounts, Truncated: summary.Truncated}, true
}

// findUsers returns the users whose login, email or display name contains
// query, ignoring case, sorted by site domain and login.
func findUsers(m *mirror.Mirror, query string) []models.SiteUser {
	q := strings.ToLower(strings.TrimSpace(query))
	var out []models.SiteUser
	for _, u := range m.Users() {
		if strings.Contains(strings.ToLower(u.Login), q) ||
			strings.Contains(strings.ToLower(u.Email), q) ||
			strings.Contains(strings.ToLower(u.DisplayName), q) {
			out = append(out, u)
		}
	}
	domains := map[string]string{}
	for _, u := range out {
		domains[u.SiteID] = siteDomain(m, u.SiteID)
	}
	slices.SortFunc(out, func(a, b models.SiteUser) int {
		if c := strings.Compare(domains[a.SiteID], domains[b.SiteID]); c != 0 {
			return c
		}
		return strings.Compare(strings.ToLower(a.Login), strings.ToLower(b.Login))
	})
	return out
}

// siteDomain returns a site's domain, or its ID if the site is unknown.
func siteDomain(m *mirror.Mirror, siteID string) string {
	if s, ok := m.Site(siteID); ok {
		return s.Domain
	}
	return siteID
}

func printSiteUsers(site models.CliSite, users models.SiteUsers) {
	verb.Printf(verb.Normal, "Users on %s:\n", verb.Blue(site.Name))
	if len(users.Users) > 0 {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "  ID\tLOGIN\tEMAIL\tNAME\tROLES")
		for _, u := range users.Users {
			fmt.Fprintf(w, "  %d\t%s\t%s\t%s\t%s\n", u.ID, u.Login, u.Email, u.DisplayName, u.Roles)
		}
		w.Flush()
	} else {
		fmt.Println("  No users to list.")
	}
	if users.Truncated {
		fmt.Printf("  Only the first %d users are listed.\n", models.MaxSiteUsers)
	}
	var counts []string
	for _, role := range wpcli.ExcludedUserRoles {
		counts = append(counts, fmt.Sprintf("%d %s", users.ExcludedCounts[role], role))
	}
	fmt.Println(verb.Gray("  Not listed: " + strings.Join(counts, ", ")))
}
