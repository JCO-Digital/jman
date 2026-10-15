package commands

import (
	"fmt"
	"strings"

	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/spf13/cobra"
)

var (
	coreMajor      bool
	coreAllowMajor bool
)

var coreCmd = &cobra.Command{
	Use:   "core <check|update|version> <target>",
	Short: "Manage WordPress core.",
	Long: `Check for core updates, update core, or show the installed core version on target sites.

'check' and 'update' go through jman-api and need the execute level; updates run as jman-api jobs that jman waits for. 'version' shows the version jman-api last recorded.

Updates go to the latest minor release unless --major is given. A major update on an update-locked site also needs --allow-major.`,
	Args: usageArgs,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return []string{"check", "update", "version"}, cobra.ShellCompDirectiveNoFileComp
		}
		if len(args) == 1 {
			return getSiteCompletions()
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: coreCommand,
}

func init() {
	coreCmd.Flags().BoolVar(&coreMajor, "major", false, "Allow major version updates for WordPress core")
	coreCmd.Flags().BoolVar(&coreAllowMajor, "allow-major", false, "Confirm a major update on update-locked sites")
	rootCmd.AddCommand(coreCmd)
}

func coreCommand(cmd *cobra.Command, args []string) error {
	operation := args[0]
	target := args[1]
	if operation != "check" && operation != "update" && operation != "version" {
		return fmt.Errorf("invalid operation: %s. Use 'check', 'update', or 'version'", operation)
	}

	access := readOnly
	if operation == "update" {
		access = changesSite
	}
	sites, m, err := promptSites(cmd, target, access)
	if err != nil {
		return err
	}
	if len(sites) == 0 {
		verb.Println(verb.Normal, "Operation cancelled or no sites matched.")
		return nil
	}

	if operation == "version" {
		for _, site := range sites {
			s, _ := m.Site(site.ID)
			if s.WPCore == nil || s.WPCore.Version == "" {
				verb.Printf(verb.Normal, "WordPress core version on %s: %s\n", verb.Blue(site.Name), verb.Gray("unknown"))
				continue
			}
			verb.Printf(verb.Normal, "WordPress core version on %s: %s\n", verb.Blue(site.Name), verb.Yellow(s.WPCore.Version))
		}
		return nil
	}

	c, err := apiClient()
	if err != nil {
		return err
	}
	ctx, stop := jobContext(cmd)
	defer stop()

	if operation == "check" {
		updates, upToDate := 0, 0
		for _, site := range sites {
			verb.Printf(verb.Verbose, "Checking WordPress core on %s...\n", site.Name)
			var core models.SiteCore
			if err := c.Get(ctx, "/sites/"+site.ID+"/core-update", &core); err != nil {
				if fatal := fatalAPIError(err); fatal != nil {
					return fatal
				}
				verb.PrintErrorf(verb.Normal, "Error checking core on %s: %v\n", site.Name, err)
				continue
			}
			if core.MinorUpdate == "" && core.MajorUpdate == "" {
				verb.Printf(verb.Verbose, "WordPress core is up to date on %s.\n", site.Name)
				upToDate++
				continue
			}
			for _, u := range []struct{ version, kind string }{{core.MinorUpdate, "minor"}, {core.MajorUpdate, "major"}} {
				if u.version != "" {
					verb.Printf(verb.Normal, "Update available for %s: %s -> %s (%s)\n", verb.Blue(site.Name), verb.Yellow(core.Version), verb.Green(u.version), u.kind)
				}
			}
			updates++
		}
		if upToDate > 0 {
			verb.Printf(verb.Normal, "%d site(s) are up to date.\n", upToDate)
		}
		if updates > 0 {
			verb.Printf(verb.Normal, "%d site(s) have updates available.\n", updates)
		}
		return nil
	}

	coreTarget := "minor"
	if coreMajor {
		coreTarget = "major"
	}
	var queued []models.UpdateJob
	for _, site := range sites {
		if coreMajor && !coreAllowMajor && m.IsCoreLocked(site.ID) {
			verb.Printf(verb.Normal, "Skipping %s: the site is update-locked; add --allow-major to update it to a new major version.\n", verb.Blue(site.Name))
			continue
		}
		verb.Printf(verb.Verbose, "Queueing WordPress core update on %s...\n", site.Name)
		var job models.UpdateJob
		body := map[string]any{"target": coreTarget, "allow_major": coreMajor && coreAllowMajor}
		if err := c.Post(ctx, "/sites/"+site.ID+"/core-update", body, &job); err != nil {
			if fatal := fatalAPIError(err); fatal != nil {
				return fatal
			}
			verb.PrintErrorf(verb.Normal, "Error updating core on %s: %v\n", site.Name, err)
			continue
		}
		queued = append(queued, job)
	}

	updated := 0
	err = waitForJobs(ctx, c, queued, sites, func(site models.CliSite, job models.UpdateJob) {
		r, ok := firstResult(job)
		switch {
		case !ok || r.Status == models.UpdateFailed || job.Status != models.UpdateJobDone:
			verb.PrintErrorf(verb.Normal, "Error updating core on %s: %s\n", site.Name, jobError(job, r))
		case r.Status == models.UpdateUpdated && r.OldVersion == r.NewVersion:
			verb.Printf(verb.Normal, "WordPress core on %s is up to date (%s).\n", verb.Blue(site.Name), r.NewVersion)
		case r.Status == models.UpdateUpdated:
			verb.Printf(verb.Normal, "Successfully updated WordPress core on %s: %s -> %s.\n", verb.Blue(site.Name), r.OldVersion, verb.Green(r.NewVersion))
			updated++
		default:
			verb.Printf(verb.Normal, "WordPress core on %s: %s.\n", verb.Blue(site.Name), strings.ToLower(r.Status))
		}
		if r.Note != "" {
			verb.Printf(verb.Normal, "  %s\n", verb.Yellow(r.Note))
		}
	})
	if updated > 0 {
		verb.Printf(verb.Normal, "Successfully updated core on %d site(s).\n", updated)
	}
	return err
}
