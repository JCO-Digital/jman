// Package mirror is the jman CLI's local copy of jman-api's data: the CLI
// snapshot (see cliproto.Snapshot), kept in the data directory. Commands
// read the mirror, and Get refreshes it from the API when it is older than
// the configured TTL. When the API can't be reached, commands keep working
// from the copy they have.
package mirror

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/JCO-Digital/jman/internal/client"
	"github.com/JCO-Digital/jman/internal/cliproto"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/credentials"
	"github.com/JCO-Digital/jman/internal/verb"
)

// ErrNoAPIURL is returned when the CLI hasn't been pointed at a jman-api.
var ErrNoAPIURL = errors.New("no jman-api configured; run `jman login <api-url>`")

// forcedTimeout bounds an explicit refresh, which the user is waiting for
// and which may follow a server-side SpinupWP refresh.
const forcedTimeout = 30 * time.Second

// updateNoticeInterval is how often the CLI mentions a newer release.
const updateNoticeInterval = 24 * time.Hour

// Meta is the bookkeeping stored next to the snapshot.
type Meta struct {
	// APIURL is the API the snapshot came from. A snapshot from another
	// API is discarded.
	APIURL string `json:"api_url"`
	ETag   string `json:"etag"`
	// FetchedAt is the last successful check with the API, whether or not
	// the data had changed.
	FetchedAt     time.Time `json:"fetched_at"`
	LastAttemptAt time.Time `json:"last_attempt_at"`
	// LastError is why the last check failed, or "" if it succeeded.
	LastError string `json:"last_error,omitempty"`
	// RefreshedAt is when the API last refreshed servers and sites from
	// SpinupWP.
	RefreshedAt      time.Time `json:"refreshed_at"`
	MinCLIVersion    string    `json:"min_cli_version,omitempty"`
	LatestCLIVersion string    `json:"latest_cli_version,omitempty"`
	LastUpdateNotice time.Time `json:"last_update_notice"`
}

// Mirror is the local copy. Snapshot is nil until one has been fetched.
type Mirror struct {
	Meta     Meta
	Snapshot *cliproto.Snapshot
}

// Options controls Get.
type Options struct {
	// Force checks the API whatever the TTL and backoff say, and fails
	// instead of falling back to the local copy.
	Force bool
}

func dir() string          { return filepath.Join(config.RunData.DataDir, "mirror") }
func snapshotPath() string { return filepath.Join(dir(), "snapshot.json") }
func metaPath() string     { return filepath.Join(dir(), "meta.json") }

// Load reads the local copy without contacting the API. A missing copy is
// not an error; the Mirror then has no Snapshot.
func Load() (*Mirror, error) {
	m := &Mirror{}
	if err := readJSON(metaPath(), &m.Meta); err != nil {
		return nil, err
	}
	var snap cliproto.Snapshot
	switch err := readJSON(snapshotPath(), &snap); {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		m.Snapshot = &snap
	}
	return m, nil
}

// Get returns the local copy, first refreshing it from the API if it is
// older than config.Cfg.CacheTTL (or always, with opts.Force). If the API
// can't be reached it returns the existing copy with a warning, and stops
// trying for config.Cfg.OfflineBackoff so that commands don't each wait for
// the timeout. It fails if there is no copy to fall back on, the login is
// missing or rejected, or the API requires a newer CLI.
func Get(ctx context.Context, opts Options) (*Mirror, error) {
	apiURL := config.Cfg.APIURL
	if apiURL == "" {
		return nil, ErrNoAPIURL
	}
	m, err := Load()
	if err != nil {
		return nil, err
	}
	if m.Meta.APIURL != apiURL {
		// Another API's data: never mix it with this one's.
		if err := Clear(); err != nil {
			return nil, err
		}
		m = &Mirror{Meta: Meta{APIURL: apiURL}}
	}

	now := time.Now()
	if !opts.Force && m.Snapshot != nil {
		if now.Sub(m.Meta.FetchedAt) < config.Cfg.CacheTTL {
			return m, nil
		}
		if m.Meta.LastError != "" && now.Sub(m.Meta.LastAttemptAt) < config.Cfg.OfflineBackoff {
			m.warnStale(now)
			return m, nil
		}
	}

	token, err := credentials.Get(apiURL)
	if err != nil {
		if errors.Is(err, credentials.ErrNotFound) {
			return nil, client.ErrUnauthorized
		}
		return nil, err
	}

	timeout := config.Cfg.APITimeout
	if opts.Force {
		timeout = forcedTimeout
	}
	fetchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := m.fetch(fetchCtx, client.New(apiURL, token), now); err != nil {
		var upgrade *client.UpgradeRequiredError
		var apiErr *client.APIError
		hardFailure := opts.Force || m.Snapshot == nil ||
			errors.Is(err, client.ErrUnauthorized) || errors.As(err, &upgrade) ||
			(errors.As(err, &apiErr) && apiErr.StatusCode < 500)
		if hardFailure {
			return nil, err
		}
		verb.PrintErrorf(verb.Normal, "Warning: can't reach jman-api (%v); using the local copy.\n", err)
		m.warnStale(now)
		return m, nil
	}
	return m, nil
}

