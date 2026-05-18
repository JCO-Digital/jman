package commands

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/spf13/cobra"
)

var (
	ignoreMonitor bool
	ignoreVuln    bool
	ignoreNegate  []string
)

var ignoreCmd = &cobra.Command{
	Use:   "ignore",
	Short: "Manage unified ignore list for monitoring and vulnerabilities",
	RunE: func(cmd *cobra.Command, args []string) error {
		return ignoreListCmd.RunE(ignoreListCmd, args)
	},
}

var ignoreListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all ignore entries",
	RunE: func(cmd *cobra.Command, args []string) error {
		entries, err := db.GetAllIgnoreEntries("")
		if err != nil {
			return err
		}

		if len(entries) == 0 {
			fmt.Println("No ignore entries found.")
			return nil
		}

		// Pre-fetch sites and servers for name resolution
		sites, _ := cache.GetCachedSites()
		siteMap := make(map[string]string)
		for _, s := range sites {
			siteMap[strconv.Itoa(s.ID)] = s.Domain
		}

		servers, _ := cache.GetCachedServers()
		serverMap := make(map[string]string)
		for _, s := range servers {
			serverMap[strconv.Itoa(s.ID)] = s.Name
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "ID\tTYPE\tTARGET\tMONITOR\tVULN\tREASON\tNEGATED")
		for _, e := range entries {
			targetDisplay := e.Target
			if e.Type == "site" {
				if name, ok := siteMap[e.Target]; ok {
					targetDisplay = fmt.Sprintf("%s (%s)", name, e.Target)
				}
			} else if e.Type == "server" {
				if name, ok := serverMap[e.Target]; ok {
					targetDisplay = fmt.Sprintf("%s (%s)", name, e.Target)
				}
			}

			negatedDisplay := "-"
			if len(e.NegatedSiteIDs) > 0 {
				var names []string
				for _, id := range e.NegatedSiteIDs {
					if name, ok := siteMap[strconv.Itoa(id)]; ok {
						names = append(names, name)
					} else {
						names = append(names, strconv.Itoa(id))
					}
				}
				negatedDisplay = strings.Join(names, ", ")
			}

			fmt.Fprintf(w, "%d\t%s\t%s\t%v\t%v\t%s\t%s\n", e.ID, e.Type, targetDisplay, e.UseForMonitor, e.UseForVuln, e.Reason, negatedDisplay)
		}
		w.Flush()
		return nil
	},
}

var ignoreAddCmd = &cobra.Command{
	Use:   "add <type> <identifier> [reason]",
	Short: "Add a new ignore entry",
	Long: `Add a new ignore entry.
Types: site, server, plugin, vuln
Identifier:
  site: domain name
  server: server name
  plugin: plugin slug
  vuln: vulnerability UUID`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		entryType := strings.ToLower(args[0])
		identifier := args[1]
		reason := ""
		if len(args) > 2 {
			reason = args[2]
		}

		target := identifier
		var negatedIDs []int

		// Resolve names to IDs for sites and servers
		if entryType == "site" {
			sites, err := cache.GetCachedSites()
			if err != nil {
				return err
			}
			found := false
			for _, s := range sites {
				if strings.ToLower(s.Domain) == strings.ToLower(identifier) {
					target = strconv.Itoa(s.ID)
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("site %q not found in cache", identifier)
			}
		} else if entryType == "server" {
			servers, err := cache.GetCachedServers()
			if err != nil {
				return err
			}
			found := false
			for _, s := range servers {
				if strings.ToLower(s.Name) == strings.ToLower(identifier) {
					target = strconv.Itoa(s.ID)
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("server %q not found in cache", identifier)
			}

			// Handle negated sites
			if len(ignoreNegate) > 0 {
				sites, err := cache.GetCachedSites()
				if err != nil {
					return err
				}
				for _, neg := range ignoreNegate {
					foundNeg := false
					for _, s := range sites {
						if strings.ToLower(s.Domain) == strings.ToLower(neg) {
							negatedIDs = append(negatedIDs, s.ID)
							foundNeg = true
							break
						}
					}
					if !foundNeg {
						fmt.Printf("Warning: negated site %q not found in cache, skipping\n", neg)
					}
				}
			}
		}

		entry := &models.IgnoreEntry{
			Type:           entryType,
			Target:         target,
			Reason:         reason,
			UseForMonitor:  ignoreMonitor,
			UseForVuln:     ignoreVuln,
			NegatedSiteIDs: negatedIDs,
		}

		username := "cli"
		if u, err := user.Current(); err == nil {
			username = u.Username
		}

		if err := db.SaveIgnoreEntry(entry, username); err != nil {
			return err
		}

		fmt.Printf("Ignore entry added (ID: %d)\n", entry.ID)
		return nil
	},
}

var ignoreRemoveCmd = &cobra.Command{
	Use:   "remove <id>",
	Short: "Remove an ignore entry by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid ID: %w", err)
		}

		if err := db.DeleteIgnoreEntry(id); err != nil {
			return err
		}

		fmt.Printf("Ignore entry %d removed.\n", id)
		return nil
	},
}

func init() {
	ignoreAddCmd.Flags().BoolVarP(&ignoreMonitor, "monitor", "m", false, "Apply to uptime monitoring")
	ignoreAddCmd.Flags().BoolVarP(&ignoreVuln, "vuln", "v", false, "Apply to vulnerability scanning")
	ignoreAddCmd.Flags().StringSliceVarP(&ignoreNegate, "negate", "n", []string{}, "Sites to negate (for server type)")

	ignoreCmd.AddCommand(ignoreListCmd)
	ignoreCmd.AddCommand(ignoreAddCmd)
	ignoreCmd.AddCommand(ignoreRemoveCmd)
	rootCmd.AddCommand(ignoreCmd)
}
