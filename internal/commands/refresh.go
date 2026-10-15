package commands

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/JCO-Digital/jman/internal/client"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/credentials"
	"github.com/JCO-Digital/jman/internal/mirror"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/spf13/cobra"
)

var refreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Update the local copy of the jman-api data",
	Long: `Downloads the latest site, server and plugin data from jman-api.

Users with the execute level also make jman-api refresh servers and sites
from SpinupWP first, so a site created moments ago shows up. jman-api skips
that step if it refreshed less than a minute ago.`,
	Args: usageArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		apiURL := config.Cfg.APIURL
		if apiURL == "" {
			return mirror.ErrNoAPIURL
		}
		token, err := credentials.Get(apiURL)
		if errors.Is(err, credentials.ErrNotFound) {
			return client.ErrUnauthorized
		} else if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
		defer cancel()

		verb.PrintErrorln(verb.Normal, "Refreshing servers and sites from SpinupWP...")
		res, err := client.New(apiURL, token).Refresh(ctx)
		var apiErr *client.APIError
		switch {
		case errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusForbidden:
			verb.PrintErrorln(verb.Normal, "Skipped: refreshing from SpinupWP needs the execute level.")
		case err != nil:
			return err
		case res.Skipped:
			verb.PrintErrorf(verb.Verbose, "jman-api last refreshed at %s; reusing that.\n", res.RefreshedAt.Local().Format(time.TimeOnly))
		}

		m, err := mirror.Get(ctx, mirror.Options{Force: true})
		if err != nil {
			return err
		}
		verb.Printf(verb.Normal, "%s %d sites on %d servers, %d plugin installs.\n", verb.Green("✓"), len(m.Sites()), len(m.Servers()), len(m.Plugins()))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(refreshCmd)
}
