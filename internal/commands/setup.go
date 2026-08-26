package commands

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/search"
	"github.com/JCO-Digital/jman/internal/update"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/JCO-Digital/jman/internal/wpcli"
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
	Long:  `Ensures the bojaco mu-plugin is installed and updated to the latest version. Accepts an optional local file path or URL.`,
	Args:  cobra.MinimumNArgs(1),
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return getSiteCompletions()
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		target := args[0]
		inputSource := ""
		if len(args) > 1 {
			inputSource = args[1]
		}

		sites, err := search.PromptSearch(target)
		if err != nil {
			return err
		}

		if len(sites) == 0 {
			verb.Println(verb.Normal, "Operation cancelled.")
			return nil
		}

		// If no source provided, get the latest URL from GitHub
		if inputSource == "" {
			inputSource, err = getLatestMuPluginURL()
			if err != nil {
				return fmt.Errorf("failed to fetch latest mu-plugin URL: %w", err)
			}
		}

		isURL := strings.HasPrefix(inputSource, "http://") || strings.HasPrefix(inputSource, "https://")
		isLocal := !isURL && inputSource != ""

		if isLocal {
			if _, err := os.Stat(inputSource); os.IsNotExist(err) {
				return fmt.Errorf("local file does not exist: %s", inputSource)
			}
		}

		for _, site := range sites {
			verb.Printf(verb.Normal, "\nSetting up mu-plugin on %s (%s)...\n", verb.Blue(site.Name), verb.Gray(site.ServerName))

			// Use path.Join for remote Linux paths
			muPluginsDir := path.Join(site.Path, "wp-content/mu-plugins")
			destPath := path.Join(muPluginsDir, "bojaco.php")

			// 1. Ensure mu-plugins directory exists
			_, err := wpcli.RunSSH(site.SSH, "mkdir", "-p", muPluginsDir)
			if err != nil {
				verb.PrintErrorf(verb.Normal, "Failed to create mu-plugins directory: %v\n", err)
				continue
			}

			// 2. Install the plugin
			if isURL {
				verb.Printf(verb.Verbose, "Downloading mu-plugin from %s to %s\n", inputSource, destPath)
				res, err := wpcli.RunSSH(site.SSH, "curl", "-sL", "-o", destPath, inputSource)
				if err != nil {
					verb.PrintErrorf(verb.Normal, "Failed to download mu-plugin: %v (stderr: %s)\n", err, res.Error)
					continue
				}
			} else {
				verb.Printf(verb.Verbose, "Uploading mu-plugin from %s to %s\n", inputSource, destPath)
				err := wpcli.UploadFile(site.SSH, inputSource, destPath)
				if err != nil {
					verb.PrintErrorf(verb.Normal, "Failed to upload mu-plugin: %v\n", err)
					continue
				}
			}

			verb.Printf(verb.Normal, "%s Successfully installed/updated mu-plugin at %s\n", verb.Green("✓"), destPath)
		}

		return nil
	},
}

