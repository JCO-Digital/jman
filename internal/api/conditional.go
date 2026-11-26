package api

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
)

// minGzipBytes is the smallest JSON response worth compressing.
const minGzipBytes = 1024

// ConditionalJSONMiddleware gives successful JSON responses to GET
// requests an ETag and compresses them for clients that accept gzip. The
// web UI polls the large lists (sites, plugins, vulnerabilities) every few
// minutes in every open tab, mostly getting the same data again: the
// browser revalidates with If-None-Match (Cache-Control: no-cache) and an
// unchanged list costs a 304 instead of megabytes.
//
// Responses that aren't JSON, that set their own ETag or encoding (the CLI
// snapshot), or that the handler flushes pass through untouched.
func ConditionalJSONMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			next.ServeHTTP(w, r)
			return
		}
		cw := &conditionalWriter{ResponseWriter: w}
		next.ServeHTTP(cw, r)
		cw.finish(r)
	})
}

// conditionalWriter buffers a JSON response until the handler is done,
// unless it decides on the first write to pass the response through.
type conditionalWriter struct {
	http.ResponseWriter
	decided     bool
	passthrough bool
	status      int
	body        bytes.Buffer
}

func (c *conditionalWriter) decide(status int) {
	if c.decided {
		return
	}
	c.decided = true
	c.status = status
	h := c.Header()
	c.passthrough = status != http.StatusOK ||
		!strings.HasPrefix(h.Get("Content-Type"), "application/json") ||
		h.Get("ETag") != "" || h.Get("Content-Encoding") != ""
	if c.passthrough {
		c.ResponseWriter.WriteHeader(status)
	}
}

func (c *conditionalWriter) WriteHeader(status int) {
	c.decide(status)
}

func (c *conditionalWriter) Write(p []byte) (int, error) {
	c.decide(http.StatusOK)
	if c.passthrough {
		return c.ResponseWriter.Write(p)
	}
	return c.body.Write(p)
}

// Flush sends what was buffered and passes the rest through: a handler
// that flushes is streaming.
func (c *conditionalWriter) Flush() {
	c.decide(http.StatusOK)
	if !c.passthrough {
		c.passthrough = true
		c.ResponseWriter.WriteHeader(c.status)
		_, _ = c.ResponseWriter.Write(c.body.Bytes())
		c.body.Reset()
	}
	if f, ok := c.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (c *conditionalWriter) finish(r *http.Request) {
	if !c.decided {
		c.decide(http.StatusOK)
	}
	if c.passthrough {
		return
	}
	body := c.body.Bytes()
	sum := sha256.Sum256(body)
	etag := `"` + hex.EncodeToString(sum[:16]) + `"`
	h := c.Header()
	h.Set("ETag", etag)
	if h.Get("Cache-Control") == "" {
		// Private: the data is per user. No-cache: always revalidate.
		h.Set("Cache-Control", "private, no-cache")
	}
	h.Add("Vary", "Accept-Encoding")
	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		h.Del("Content-Type")
		c.ResponseWriter.WriteHeader(http.StatusNotModified)
		return
	}
	if len(body) >= minGzipBytes && acceptsGzip(r) {
		var gz bytes.Buffer
		zw, _ := gzip.NewWriterLevel(&gz, gzip.BestSpeed)
		_, _ = zw.Write(body)
		if err := zw.Close(); err == nil {
			h.Set("Content-Encoding", "gzip")
			body = gz.Bytes()
		}
	}
	h.Set("Content-Length", strconv.Itoa(len(body)))
	c.ResponseWriter.WriteHeader(c.status)
	_, _ = c.ResponseWriter.Write(body)
}

// acceptsGzip reports whether a request's Accept-Encoding allows gzip,
// which it does unless gzip is missing or given q=0.
func acceptsGzip(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		name, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if !strings.EqualFold(strings.TrimSpace(name), "gzip") {
			continue
		}
		for _, p := range strings.Split(params, ";") {
			k, v, _ := strings.Cut(strings.TrimSpace(p), "=")
			if strings.EqualFold(k, "q") {
				if q, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && q == 0 {
					return false
				}
			}
		}
		return true
	}
	return false
}
