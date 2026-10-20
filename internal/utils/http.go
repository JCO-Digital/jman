package utils

import (
	"io"
	"net/http"
	"time"
)

// DefaultTimeout is the default timeout for HTTP requests if not specified.
const DefaultTimeout = 15 * time.Second

// UserAgent is the default User-Agent header for all outgoing requests.
const UserAgent = "jman/1.0 (WordPress Management Tool)"

// MaxResponseBodySize caps how much of a response body callers will read,
// bounding memory use if an upstream API is compromised or misbehaves.
const MaxResponseBodySize = 20 * 1024 * 1024 // 20 MiB

// NewHTTPClient returns a new http.Client with a standardized timeout and a
// response body size cap applied via its Transport.
func NewHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: &limitedBodyTransport{base: http.DefaultTransport},
	}
}

// NewFreshConnHTTPClient is NewHTTPClient without keep-alive: every
// request opens a new connection, looking the host up in DNS again. Uptime
// checks use it so they see a site as a new visitor would. A kept-alive
// connection would otherwise keep reaching the old server after a DNS
// change, for as long as requests come often enough to keep it open (the
// idle timeout is 90 seconds), e.g. while a site is checked often because
// it's down.
func NewFreshConnHTTPClient(timeout time.Duration) *http.Client {
	client := NewHTTPClient(timeout)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableKeepAlives = true
	client.Transport = &limitedBodyTransport{base: transport}
	return client
}

// limitedBodyTransport wraps a RoundTripper so every response body is capped
// at MaxResponseBodySize, regardless of which caller reads it.
type limitedBodyTransport struct {
	base http.RoundTripper
}

func (t *limitedBodyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil || resp == nil {
		return resp, err
	}
	resp.Body = &limitedReadCloser{
		r: io.LimitReader(resp.Body, MaxResponseBodySize),
		c: resp.Body,
	}
	return resp, nil
}

// limitedReadCloser pairs a size-limited Reader with the original body's
// Close, so callers can still close the underlying connection normally.
type limitedReadCloser struct {
	r io.Reader
	c io.Closer
}

func (l *limitedReadCloser) Read(p []byte) (int, error) { return l.r.Read(p) }
func (l *limitedReadCloser) Close() error               { return l.c.Close() }

// SetStandardHeaders applies global headers like User-Agent to an outgoing request.
func SetStandardHeaders(req *http.Request) {
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", UserAgent)
	}
}