// fetch checks the API for a new snapshot and saves the result. Failures
// are recorded in the meta so that Get can back off.
func (m *Mirror) fetch(ctx context.Context, c *client.Client, now time.Time) error {
	etag := ""
	if m.Snapshot != nil {
		etag = m.Meta.ETag
	}
	resp, err := c.Snapshot(ctx, etag)
	if min, latest, ok := c.Versions(); ok {
		m.Meta.MinCLIVersion, m.Meta.LatestCLIVersion = min, latest
	}
	m.Meta.LastAttemptAt = now
	if err == nil && !resp.NotModified {
		err = m.setSnapshot(resp.Body)
	}
	if err != nil {
		m.Meta.LastError = err.Error()
		_ = m.saveMeta()
		return err
	}

	m.Meta.ETag = resp.ETag
	m.Meta.FetchedAt = now
	m.Meta.LastError = ""
	if !resp.RefreshedAt.IsZero() {
		m.Meta.RefreshedAt = resp.RefreshedAt
	}
	if !resp.NotModified {
		if err := writeFileAtomic(snapshotPath(), resp.Body); err != nil {
			return err
		}
	}
	return m.saveMeta()
}

func (m *Mirror) setSnapshot(body []byte) error {
	var snap cliproto.Snapshot
	if err := json.Unmarshal(body, &snap); err != nil {
		return fmt.Errorf("failed to parse snapshot: %w", err)
	}
	if snap.SchemaVersion > cliproto.SnapshotSchemaVersion {
		return &client.UpgradeRequiredError{
			Current: config.AppVersion,
			Reason:  fmt.Sprintf("jman-api sends data in format %d, which jman %s can't read", snap.SchemaVersion, config.AppVersion),
		}
	}
	m.Snapshot = &snap
	return nil
}

// warnStale prints a warning when the local copy is older than
// config.Cfg.StaleWarning.
func (m *Mirror) warnStale(now time.Time) {
	if m.Meta.FetchedAt.IsZero() || config.Cfg.StaleWarning <= 0 {
		return
	}
	if age := now.Sub(m.Meta.FetchedAt); age > config.Cfg.StaleWarning {
		verb.PrintErrorf(verb.Normal, "Warning: working from data last updated %s ago.\n", age.Round(time.Hour))
	}
}

// CheckVersion returns an error if the API last reported a minimum CLI
// version newer than this one. It needs no network, so the minimum also
// applies offline.
func CheckVersion() error {
	m, err := Load()
	if err != nil || m.Meta.APIURL != config.Cfg.APIURL {
		return nil
	}
	return client.CheckMinVersion(config.AppVersion, m.Meta.MinCLIVersion)
}

// UpdateNotice prints a notice, at most once a day, when the API reported
// a newer CLI release than this one.
func UpdateNotice() {
	m, err := Load()
	if err != nil || !client.NewerAvailable(config.AppVersion, m.Meta.LatestCLIVersion) {
		return
	}
	if time.Since(m.Meta.LastUpdateNotice) < updateNoticeInterval {
		return
	}
	verb.PrintErrorf(verb.Normal, "jman %s is available (you have %s); run `jman update`.\n", m.Meta.LatestCLIVersion, config.AppVersion)
	m.Meta.LastUpdateNotice = time.Now()
	_ = m.saveMeta()
}

// Clear deletes the local copy, e.g. on logout.
func Clear() error {
	if err := os.RemoveAll(dir()); err != nil {
		return fmt.Errorf("failed to remove %s: %w", dir(), err)
	}
	return nil
}

func (m *Mirror) saveMeta() error {
	b, err := json.MarshalIndent(m.Meta, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(metaPath(), b)
}

func readJSON(path string, dest any) error {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if path == metaPath() {
			return nil
		}
		return err
	}
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", path, err)
	}
	if err := json.Unmarshal(b, dest); err != nil {
		return fmt.Errorf("failed to parse %s: %w", path, err)
	}
	return nil
}

func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("failed to create %s: %w", filepath.Dir(path), err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	return nil
}
