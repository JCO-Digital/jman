package monitor

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
)

// htmlSniffSize is how much of a response body we read to sanity-check that
// a 2xx response actually contains HTML, rather than an empty or blank page.
const htmlSniffSize = 4096

// htmlMarkers are byte sequences we expect to find somewhere near the start
// of a genuine HTML document.
var htmlMarkers = [][]byte{
	[]byte("<!doctype html"),
	[]byte("<html"),
	[]byte("<head"),
	[]byte("<body"),
}

// looksLikeHTML performs a lightweight sanity check on the start of a
// response body, so a 200 response with an empty or blank page isn't
// mistaken for a healthy site.
func looksLikeHTML(body []byte) bool {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return false
	}
	lower := bytes.ToLower(trimmed)
	for _, marker := range htmlMarkers {
		if bytes.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// ProbeResult is the outcome of probing a check's target once.
type ProbeResult struct {
	Up      bool
	Message string // why the target is down; empty when up
	Code    int    // HTTP status code; 0 when there was no response
}

// jsonBodyLimit caps how much of a JSON body a check reads.
const jsonBodyLimit = 1 << 20 // 1 MiB

// HTTPProbe is what an HTTP check requests and what it expects back.
type HTTPProbe struct {
	URL    string
	Expect string // models.MonitorExpectHTML or models.MonitorExpectJSON
	// AssertPath, AssertOp and AssertValue are an optional assertion on a
	// JSON body (see checkAssertion).
	AssertPath  string
	AssertOp    string
	AssertValue json.RawMessage
	CacheBypass bool
	// Headers are the check's secret headers. They are only ever sent, and
	// never to another host after a redirect.
	Headers map[string]string
}

// NewHTTPProbe builds the probe for a check that checks host. headers are
// the check's secret header values.
func NewHTTPProbe(c models.MonitorCheck, host string, headers map[string]string) HTTPProbe {
	path := c.Path
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	cacheBypass := config.Cfg.MonitorCacheBypass
	if c.CacheBypass != nil {
		cacheBypass = *c.CacheBypass
	}
	return HTTPProbe{
		URL:         "https://" + host + path,
		Expect:      c.Expect,
		AssertPath:  c.AssertPath,
		AssertOp:    c.AssertOp,
		AssertValue: c.AssertValue,
		CacheBypass: cacheBypass,
		Headers:     headers,
	}
}

// ProbeOnce runs one probe with a fresh engine, for trying out a check's
// settings. It records nothing.
func ProbeOnce(p HTTPProbe) ProbeResult {
	return NewEngine().probeHTTP(p)
}

// probeHTTP requests the probe's URL and requires a 2xx response whose
// body matches its expectation: HTML, or JSON passing the assertion.
func (e *Engine) probeHTTP(p HTTPProbe) ProbeResult {
	url := p.URL
	if p.CacheBypass {
		sep := "?"
		if strings.Contains(url, "?") {
			sep = "&"
		}
		url += fmt.Sprintf("%sjman_cache_bypass=%d", sep, time.Now().UnixNano())
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return ProbeResult{Message: fmt.Sprintf("Request Error: %v", err)}
	}
	utils.SetStandardHeaders(req)
	client := e.client
	if len(p.Headers) > 0 {
		for name, value := range p.Headers {
			req.Header.Set(name, value)
		}
		client = withoutHeadersOnRedirect(e.client, p.Headers)
	}
	resp, err := client.Do(req)
	if err != nil {
		return ProbeResult{Message: fmt.Sprintf("Error: %v", err)}
	}
	defer resp.Body.Close()

	result := ProbeResult{Code: resp.StatusCode}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.Message = fmt.Sprintf("HTTP %d", resp.StatusCode)
		return result
	}

	if p.Expect == models.MonitorExpectJSON {
		result.Message = checkJSONBody(resp.Body, p)
		result.Up = result.Message == ""
		return result
	}

	buf := make([]byte, htmlSniffSize)
	n, readErr := io.ReadFull(resp.Body, buf)
	switch {
	case readErr != nil && !errors.Is(readErr, io.ErrUnexpectedEOF) && !errors.Is(readErr, io.EOF):
		result.Message = fmt.Sprintf("Body read error: %v", readErr)
	case !looksLikeHTML(buf[:n]):
		result.Message = "Empty or non-HTML response body"
	default:
		result.Up = true
	}
	return result
}

