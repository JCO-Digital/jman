package commands

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/JCO-Digital/jman/internal/mirror"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/search"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/JCO-Digital/jman/internal/wpcli"
	"github.com/spf13/cobra"
)

var (
	flagUserLive     bool
	flagUserRole     string
	flagUserPassword string
	flagUserReassign string
)

// generatedPasswordLength is the length of the passwords jman user add
// generates: letters and digits, about 143 bits.
const generatedPasswordLength = 24

var userCmd = &cobra.Command{
	Use:   "user",
	Short: "WordPress users on target sites.",
	Long:  `List, find, add or delete the WordPress users of the sites. Subscribers and WooCommerce customers aren't listed, as some sites have thousands; they are only counted.`,
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
		fmt.Fprintln(w, "SITE\tLOGIN\tEMAIL\tNAME\tROLES\tPOSTS")
		for _, u := range matches {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", siteDomain(m, u.SiteID), u.Login, u.Email, u.DisplayName, u.Roles, postCount(u))
		}
		return w.Flush()
	},
}

var addUserCmd = &cobra.Command{
	Use:   "add <target> <login> <email>",
	Short: "Create a user on target sites.",
	Long: `Creates a WordPress user on target sites, as jman-api jobs. The role defaults to administrator.

Without --password, a random password is generated and shown once at the end. It's the same on every site, e.g. for a site's production, staging and development copies. jman-api keeps the password in memory only until the user is created.`,
	Args: usageArgs,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return getSiteCompletions()
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		login, email := args[1], args[2]
		sites, _, err := promptSites(cmd, args[0], changesSite)
		if err != nil {
			return err
		}
		if len(sites) == 0 {
			fmt.Println("Operation cancelled or no sites matched.")
			return nil
		}

		password, generated := flagUserPassword, false
		if password == "" {
			if password, err = generatePassword(generatedPasswordLength); err != nil {
				return err
			}
			generated = true
		}

		c, err := apiClient()
		if err != nil {
			return err
		}
		ctx, stop := jobContext(cmd)
		defer stop()

		var queued []models.UpdateJob
		for _, site := range sites {
			verb.Printf(verb.Verbose, "Queueing user %s on %s...\n", login, site.Name)
			var job models.UpdateJob
			body := map[string]any{"login": login, "email": email, "role": flagUserRole, "password": password}
			if err := c.Post(ctx, "/sites/"+site.ID+"/users", body, &job); err != nil {
				if fatal := fatalAPIError(err); fatal != nil {
					return fatal
				}
				verb.PrintErrorf(verb.Normal, "Error creating %s on %s: %v\n", login, site.Name, err)
				continue
			}
			queued = append(queued, job)
		}

		created := 0
		err = waitForJobs(ctx, c, queued, sites, func(site models.CliSite, job models.UpdateJob) {
			r, ok := firstResult(job)
			if !ok || r.Status == models.UpdateFailed || job.Status != models.UpdateJobDone {
				verb.PrintErrorf(verb.Normal, "Error creating %s on %s: %s\n", login, site.Name, jobError(job, r))
				return
			}
			verb.Printf(verb.Normal, "Created %s (%s) on %s.\n", verb.Yellow(login), flagUserRole, verb.Blue(site.Name))
			created++
		})
		if created > 0 && generated {
			same := ""
			if created > 1 {
				same = fmt.Sprintf(" (the same on all %d sites)", created)
			}
			verb.Printf(verb.Quiet, "Password for %s%s: %s\n", login, same, password)
		}
		return err
	},
}

