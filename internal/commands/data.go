package commands

import (
	"errors"
	"fmt"

	"github.com/JCO-Digital/jman/internal/client"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/credentials"
	"github.com/JCO-Digital/jman/internal/mirror"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/search"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/spf13/cobra"
)

// flagFresh makes commands check jman-api for new data whatever the age of
// the local copy.
var flagFresh bool

// flagYes answers every confirmation with yes: a command runs on all the
// sites matching its target without asking.
var flagYes bool

func init() {
	rootCmd.PersistentFlags().BoolVar(&flagFresh, "fresh", false, "check jman-api for new data first, however recent the local copy is")
	rootCmd.PersistentFlags().BoolVarP(&flagYes, "yes", "y", false, "don't ask for confirmation; run on every matching site")
}

// loadMirror returns the local copy of the jman-api data, refreshed first
// if it's older than the configured cacheTTL (or always, with --fresh).
func loadMirror(cmd *cobra.Command) (*mirror.Mirror, error) {
	return mirror.Get(cmd.Context(), mirror.Options{Force: flagFresh})
}

// completionMirror returns the local copy without contacting the API, for
// shell completion, which must be fast. It is empty if there is none.
func completionMirror() *mirror.Mirror {
	m, err := mirror.Load()
	if err != nil {
		return &mirror.Mirror{}
	}
	return m
}

// apiClient returns a client for the configured jman-api, logged in.
func apiClient() (*client.Client, error) {
	apiURL := config.Cfg.APIURL
	if apiURL == "" {
		return nil, mirror.ErrNoAPIURL
	}
	token, err := credentials.Get(apiURL)
	if errors.Is(err, credentials.ErrNotFound) {
		return nil, client.ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	return client.New(apiURL, token), nil
}

// siteAccess says whether a command only reads from the sites it targets.
type siteAccess bool

const (
	// readOnly commands run on a single matching site without asking.
	readOnly siteAccess = false
	// changesSite commands ask the user to confirm even a single match.
	changesSite siteAccess = true
)

// promptSites finds the sites matching target and asks the user to pick
// from them, or to confirm a single match unless the command is readOnly.
// It returns the mirror too, for further lookups.
func promptSites(cmd *cobra.Command, target string, access siteAccess) ([]models.CliSite, *mirror.Mirror, error) {
	m, err := loadMirror(cmd)
	if err != nil {
		return nil, nil, err
	}
	if flagYes {
		sites := search.SearchSites(m.CliSites(), target)
		if len(sites) == 0 {
			return nil, nil, fmt.Errorf("no sites found")
		}
		// Still show what --yes picked, as a broad target may match more
		// sites than intended.
		if access == changesSite || len(sites) > 1 {
			verb.PrintErrorf(verb.Normal, "Running on %d site(s):\n", len(sites))
			for _, site := range sites {
				verb.PrintErrorf(verb.Normal, "  %s %s\n", verb.Blue(site.Name), verb.Gray("("+site.ServerName+")"))
			}
		}
		return sites, m, nil
	}
	sites, err := search.PromptSearch(m.CliSites(), target, bool(access))
	if err != nil {
		return nil, nil, err
	}
	return sites, m, nil
}
