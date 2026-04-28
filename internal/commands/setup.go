package commands

import (
	"fmt"
	"os"
	"path"
	"strings"

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
			return getSiteCompletions(toComplete)
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
	rootCmd.AddCommand(setupCmd)
}
