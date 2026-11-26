package commands

import (
	"fmt"

	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/spf13/cobra"
)

var modsCmd = &cobra.Command{
	Use:   "mods <enable|disable|allow|disallow> <target>",
	Short: "Set DISALLOW_FILE_MODS on target sites.",
	Long:  `Sets DISALLOW_FILE_MODS in the wp-config.php of target sites, as jman-api jobs: "disable" (or "disallow") stops plugin, theme and core changes from wp-admin, "enable" (or "allow") allows them again.`,
	Args:  usageArgs,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return []string{"enable", "disable", "allow", "disallow"}, cobra.ShellCompDirectiveNoFileComp
		}
		if len(args) == 1 {
			return getSiteCompletions()
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		disallow := true
		if len(args) > 1 {
			switch args[0] {
			case "enable", "allow":
				disallow = false
			case "disable", "disallow":
				disallow = true
			default:
				return fmt.Errorf("invalid operation: %s. Use 'enable', 'disable', 'allow' or 'disallow'.", args[0])
			}
		}
		target := args[1]

		sites, _, err := promptSites(cmd, target, changesSite)
		if err != nil {
			return err
		}

		if len(sites) == 0 {
			fmt.Println("Operation cancelled or no sites matched.")
			return nil
		}

		c, err := apiClient()
		if err != nil {
			return err
		}
		ctx, stop := jobContext(cmd)
		defer stop()

		ids := make([]string, len(sites))
		for i, s := range sites {
			ids[i] = s.ID
		}
		var resp struct {
			Jobs []models.UpdateJob `json:"jobs"`
		}
		if err := c.Post(ctx, "/file-mods", map[string]any{"sites": ids, "disallow": disallow}, &resp); err != nil {
			if fatal := fatalAPIError(err); fatal != nil {
				return fatal
			}
			return err
		}

		enableText := "enable"
		if disallow {
			enableText = "disable"
		}
		verb.Printf(verb.Normal, "Queued: set file mods to %s on %d site(s)\n", enableText, len(resp.Jobs))
		return waitForJobs(ctx, c, resp.Jobs, sites, func(site models.CliSite, job models.UpdateJob) {
			if r, ok := firstResult(job); !ok || r.Status == models.UpdateFailed {
				verb.Printf(verb.Normal, "Error setting DISALLOW_FILE_MODS for %s: %s\n", verb.Blue(site.Name), verb.Red(jobError(job, r)))
				return
			}
			verb.Printf(verb.Normal, "%s Set file mods to %s for %s\n", verb.Green("✓"), enableText, verb.Blue(site.Name))
		})
	},
}

func init() {
	rootCmd.AddCommand(modsCmd)
}