var setupCompatCmd = &cobra.Command{
	Use:   "compat <slug>",
	Short: "Install or update a compatibility plugin",
	Long:  `Checks for a compatibility mu-plugin for the specified slug in the JCO-Digital/bojaco-compat repository and installs/updates it on all sites that have the plugin installed.`,
	Args:  cobra.ExactArgs(1),
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return getPluginCompletions(toComplete, false)
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		slug := args[0]

		// 1. Look in the https://github.com/JCO-Digital/bojaco-compat repo for a file called "compat-[slug].php"
		compatURL := fmt.Sprintf("https://raw.githubusercontent.com/JCO-Digital/bojaco-compat/main/compat-%s.php", slug)

		verb.Printf(verb.Normal, "Checking JCO-Digital/bojaco-compat for compat-%s.php...\n", slug)

		client := &http.Client{
			Timeout: 10 * time.Second,
		}

		resp, err := client.Head(compatURL)
		if err != nil {
			return fmt.Errorf("failed to connect to bojaco-compat repository: %w", err)
		}
		resp.Body.Close()

		// Fallback to master branch if main branch isn't found
		if resp.StatusCode == http.StatusNotFound {
			compatURL = fmt.Sprintf("https://raw.githubusercontent.com/JCO-Digital/bojaco-compat/master/compat-%s.php", slug)
			resp, err = client.Head(compatURL)
			if err != nil {
				return fmt.Errorf("failed to connect to bojaco-compat repository on master branch: %w", err)
			}
			resp.Body.Close()
		}

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("compatibility file compat-%s.php does not exist in the bojaco-compat repository (HTTP %d)", slug, resp.StatusCode)
		}

		verb.Printf(verb.Normal, "%s Compatibility file found!\n", verb.Green("✓"))

		// 2. Find all sites that have the plugin installed
		siteIDs, err := db.GetSitesWithPlugin(slug)
		if err != nil {
			return fmt.Errorf("failed to query sites with plugin %q: %w", slug, err)
		}

		if len(siteIDs) == 0 {
			verb.Printf(verb.Normal, "No sites found in the cache with plugin %q installed.\n", slug)
			return nil
		}

		siteIDMap := make(map[int]bool)
		for _, id := range siteIDs {
			siteIDMap[id] = true
		}

		allSites, err := cache.GetSiteList()
		if err != nil {
			return fmt.Errorf("failed to retrieve site list: %w", err)
		}

		var targetSites []models.CliSite
		for _, site := range allSites {
			if siteIDMap[site.ID] {
				targetSites = append(targetSites, site)
			}
		}

		if len(targetSites) == 0 {
			verb.Printf(verb.Normal, "No matching active WordPress sites found with plugin %q installed.\n", slug)
			return nil
		}

		// Prompt user for confirmation before deploying
		reader := bufio.NewReader(os.Stdin)
		if len(targetSites) == 1 {
			site := targetSites[0]
			verb.Printf(verb.Normal, "Found 1 site with plugin %q installed: %s %s\n", slug, verb.Blue(site.Name), verb.Gray("("+site.ServerName+")"))
			verb.Printf(verb.Normal, "Do you want to install the compatibility plugin on this site? [Y/n]: ")
			response, err := reader.ReadString('\n')
			if err != nil {
				return err
			}
			response = strings.TrimSpace(strings.ToLower(response))
			if response == "n" || response == "no" {
				verb.Println(verb.Normal, "Operation cancelled.")
				return nil
			}
		} else {
			verb.Printf(verb.Normal, "Found %d sites with plugin %q installed:\n", len(targetSites), slug)
			for i, site := range targetSites {
				verb.Printf(verb.Normal, "[%d] %s %s\n", i+1, verb.Blue(site.Name), verb.Gray("("+site.ServerName+")"))
			}
			verb.Printf(verb.Normal, "Do you want to install the compatibility plugin on these %d sites? [Y/n]: ", len(targetSites))
			response, err := reader.ReadString('\n')
			if err != nil {
				return err
			}
			response = strings.TrimSpace(strings.ToLower(response))
			if response == "n" || response == "no" {
				verb.Println(verb.Normal, "Operation cancelled.")
				return nil
			}
		}

		// 3. Install the compat file into /wp-content/mu-plugins of each selected site
		for _, site := range targetSites {
			verb.Printf(verb.Normal, "\nInstalling compatibility plugin on %s (%s)...\n", verb.Blue(site.Name), verb.Gray(site.ServerName))

			muPluginsDir := path.Join(site.Path, "wp-content/mu-plugins")
			destPath := path.Join(muPluginsDir, fmt.Sprintf("compat-%s.php", slug))

			// 1. Ensure mu-plugins directory exists
			_, err := wpcli.RunSSH(site.SSH, "mkdir", "-p", muPluginsDir)
			if err != nil {
				verb.PrintErrorf(verb.Normal, "Failed to create mu-plugins directory: %v\n", err)
				continue
			}

			// 2. Download from GitHub raw URL directly using curl on remote server
			verb.Printf(verb.Verbose, "Downloading compatibility plugin from %s to %s\n", compatURL, destPath)
			res, err := wpcli.RunSSH(site.SSH, "curl", "-sL", "-o", destPath, compatURL)
			if err != nil {
				verb.PrintErrorf(verb.Normal, "Failed to download compatibility plugin: %v (stderr: %s)\n", err, res.Error)
				continue
			}

			verb.Printf(verb.Normal, "%s Successfully installed/updated compatibility plugin at %s\n", verb.Green("✓"), destPath)
		}

		return nil
	},
}

func getLatestMuPluginURL() (string, error) {
	repoURL := "https://api.github.com/repos/JCO-Digital/bojaco-mu-plugin/releases/latest"
	release, err := update.GetLatestRelease(repoURL)
	if err != nil {
		return "", err
	}

	for _, asset := range release.Assets {
		if strings.HasSuffix(asset.Name, ".php") {
			return asset.BrowserDownloadURL, nil
		}
	}

	return "", fmt.Errorf("could not find a .php asset in the latest release")
}

func init() {
	setupCmd.AddCommand(setupMuCmd)
	setupCmd.AddCommand(setupCompatCmd)
	rootCmd.AddCommand(setupCmd)
}
