package api

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serveConditional(h http.HandlerFunc, method string, header map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api/x", nil)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	JsonMiddleware(ConditionalJSONMiddleware(h)).ServeHTTP(rec, req)
	return rec
}

var bigJSON = `[` + strings.Repeat(`{"name":"akismet","version":"5.0"},`, 100) + `{}]`

func TestConditionalJSONMiddleware(t *testing.T) {
	list := func(w http.ResponseWriter, r *http.Request) { WriteJSON(w, http.StatusOK, []byte(bigJSON)) }
	raw := func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, bigJSON) }

	rec := serveConditional(raw, http.MethodGet, map[string]string{"Accept-Encoding": "gzip, deflate, br"})
	etag := rec.Header().Get("ETag")
	if rec.Code != http.StatusOK || etag == "" || rec.Header().Get("Content-Encoding") != "gzip" || rec.Header().Get("Cache-Control") != "private, no-cache" {
		t.Fatalf("first GET: %d %v", rec.Code, rec.Header())
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	if body, _ := io.ReadAll(zr); string(body) != bigJSON {
		t.Errorf("decompressed body differs")
	}

	// Revalidating with the ETag: 304 without a body.
	rec = serveConditional(raw, http.MethodGet, map[string]string{"If-None-Match": etag, "Accept-Encoding": "gzip"})
	if rec.Code != http.StatusNotModified || rec.Body.Len() != 0 || rec.Header().Get("ETag") != etag {
		t.Errorf("revalidation: %d, %d bytes, %v", rec.Code, rec.Body.Len(), rec.Header())
	}
	// Changed data: a new ETag and the body.
	rec = serveConditional(list, http.MethodGet, map[string]string{"If-None-Match": etag})
	if rec.Code != http.StatusOK || rec.Header().Get("ETag") == etag || rec.Header().Get("Content-Encoding") != "" || rec.Body.Len() == 0 {
		t.Errorf("changed data without gzip: %d %v", rec.Code, rec.Header())
	}
	// gzip refused.
	if rec = serveConditional(raw, http.MethodGet, map[string]string{"Accept-Encoding": "gzip;q=0"}); rec.Header().Get("Content-Encoding") != "" {
		t.Errorf("gzip;q=0 got gzip")
	}
	// Small bodies aren't compressed.
	small := func(w http.ResponseWriter, r *http.Request) { WriteJSON(w, http.StatusOK, map[string]int{"n": 1}) }
	if rec = serveConditional(small, http.MethodGet, map[string]string{"Accept-Encoding": "gzip"}); rec.Header().Get("Content-Encoding") != "" || rec.Header().Get("ETag") == "" {
		t.Errorf("small body: %v", rec.Header())
	}
}

func TestConditionalJSONMiddlewarePassesThrough(t *testing.T) {
	cases := map[string]struct {
		method string
		h      http.HandlerFunc
	}{
		"POST": {http.MethodPost, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, bigJSON) }},
		"error": {http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
			WriteError(w, http.StatusNotFound, strings.Repeat("x", 2000))
		}},
		"not JSON": {http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/zip")
			_, _ = io.WriteString(w, bigJSON)
		}},
		"own ETag": {http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("ETag", `"mine"`)
			_, _ = io.WriteString(w, bigJSON)
		}},
		"flushed": {http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, "a")
			w.(http.Flusher).Flush()
			_, _ = io.WriteString(w, "b")
		}},
	}
	for name, c := range cases {
		rec := serveConditional(c.h, c.method, map[string]string{"Accept-Encoding": "gzip"})
		if rec.Header().Get("Content-Encoding") != "" || (name != "own ETag" && rec.Header().Get("ETag") != "") {
			t.Errorf("%s: headers %v", name, rec.Header())
		}
		if name == "flushed" && rec.Body.String() != "ab" {
			t.Errorf("flushed body = %q", rec.Body.String())
		}
	}
}
