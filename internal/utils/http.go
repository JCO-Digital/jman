package utils

import (
	"net/http"
	"time"
)

// DefaultTimeout is the default timeout for HTTP requests if not specified.
const DefaultTimeout = 15 * time.Second

// UserAgent is the default User-Agent header for all outgoing requests.
const UserAgent = "jman/1.0 (WordPress Management Tool)"

// NewHTTPClient returns a new http.Client with a standardized timeout.
func NewHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &http.Client{
		Timeout: timeout,
	}
}

// SetStandardHeaders applies global headers like User-Agent to an outgoing request.
func SetStandardHeaders(req *http.Request) {
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", UserAgent)
	}
}
