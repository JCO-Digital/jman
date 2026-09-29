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
	"github.com/JCO-Digital/jman/internal/utils"
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

		// Pre-fetch sites and servers (by UUID) for name resolution
		siteMap, serverMap := ignoreTargetNames()

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
					if name, ok := siteMap[id]; ok {
						names = append(names, name)
					} else {
						names = append(names, id)
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
		if entryType == "vuln" {
			entryType = "vulnerability"
		}
		identifier := args[1]
		reason := ""
		if len(args) > 2 {
			reason = args[2]
		}

		target := identifier
		var negatedIDs []string

		// Resolve names to UUIDs for sites and servers
		siteMap, serverMap := ignoreTargetNames()
		if entryType == "site" {
			id, ok := findIDByName(siteMap, identifier)
			if !ok {
				return fmt.Errorf("site %q not found", identifier)
			}
			target = id
		} else if entryType == "server" {
			id, ok := findIDByName(serverMap, identifier)
			if !ok {
				return fmt.Errorf("server %q not found", identifier)
			}
			target = id

			// Handle negated sites
			for _, neg := range ignoreNegate {
				if id, ok := findIDByName(siteMap, neg); ok {
					negatedIDs = append(negatedIDs, id)
				} else {
					fmt.Printf("Warning: negated site %q not found, skipping\n", neg)
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
	ignoreAddCmd.Flags().BoolVar(&ignoreVuln, "vuln", false, "Apply to vulnerability scanning")
	ignoreAddCmd.Flags().StringSliceVarP(&ignoreNegate, "negate", "n", []string{}, "Sites to negate (for server type)")

	ignoreCmd.AddCommand(ignoreListCmd)
	ignoreCmd.AddCommand(ignoreAddCmd)
	ignoreCmd.AddCommand(ignoreRemoveCmd)
	rootCmd.AddCommand(ignoreCmd)
}

// ignoreTargetNames returns site UUID -> domain and server UUID -> name maps
// from the inventory, plus the SpinupWP cache for entities not yet synced.
func ignoreTargetNames() (sites, servers map[string]string) {
	sites = make(map[string]string)
	servers = make(map[string]string)

	if cached, err := cache.GetCachedSites(); err == nil {
		for _, s := range cached {
			sites[utils.SpinupWPSiteUUID(s.ID)] = s.Domain
		}
	}
	if cached, err := cache.GetCachedServers(); err == nil {
		for _, s := range cached {
			servers[utils.SpinupWPServerUUID(s.ID)] = s.Name
		}
	}
	if managed, err := db.ListManagedSites(); err == nil {
		for _, s := range managed {
			sites[s.ID] = s.Domain
		}
	}
	if managed, err := db.ListManagedServers(); err == nil {
		for _, s := range managed {
			servers[s.ID] = s.Name
		}
	}
	return sites, servers
}

// findIDByName looks up the UUID whose name matches (case-insensitively).
func findIDByName(names map[string]string, name string) (string, bool) {
	for id, n := range names {
		if strings.EqualFold(n, name) {
			return id, true
		}
	}
	return "", false
}
