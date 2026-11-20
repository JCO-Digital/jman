package commands

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/JCO-Digital/jman/internal/client"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Post-provisioning setup commands",
	Long:  `A collection of commands to perform post-provisioning tasks on sites or servers.`,
}

var setupMuCmd = &cobra.Command{
	Use:   "mu <target> [path-or-url]",
	Short: "Install or update the bojaco mu-plugin",
	Long: `Installs or updates the bojaco mu-plugin on target sites, as jman-api jobs. Without a path or URL, jman-api installs the latest release from JCO-Digital/bojaco-mu-plugin. A URL must be one of its release assets; a local file is uploaded.

jman-api adds the file's hash to the scan allowlist, so the file scan doesn't raise a security incident for the change.`,
	Args: usageArgs,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return getSiteCompletions()
		}
		return nil, cobra.ShellCompDirectiveDefault
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		source := ""
		if len(args) > 1 {
			source = args[1]
		}
		isURL := strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://")
		if source != "" && !isURL {
			if info, err := os.Stat(source); err != nil || info.IsDir() {
				return fmt.Errorf("local file does not exist: %s", source)
			}
		}

		sites, _, err := promptSites(cmd, args[0], changesSite)
		if err != nil {
			return err
		}
		if len(sites) == 0 {
			verb.Println(verb.Normal, "Operation cancelled.")
			return nil
		}

		return queueMuPlugin(cmd, sites, func(ctx context.Context, c *client.Client, ids []string, out *muPluginResponse) error {
			if source != "" && !isURL {
				return c.PostFile(ctx, "/mu-plugins", source, map[string]string{"plugin": "bojaco", "sites": strings.Join(ids, ",")}, out)
			}
			return c.Post(ctx, "/mu-plugins", map[string]any{"sites": ids, "plugin": "bojaco", "source": source}, out)
		})
	},
}

var setupCompatCmd = &cobra.Command{
	Use:   "compat <slug>",
	Short: "Install or update a compatibility plugin",
	Long: `Installs or updates the compatibility mu-plugin compat-<slug>.php from the JCO-Digital/bojaco-compat repository on all sites that have the plugin installed, as jman-api jobs.

jman-api downloads the file and adds its hash to the scan allowlist, so the file scan doesn't raise a security incident for the change.`,
	Args: usageArgs,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return getPluginCompletions(toComplete, false)
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		slug := args[0]
		if !utils.IsValidSlug(slug) {
			return fmt.Errorf("invalid plugin slug %q: must contain only lowercase alphanumeric characters and hyphens", slug)
		}

		// Find all sites that have the plugin installed.
		m, err := loadMirror(cmd)
		if err != nil {
			return err
		}
		withPlugin := make(map[string]bool)
		for _, p := range m.Plugins() {
			if p.Name == slug && p.Status != "must-use" && p.Status != "dropin" {
				withPlugin[p.SiteID] = true
			}
		}
		var targetSites []models.CliSite
		for _, site := range m.CliSites() {
			if withPlugin[site.ID] {
				targetSites = append(targetSites, site)
			}
		}
		if len(targetSites) == 0 {
			verb.Printf(verb.Normal, "No sites found with plugin %q installed.\n", slug)
			return nil
		}

		// Prompt user for confirmation before deploying
		if flagYes {
			verb.Printf(verb.Normal, "Installing on %d site(s) with plugin %q installed:\n", len(targetSites), slug)
			for _, site := range targetSites {
				verb.Printf(verb.Normal, "  %s %s\n", verb.Blue(site.Name), verb.Gray("("+site.ServerName+")"))
			}
		} else {
			if len(targetSites) == 1 {
				site := targetSites[0]
				verb.Printf(verb.Normal, "Found 1 site with plugin %q installed: %s %s\n", slug, verb.Blue(site.Name), verb.Gray("("+site.ServerName+")"))
				verb.Printf(verb.Normal, "Do you want to install the compatibility plugin on this site? [Y/n]: ")
			} else {
				verb.Printf(verb.Normal, "Found %d sites with plugin %q installed:\n", len(targetSites), slug)
				for i, site := range targetSites {
					verb.Printf(verb.Normal, "[%d] %s %s\n", i+1, verb.Blue(site.Name), verb.Gray("("+site.ServerName+")"))
				}
				verb.Printf(verb.Normal, "Do you want to install the compatibility plugin on these %d sites? [Y/n]: ", len(targetSites))
			}
			response, err := bufio.NewReader(os.Stdin).ReadString('\n')
			if err != nil {
				return err
			}
			response = strings.TrimSpace(strings.ToLower(response))
			if response == "n" || response == "no" {
				verb.Println(verb.Normal, "Operation cancelled.")
				return nil
			}
		}

		return queueMuPlugin(cmd, targetSites, func(ctx context.Context, c *client.Client, ids []string, out *muPluginResponse) error {
			return c.Post(ctx, "/mu-plugins", map[string]any{"sites": ids, "plugin": "compat", "slug": slug}, out)
		})
	},
}

// muPluginResponse is jman-api's answer to a mu-plugin install request.
type muPluginResponse struct {
	File   string             `json:"file"`
	Source string             `json:"source"`
	SHA256 string             `json:"sha256"`
	Jobs   []models.UpdateJob `json:"jobs"`
}

// queueMuPlugin asks jman-api to install a mu-plugin file on sites with
// send, and waits for the jobs.
func queueMuPlugin(cmd *cobra.Command, sites []models.CliSite, send func(context.Context, *client.Client, []string, *muPluginResponse) error) error {
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
	var resp muPluginResponse
	if err := send(ctx, c, ids, &resp); err != nil {
		if fatal := fatalAPIError(err); fatal != nil {
			return fatal
		}
		return err
	}
	verb.Printf(verb.Normal, "Queued %s on %d site(s) from %s\n", verb.Yellow(resp.File), len(resp.Jobs), resp.Source)
	verb.Printf(verb.Verbose, "sha256 %s (added to the scan allowlist)\n", resp.SHA256)

	return waitForJobs(ctx, c, resp.Jobs, sites, func(site models.CliSite, job models.UpdateJob) {
		r, ok := firstResult(job)
		if !ok || r.Status == models.UpdateFailed {
			verb.Printf(verb.Normal, "Failed to install %s on %s: %s\n", verb.Yellow(resp.File), verb.Blue(site.Name), verb.Red(jobError(job, r)))
			return
		}
		verb.Printf(verb.Normal, "%s Installed %s on %s\n", verb.Green("✓"), verb.Yellow(resp.File), verb.Blue(site.Name))
	})
}

func init() {
	setupCmd.AddCommand(setupMuCmd)
	setupCmd.AddCommand(setupCompatCmd)
	rootCmd.AddCommand(setupCmd)
}