var deleteUserCmd = &cobra.Command{
	Use:   "delete <query>",
	Short: "Delete users matching a login or email.",
	Long: `Finds the users whose login or email contains the query (ignoring case) on every site, asks which to delete and deletes them as jman-api jobs. If a login or email equals the query, only those users are offered.

Deleting a user deletes all their posts. A user who has posts is only deleted with --reassign, which names the user who gets the posts on each site, by exact login or email. Users on multisite sites aren't offered.`,
	Args: usageArgs,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := loadMirror(cmd)
		if err != nil {
			return err
		}
		reachable := map[string]models.CliSite{}
		for _, s := range m.CliSites() {
			reachable[s.ID] = s
		}

		var candidates []models.SiteUser
		skipped := 0
		for _, u := range matchUsers(m, args[0]) {
			site, ok := m.Site(u.SiteID)
			if _, canWPCLI := reachable[u.SiteID]; !ok || !canWPCLI || site.Multisite {
				skipped++
				continue
			}
			candidates = append(candidates, u)
		}
		if skipped > 0 {
			verb.Printf(verb.Normal, "Leaving out %d matching user(s) on multisite sites or sites WP-CLI can't reach.\n", skipped)
		}
		if len(candidates) == 0 {
			verb.Printf(verb.Normal, "No users to delete matching '%s'.\n", args[0])
			return nil
		}

		label := func(u models.SiteUser) string {
			return fmt.Sprintf("%s <%s> @ %s %s", verb.Yellow(u.Login), u.Email, verb.Blue(siteDomain(m, u.SiteID)), verb.Gray("("+postCount(u)+" posts)"))
		}
		selected := candidates
		if flagYes {
			verb.PrintErrorf(verb.Normal, "Deleting %d user(s):\n", len(selected))
			for _, u := range selected {
				verb.PrintErrorf(verb.Normal, "  %s\n", label(u))
			}
		} else if selected, err = search.PromptSelect(candidates, "user", label, true); err != nil {
			return err
		}
		if len(selected) == 0 {
			fmt.Println("Operation cancelled.")
			return nil
		}

		c, err := apiClient()
		if err != nil {
			return err
		}
		ctx, stop := jobContext(cmd)
		defer stop()

		var queued []models.UpdateJob
		logins := map[int64]string{} // by job ID
		for _, u := range selected {
			path := fmt.Sprintf("/sites/%s/users/%d", u.SiteID, u.ID)
			if flagUserReassign != "" {
				path += "?reassign=" + url.QueryEscape(flagUserReassign)
			}
			var job models.UpdateJob
			if err := c.Delete(ctx, path, &job); err != nil {
				if fatal := fatalAPIError(err); fatal != nil {
					return fatal
				}
				verb.PrintErrorf(verb.Normal, "Error deleting %s on %s: %v\n", u.Login, siteDomain(m, u.SiteID), err)
				continue
			}
			logins[job.ID] = u.Login
			queued = append(queued, job)
		}

		sites := make([]models.CliSite, 0, len(reachable))
		for _, s := range reachable {
			sites = append(sites, s)
		}
		deleted := 0
		err = waitForJobs(ctx, c, queued, sites, func(site models.CliSite, job models.UpdateJob) {
			login := logins[job.ID]
			r, ok := firstResult(job)
			if !ok || r.Status == models.UpdateFailed || job.Status != models.UpdateJobDone {
				verb.PrintErrorf(verb.Normal, "Error deleting %s on %s: %s\n", login, site.Name, jobError(job, r))
				return
			}
			note := ""
			if r.Note != "" {
				note = " " + r.Note + "."
			}
			verb.Printf(verb.Normal, "Deleted %s on %s.%s\n", verb.Yellow(login), verb.Blue(site.Name), note)
			deleted++
		})
		if deleted > 1 {
			verb.Printf(verb.Normal, "Deleted %d users.\n", deleted)
		}
		return err
	},
}

func init() {
	listUserCmd.Flags().BoolVar(&flagUserLive, "live", false, "read the users from the sites over SSH")
	addUserCmd.Flags().StringVar(&flagUserRole, "role", "administrator", "the new user's role")
	addUserCmd.Flags().StringVar(&flagUserPassword, "password", "", "the new user's password (generated if not given)")
	deleteUserCmd.Flags().StringVar(&flagUserReassign, "reassign", "", "login or email of the user who gets the deleted users' posts")
	userCmd.AddCommand(listUserCmd)
	userCmd.AddCommand(findUserCmd)
	userCmd.AddCommand(addUserCmd)
	userCmd.AddCommand(deleteUserCmd)
	rootCmd.AddCommand(userCmd)
}

// generatePassword returns a random password of n letters and digits.
func generatePassword(n int) (string, error) {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		k, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		if err != nil {
			return "", fmt.Errorf("failed to generate a password: %w", err)
		}
		b[i] = chars[k.Int64()]
	}
	return string(b), nil
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
	sortUsersBySite(m, out)
	return out
}

// matchUsers returns the users whose login or email contains query,
// ignoring case, sorted by site domain and login. If any login or email
// equals query, only those users are returned, as with site targets.
func matchUsers(m *mirror.Mirror, query string) []models.SiteUser {
	q := strings.ToLower(strings.TrimSpace(query))
	var exact, partial []models.SiteUser
	for _, u := range m.Users() {
		login, email := strings.ToLower(u.Login), strings.ToLower(u.Email)
		if login == q || email == q {
			exact = append(exact, u)
		}
		if strings.Contains(login, q) || strings.Contains(email, q) {
			partial = append(partial, u)
		}
	}
	out := partial
	if len(exact) > 0 {
		out = exact
	}
	sortUsersBySite(m, out)
	return out
}

// sortUsersBySite sorts users by site domain and login.
func sortUsersBySite(m *mirror.Mirror, users []models.SiteUser) {
	domains := map[string]string{}
	for _, u := range users {
		domains[u.SiteID] = siteDomain(m, u.SiteID)
	}
	slices.SortFunc(users, func(a, b models.SiteUser) int {
		if c := strings.Compare(domains[a.SiteID], domains[b.SiteID]); c != 0 {
			return c
		}
		return strings.Compare(strings.ToLower(a.Login), strings.ToLower(b.Login))
	})
}

// postCount formats a user's post count, "?" if it's unknown.
func postCount(u models.SiteUser) string {
	if u.PostCount == nil {
		return "?"
	}
	return strconv.Itoa(*u.PostCount)
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
		fmt.Fprintln(w, "  ID\tLOGIN\tEMAIL\tNAME\tROLES\tPOSTS")
		for _, u := range users.Users {
			fmt.Fprintf(w, "  %d\t%s\t%s\t%s\t%s\t%s\n", u.ID, u.Login, u.Email, u.DisplayName, u.Roles, postCount(u))
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
