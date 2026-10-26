package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/JCO-Digital/jman/internal/agent/scan"
	"github.com/JCO-Digital/jman/internal/models"
)

// Client talks to jman-api on behalf of a single server's jman-agent instance.
type Client struct {
	cfg        Config
	httpClient *http.Client
}

func NewClient(cfg Config) *Client {
	return &Client{cfg: cfg, httpClient: &http.Client{Timeout: 30 * time.Second}}
}

// FetchManifest retrieves the list of sites this agent's server is
// responsible for collecting data on.
func (c *Client) FetchManifest(ctx context.Context) (*models.AgentManifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.APIURL+"/agent/manifest", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Agent-Token", c.cfg.Token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach jman-api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("manifest request failed: status %d", resp.StatusCode)
	}

	var manifest models.AgentManifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("failed to decode manifest: %w", err)
	}
	return &manifest, nil
}

// SendReport posts a collection report, retrying a few times with backoff
// before giving up. Phase 1 data (disk usage, wp flags) is a point-in-time
// snapshot re-collected every cycle, so a dropped report after retries is
// simply superseded by the next cycle rather than requiring persistent
// spooling — that becomes necessary once incremental log data is involved.
func (c *Client) SendReport(ctx context.Context, report models.AgentReport) error {
	body, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("failed to encode report: %w", err)
	}

	const maxAttempts = 3
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * 5 * time.Second):
			}
		}

		if err := c.postReport(ctx, body); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return fmt.Errorf("failed to send report after %d attempts: %w", maxAttempts, lastErr)
}

func (c *Client) postReport(ctx context.Context, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.APIURL+"/agent/report", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("X-Agent-Token", c.cfg.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to reach jman-api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("report request failed: status %d", resp.StatusCode)
	}
	return nil
}

// errChecksumsUnknown is returned when wordpress.org has no checksums for
// a release.
var errChecksumsUnknown = errors.New("wordpress.org has no checksums for this release")

// FetchCoreChecksums returns the checksums of a WordPress release, which
// jman-api fetches from wordpress.org and caches.
func (c *Client) FetchCoreChecksums(ctx context.Context, version, locale string) (*scan.CoreChecksums, error) {
	q := url.Values{"version": {version}, "locale": {locale}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.APIURL+"/agent/core-checksums?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Agent-Token", c.cfg.Token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach jman-api: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, errChecksumsUnknown
	default:
		return nil, fmt.Errorf("core checksums request failed: status %d", resp.StatusCode)
	}
	var sums scan.CoreChecksums
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&sums); err != nil {
		return nil, fmt.Errorf("failed to decode core checksums: %w", err)
	}
	if sums.Version != version || sums.Locale != locale || len(sums.MD5) == 0 {
		return nil, fmt.Errorf("jman-api sent checksums for %s (%s), not %s (%s)", sums.Version, sums.Locale, version, locale)
	}
	return &sums, nil
}

// FetchPluginChecksums returns the SHA-256 checksums of a plugin release's
// PHP files, which jman-api fetches from wordpress.org and caches.
func (c *Client) FetchPluginChecksums(ctx context.Context, slug, version string) (*scan.PluginChecksums, error) {
	q := url.Values{"slug": {slug}, "version": {version}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.APIURL+"/agent/plugin-checksums?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Agent-Token", c.cfg.Token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach jman-api: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, errChecksumsUnknown
	default:
		return nil, fmt.Errorf("plugin checksums request failed: status %d", resp.StatusCode)
	}
	var sums scan.PluginChecksums
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&sums); err != nil {
		return nil, fmt.Errorf("failed to decode plugin checksums: %w", err)
	}
	if sums.Slug != slug || sums.Version != version || len(sums.SHA256) == 0 {
		return nil, fmt.Errorf("jman-api sent checksums for %s %s, not %s %s", sums.Slug, sums.Version, slug, version)
	}
	// Only PHP files are checked.
	for p := range sums.SHA256 {
		if !scan.IsPHPPath(p) {
			delete(sums.SHA256, p)
		}
	}
	return &sums, nil
}
