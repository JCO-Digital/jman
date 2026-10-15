package commands

import (
	"errors"

	"github.com/JCO-Digital/jman/internal/client"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/credentials"
	"github.com/JCO-Digital/jman/internal/mirror"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/search"
	"github.com/spf13/cobra"
)

// flagFresh makes commands check jman-api for new data whatever the age of
// the local copy.
var flagFresh bool

func init() {
	rootCmd.PersistentFlags().BoolVar(&flagFresh, "fresh", false, "check jman-api for new data first, however recent the local copy is")
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

// promptSites finds the sites matching target and asks the user to confirm
// or pick from them. It returns the mirror too, for further lookups.
func promptSites(cmd *cobra.Command, target string) ([]models.CliSite, *mirror.Mirror, error) {
	m, err := loadMirror(cmd)
	if err != nil {
		return nil, nil, err
	}
	sites, err := search.PromptSearch(m.CliSites(), target)
	if err != nil {
		return nil, nil, err
	}
	return sites, m, nil
}
