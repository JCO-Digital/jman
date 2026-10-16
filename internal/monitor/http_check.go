package monitor

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
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

// probeHTTP requests url and requires a 2xx response whose body looks like
// HTML.
func (e *Engine) probeHTTP(url string) ProbeResult {
	if config.Cfg.MonitorCacheBypass {
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
	resp, err := e.client.Do(req)
	if err != nil {
		return ProbeResult{Message: fmt.Sprintf("Error: %v", err)}
	}
	defer resp.Body.Close()

	result := ProbeResult{Code: resp.StatusCode}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.Message = fmt.Sprintf("HTTP %d", resp.StatusCode)
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
