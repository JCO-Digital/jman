package textdiff

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

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

func withoutCacheBypass(u *url.URL) string {
	c := *u
	q := c.Query()
	if !q.Has(utils.CacheBypassParam) {
		return c.String()
	}
	q.Del(utils.CacheBypassParam)
	c.RawQuery = q.Encode()
	return c.String()
}
