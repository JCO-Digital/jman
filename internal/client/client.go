// Package client is the jman CLI's HTTP client for jman-api.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/cliproto"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/hashicorp/go-version"
)

// ErrUnauthorized is returned when the API rejects the CLI's token, or the
// CLI has none.
var ErrUnauthorized = errors.New("not logged in, or the login has expired or been revoked; run `jman login`")

// APIError is a non-success response from the API.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("jman-api returned %d", e.StatusCode)
	}
	return fmt.Sprintf("jman-api: %s (%d)", e.Message, e.StatusCode)
}

// UpgradeRequiredError is returned when the API requires a newer CLI.
type UpgradeRequiredError struct {
	Current, Minimum string
	// Reason replaces the default explanation when set.
	Reason string
}

func (e *UpgradeRequiredError) Error() string {
	if e.Reason != "" {
		return e.Reason + "; run `jman update`"
	}
	return fmt.Sprintf("jman %s is too old for this jman-api, which requires %s or newer; run `jman update`", e.Current, e.Minimum)
}

// Client talks to one jman-api.
type Client struct {
	baseURL string
	token   string
	http    *http.Client

	// minVersion and latestVersion are the CLI versions the API reported
	// in its last response.
	minVersion, latestVersion string
	sawVersions               bool
}

// New returns a client for the API at baseURL (see NormalizeURL),
// authenticating with token if it isn't empty.
func New(baseURL, token string) *Client {
	return &Client{baseURL: baseURL, token: token, http: &http.Client{}}
}

// NormalizeURL validates an API URL entered by a user and returns it in
// the form the CLI stores: scheme and host, plus any path prefix, without
// a trailing slash or "/api".
func NormalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return "", fmt.Errorf("invalid API URL %q: expected something like https://jman.example.com", raw)
	}
	u.RawQuery, u.Fragment = "", ""
	u.Path = strings.TrimSuffix(strings.TrimSuffix(u.Path, "/"), "/api")
	return strings.TrimSuffix(u.String(), "/"), nil
}

// Versions returns the minimum and latest CLI versions the API reported in
// its last response, and whether there was one.
func (c *Client) Versions() (min, latest string, ok bool) {
	return c.minVersion, c.latestVersion, c.sawVersions
}

// do sends a request to path (under /api) and returns the response, which
// the caller must close. It records the version headers, refuses to go on
// when the CLI is below the minimum version, and turns error statuses
// other than 304 into errors.
func (c *Client) do(ctx context.Context, method, path string, body any, header http.Header) (*http.Response, error) {
	if body == nil {
		return c.doRaw(ctx, method, path, nil, header)
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	if header == nil {
		header = http.Header{}
	}
	header.Set("Content-Type", "application/json")
	return c.doRaw(ctx, method, path, bytes.NewReader(b), header)
}

// doRaw is do with the request body given as a reader.
func (c *Client) doRaw(ctx context.Context, method, path string, body io.Reader, header http.Header) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+"/api"+path, body)
	if err != nil {
		return nil, err
	}
	for k, v := range header {
		req.Header[k] = v
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("User-Agent", "jman/"+config.AppVersion)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}

	if latest := resp.Header.Get(cliproto.LatestCLIVersionHeader); latest != "" {
		c.sawVersions = true
		c.latestVersion = latest
		c.minVersion = resp.Header.Get(cliproto.MinCLIVersionHeader)
	}
	if err := CheckMinVersion(config.AppVersion, c.minVersion); err != nil {
		resp.Body.Close()
		return nil, err
	}

	if resp.StatusCode < 400 {
		return resp, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized && c.token != "" {
		return nil, ErrUnauthorized
	}
	var e struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&e)
	return nil, &APIError{StatusCode: resp.StatusCode, Message: e.Error}
}

func (c *Client) doJSON(ctx context.Context, method, path string, body, out any) error {
	resp, err := c.do(ctx, method, path, body, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("failed to read jman-api response: %w", err)
	}
	return nil
}

// Health returns the API's health check, which needs no login.
func (c *Client) Health(ctx context.Context) (*cliproto.Health, error) {
	var h cliproto.Health
	if err := c.doJSON(ctx, http.MethodGet, "/health", nil, &h); err != nil {
		return nil, err
	}
	return &h, nil
}