// withoutHeadersOnRedirect returns a copy of client that drops the given
// headers when a redirect leaves the original host. Go only strips its own
// sensitive headers (Authorization, Cookie) on such redirects.
func withoutHeadersOnRedirect(client *http.Client, headers map[string]string) *http.Client {
	c := *client
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		if req.URL.Host != via[0].URL.Host {
			for name := range headers {
				req.Header.Del(name)
			}
		}
		return nil
	}
	return &c
}

// checkJSONBody returns why body fails the probe's JSON expectation, or ""
// if it passes.
func checkJSONBody(body io.Reader, p HTTPProbe) string {
	data, err := io.ReadAll(io.LimitReader(body, jsonBodyLimit+1))
	if err != nil {
		return fmt.Sprintf("Body read error: %v", err)
	}
	if len(data) > jsonBodyLimit {
		return "JSON body larger than 1 MiB"
	}
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		return "Invalid JSON body"
	}
	if p.AssertOp == "" {
		return ""
	}
	return checkAssertion(doc, p.AssertPath, p.AssertOp, p.AssertValue)
}

// checkAssertion checks one assertion on a decoded JSON document and
// returns why it fails, or "" if it holds. path is dot-separated; a
// numeric segment indexes an array ("items.0.status"). "exists" requires
// the path to be present (null counts); "equals" requires its value to
// equal want, compared as JSON values, so true and "true" differ.
func checkAssertion(doc any, path, op string, want json.RawMessage) string {
	got, ok := lookupJSONPath(doc, path)
	if !ok {
		return fmt.Sprintf("Assertion failed: %s is missing", path)
	}
	if op == models.MonitorAssertExists {
		return ""
	}

	var wantValue any
	if err := json.Unmarshal(want, &wantValue); err != nil {
		return fmt.Sprintf("Assertion failed: invalid expected value for %s", path)
	}
	if reflect.DeepEqual(got, wantValue) {
		return ""
	}
	gotJSON, _ := json.Marshal(got)
	return fmt.Sprintf("Assertion failed: %s is %s, want %s", path, truncate(string(gotJSON), 80), truncate(string(want), 80))
}

// lookupJSONPath follows a dot-separated path into a decoded JSON document.
func lookupJSONPath(doc any, path string) (any, bool) {
	cur := doc
	for _, seg := range strings.Split(path, ".") {
		switch v := cur.(type) {
		case map[string]any:
			next, ok := v[seg]
			if !ok {
				return nil, false
			}
			cur = next
		case []any:
			i, err := strconv.Atoi(seg)
			if err != nil || i < 0 || i >= len(v) {
				return nil, false
			}
			cur = v[i]
		default:
			return nil, false
		}
	}
	return cur, true
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// confirmHTTP applies the HTTP checks' confirmation rules to a probe
// result: a failure starts an investigation, three failures in a row
// confirm an outage, and three successes end an investigation. It returns
// what the result means for the check's outage and when to check next.
// Called with status.Mu held.
func confirmHTTP(status *CheckStatus, up bool) (outcome, time.Duration) {
	if up {
		status.ConsecutiveSuccesses++
		status.FailureCount = 0
	} else {
		status.FailureCount++
		status.ConsecutiveSuccesses = 0
	}

	switch status.CurrentMode {
	case ModeNormal:
		if !up {
			status.CurrentMode = ModeInvestigation
			return outcomeNone, time.Minute
		}
		return outcomeNone, 5 * time.Minute

	case ModeInvestigation:
		if up {
			if status.ConsecutiveSuccesses >= 3 {
				status.CurrentMode = ModeNormal
				return outcomeNone, 5 * time.Minute
			}
			return outcomeNone, time.Minute
		}
		if status.FailureCount >= 3 {
			status.CurrentMode = ModeAlert
			return outcomeDown, time.Minute
		}
		return outcomeNone, time.Minute

	case ModeAlert:
		if up {
			status.CurrentMode = ModeNormal
			return outcomeRecovered, 5 * time.Minute
		}
		return outcomeStillDown, time.Minute

	default:
		// Fallback for unknown modes
		status.CurrentMode = ModeNormal
		return outcomeNone, 5 * time.Minute
	}
}
