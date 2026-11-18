package textdiff

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"

	"github.com/JCO-Digital/jman/internal/utils"
)

// Page is one fetch of a page.
type Page struct {
	// Status is the HTTP status code, 0 when there was no response.
	Status int
	// FinalURL is the URL after redirects, without the cache bypass
	// parameter.
	FinalURL string
	// Lines is the page's visible text (see Extract).
	Lines []string
	// Error says why the page couldn't be compared: no response, a non-2xx
	// status, an error page or an empty body. Empty on success.
	Error string
}

// errorMarkers show a page that is broken whatever its status code:
// WordPress's wp_die() page (critical error, database connection error)
// and PHP errors printed into the page.
var errorMarkers = []struct {
	marker []byte
	msg    string
}{
	{[]byte(`id="error-page"`), "WordPress error page"},
	{[]byte(`<b>Fatal error</b>:`), "PHP fatal error in page"},
	{[]byte(`<b>Parse error</b>:`), "PHP parse error in page"},
}

// Fetch requests pageURL past the page cache (see utils.CacheBypassURL),
// sending headers, which are dropped on a redirect to another host, and
// returns its visible text.
func Fetch(ctx context.Context, client *http.Client, pageURL string, headers map[string]string) Page {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, utils.CacheBypassURL(pageURL), nil)
	if err != nil {
		return Page{Error: err.Error()}
	}
	utils.SetStandardHeaders(req)
	if len(headers) > 0 {
		for name, value := range headers {
			req.Header.Set(name, value)
		}
		client = utils.WithoutHeadersOnRedirect(client, headers)
	}
	resp, err := client.Do(req)
	if err != nil {
		return Page{Error: err.Error()}
	}
	defer resp.Body.Close()

	p := Page{Status: resp.StatusCode, FinalURL: withoutCacheBypass(resp.Request.URL)}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		p.Error = fmt.Sprintf("Body read error: %v", err)
		return p
	}
	for _, m := range errorMarkers {
		if bytes.Contains(body, m.marker) {
			p.Error = m.msg
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				p.Error += fmt.Sprintf(" (HTTP %d)", resp.StatusCode)
			}
			return p
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		p.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		return p
	}
	if len(bytes.TrimSpace(body)) == 0 {
		p.Error = "Empty page"
		return p
	}
	lines, err := Extract(bytes.NewReader(body))
	if err != nil {
		p.Error = fmt.Sprintf("HTML parse error: %v", err)
		return p
	}
	p.Lines = lines
	return p
}

// bypassParam matches the cache bypass parameter anywhere in a URL: in its
// own query, or URL-encoded (once or twice) inside another parameter, as
// in a redirect to wp-login.php?redirect_to=<the page with its bypass>.
// The separator before it is captured, and so is one after it, so the URL
// stays well formed without it.
var bypassParam = regexp.MustCompile(`(?i)(\?|&|%3F|%26|%253F|%2526)` + utils.CacheBypassParam +
	`(?:=|%3D|%253D)\d+(&|%26|%2526)?`)

// withoutCacheBypass returns u without the cache bypass parameter, which
// differs on every fetch, so final URLs of two fetches compare equal.
func withoutCacheBypass(u *url.URL) string {
	return bypassParam.ReplaceAllStringFunc(u.String(), func(m string) string {
		sub := bypassParam.FindStringSubmatch(m)
		if sub[2] == "" {
			// The last parameter: drop it with the separator before it.
			return ""
		}
		// Followed by another parameter: keep the separator before it
		// ("?" or "&") and drop the one after it.
		return sub[1]
	})
}