// ExchangeCode trades an approved login code and its PKCE verifier for a
// CLI token.
func (c *Client) ExchangeCode(ctx context.Context, code, verifier string) (*cliproto.TokenResponse, error) {
	var resp cliproto.TokenResponse
	err := c.doJSON(ctx, http.MethodPost, "/cli/token", cliproto.TokenRequest{Code: code, Verifier: verifier}, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// RevokeCurrentToken revokes the client's own token.
func (c *Client) RevokeCurrentToken(ctx context.Context) error {
	return c.doJSON(ctx, http.MethodDelete, "/cli/token", nil, nil)
}

// Refresh asks the API to refresh servers and sites from SpinupWP and waits
// for it. It needs the execute level.
func (c *Client) Refresh(ctx context.Context) (*cliproto.RefreshResult, error) {
	var r cliproto.RefreshResult
	if err := c.doJSON(ctx, http.MethodPost, "/refresh", nil, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// SnapshotResponse is the result of Snapshot.
type SnapshotResponse struct {
	// NotModified is true when the snapshot still matches the given ETag;
	// Body is then empty.
	NotModified bool
	Body        []byte
	ETag        string
	// RefreshedAt is when the API last refreshed servers and sites.
	RefreshedAt time.Time
}

// Snapshot fetches the CLI snapshot. With a non-empty etag, an unchanged
// snapshot comes back as NotModified without a body.
func (c *Client) Snapshot(ctx context.Context, etag string) (*SnapshotResponse, error) {
	header := http.Header{}
	if etag != "" {
		header.Set("If-None-Match", etag)
	}
	resp, err := c.do(ctx, http.MethodGet, "/cli/snapshot", nil, header)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	out := &SnapshotResponse{ETag: resp.Header.Get("ETag")}
	if t, err := time.Parse(time.RFC3339, resp.Header.Get(cliproto.RefreshedAtHeader)); err == nil {
		out.RefreshedAt = t
	}
	if resp.StatusCode == http.StatusNotModified {
		out.NotModified = true
		if out.ETag == "" {
			out.ETag = etag
		}
		return out, nil
	}
	if out.Body, err = io.ReadAll(resp.Body); err != nil {
		return nil, fmt.Errorf("failed to read snapshot: %w", err)
	}
	return out, nil
}

// CheckMinVersion returns an UpgradeRequiredError if current is older than
// minimum. Development builds and unparsable versions are never blocked.
func CheckMinVersion(current, minimum string) error {
	if minimum == "" {
		return nil
	}
	if older, ok := isOlder(current, minimum); ok && older {
		return &UpgradeRequiredError{Current: current, Minimum: minimum}
	}
	return nil
}

// NewerAvailable reports whether latest is a newer release than current.
// It is false for development builds.
func NewerAvailable(current, latest string) bool {
	older, ok := isOlder(current, latest)
	return ok && older
}

func isOlder(current, other string) (bool, bool) {
	if current == "" || current == "dev" || other == "" || other == "dev" {
		return false, false
	}
	c, err1 := version.NewVersion(current)
	o, err2 := version.NewVersion(other)
	if err1 != nil || err2 != nil {
		return false, false
	}
	return c.LessThan(o), true
}

// Get sends a GET to path (under /api) and decodes the JSON response into
// out.
func (c *Client) Get(ctx context.Context, path string, out any) error {
	return c.doJSON(ctx, http.MethodGet, path, nil, out)
}

// Post sends body as JSON to path (under /api) and decodes the JSON
// response into out, if out isn't nil.
func (c *Client) Post(ctx context.Context, path string, body, out any) error {
	return c.doJSON(ctx, http.MethodPost, path, body, out)
}

// Delete sends a DELETE to path (under /api).
func (c *Client) Delete(ctx context.Context, path string) error {
	return c.doJSON(ctx, http.MethodDelete, path, nil, nil)
}

// PostFile uploads a local file as multipart/form-data to path (under
// /api), in the form field "file", along with fields, and decodes the JSON
// response into out.
func (c *Client) PostFile(ctx context.Context, path, filePath string, fields map[string]string, out any) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		for k, v := range fields {
			if err := mw.WriteField(k, v); err != nil {
				pw.CloseWithError(err)
				return
			}
		}
		part, err := mw.CreateFormFile("file", filepath.Base(filePath))
		if err == nil {
			_, err = io.Copy(part, f)
		}
		if err == nil {
			err = mw.Close()
		}
		pw.CloseWithError(err)
	}()

	header := http.Header{}
	header.Set("Content-Type", mw.FormDataContentType())
	resp, err := c.doRaw(ctx, http.MethodPost, path, pr, header)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// jobPollInterval is how often WaitForJobs checks on queued jobs.
var jobPollInterval = 2 * time.Second

// WaitForJobs polls the given update jobs until each has finished, calling
// done once per job, in the order they finish.
func (c *Client) WaitForJobs(ctx context.Context, jobs []models.UpdateJob, done func(models.UpdateJob)) error {
	pending := make(map[int64]bool, len(jobs))
	for _, j := range jobs {
		pending[j.ID] = true
	}
	for len(pending) > 0 {
		for id := range pending {
			var job models.UpdateJob
			if err := c.Get(ctx, fmt.Sprintf("/update-jobs/%d", id), &job); err != nil {
				return err
			}
			if !job.Active() {
				delete(pending, id)
				done(job)
			}
		}
		if len(pending) == 0 {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(jobPollInterval):
		}
	}
	return nil
}
