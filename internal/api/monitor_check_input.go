package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/JCO-Digital/jman/internal/models"
)

// MonitorCheckRequest is the body for creating, editing or testing a
// monitor check. On edit, absent fields keep their current value.
type MonitorCheckRequest struct {
	// SiteID links a new check to a site; create and test only.
	SiteID *string  `json:"site_id"`
	Name   *string  `json:"name"`
	Host   *string  `json:"host"`
	Path   *string  `json:"path"`
	Expect *string  `json:"expect"`
	Assert *string  `json:"assert_path"`
	Op     *string  `json:"assert_op"`
	Value  rawField `json:"assert_value"`
	// CacheBypass is true, false, or null to follow the global setting.
	CacheBypass rawField `json:"cache_bypass"`

	// ID names a stored check to test: the request's fields override its
	// settings and its secret headers are sent. Test only.
	ID *string `json:"id"`
	// SecretHeaders are header values to send when testing, overriding the
	// stored check's. Test only; never stored.
	SecretHeaders map[string]string `json:"secret_headers"`
}

// rawField is a JSON field that tells "absent" (nil) from null.
type rawField []byte

func (f *rawField) UnmarshalJSON(b []byte) error {
	*f = append(rawField{}, b...)
	return nil
}

func (f rawField) isNull() bool {
	return bytes.Equal(bytes.TrimSpace(f), []byte("null"))
}

// probeFields reports whether a request changes what the check probes, as
// opposed to only its name.
func (req MonitorCheckRequest) probeFields() bool {
	return req.Host != nil || req.Path != nil || req.Expect != nil || req.Assert != nil ||
		req.Op != nil || req.Value != nil || req.CacheBypass != nil
}

// applyTo copies the request's fields onto c and validates the result.
func (req MonitorCheckRequest) applyTo(c *models.MonitorCheck) error {
	if req.Name != nil {
		c.Name = strings.TrimSpace(*req.Name)
	}
	if req.Host != nil {
		c.Host = strings.ToLower(strings.TrimSpace(*req.Host))
	}
	if req.Path != nil {
		c.Path = strings.TrimSpace(*req.Path)
	}
	if req.Expect != nil {
		c.Expect = strings.TrimSpace(*req.Expect)
	}
	if req.Assert != nil {
		c.AssertPath = strings.TrimSpace(*req.Assert)
	}
	if req.Op != nil {
		c.AssertOp = strings.TrimSpace(*req.Op)
	}
	if req.Value != nil {
		c.AssertValue = nil
		if !req.Value.isNull() {
			var compact bytes.Buffer
			if err := json.Compact(&compact, req.Value); err != nil {
				return fmt.Errorf("assert_value must be JSON")
			}
			c.AssertValue = compact.Bytes()
		}
	}
	if req.CacheBypass != nil {
		c.CacheBypass = nil
		if !req.CacheBypass.isNull() {
			var b bool
			if err := json.Unmarshal(req.CacheBypass, &b); err != nil {
				return fmt.Errorf("cache_bypass must be true, false or null")
			}
			c.CacheBypass = &b
		}
	}
	if c.Path == "" {
		c.Path = "/"
	}
	if c.Expect == "" {
		c.Expect = models.MonitorExpectHTML
	}
	return validateMonitorCheck(c)
}

func validateMonitorCheck(c *models.MonitorCheck) error {
	if c.Type != models.MonitorCheckTypeHTTP {
		return fmt.Errorf("unsupported check type %q", c.Type)
	}
	if len(c.Name) > 200 {
		return fmt.Errorf("name is longer than 200 characters")
	}

	if c.IsDefault {
		if c.Host != "" {
			return fmt.Errorf("a site's default check takes its host from the site")
		}
	} else if err := validateCheckHost(c.Host); err != nil {
		return err
	}

	if !strings.HasPrefix(c.Path, "/") || strings.ContainsAny(c.Path, " \t\r\n#") {
		return fmt.Errorf("path must start with / and contain no spaces or #")
	}
	if _, err := url.ParseRequestURI(c.Path); err != nil {
		return fmt.Errorf("invalid path: %v", err)
	}

	switch c.Expect {
	case models.MonitorExpectHTML, models.MonitorExpectJSON:
	default:
		return fmt.Errorf("expect must be %q or %q", models.MonitorExpectHTML, models.MonitorExpectJSON)
	}

	switch c.AssertOp {
	case "":
		if c.AssertPath != "" || len(c.AssertValue) > 0 {
			return fmt.Errorf("assert_path and assert_value need assert_op")
		}
		return nil
	case models.MonitorAssertEquals:
		if len(c.AssertValue) == 0 {
			return fmt.Errorf("assert_op %q needs assert_value", c.AssertOp)
		}
	case models.MonitorAssertExists:
		if len(c.AssertValue) > 0 {
			return fmt.Errorf("assert_op %q takes no assert_value", c.AssertOp)
		}
	default:
		return fmt.Errorf("assert_op must be %q or %q", models.MonitorAssertEquals, models.MonitorAssertExists)
	}
	if c.Expect != models.MonitorExpectJSON {
		return fmt.Errorf("assertions need expect %q", models.MonitorExpectJSON)
	}
	if c.AssertPath == "" || strings.HasPrefix(c.AssertPath, ".") || strings.HasSuffix(c.AssertPath, ".") || strings.Contains(c.AssertPath, "..") {
		return fmt.Errorf("assert_path must be a dotted path such as status or items.0.state")
	}
	return nil
}

// validateCheckHost requires a bare host name, optionally with a port.
func validateCheckHost(host string) error {
	if host == "" {
		return fmt.Errorf("host is required")
	}
	u, err := url.Parse("https://" + host)
	if err != nil || u.Host != host || u.Hostname() == "" || u.User != nil {
		return fmt.Errorf("host must be a bare host name such as api.example.com, without scheme or path")
	}
	return nil
}

var headerNamePattern = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")

// reservedHeaders can't be set as secret headers: the HTTP client manages them.
var reservedHeaders = map[string]bool{
	"Host": true, "Content-Length": true, "Transfer-Encoding": true, "Connection": true,
}

// normalizeSecretHeader validates a secret header and returns its
// canonical name.
func normalizeSecretHeader(name, value string) (string, error) {
	if !headerNamePattern.MatchString(name) {
		return "", fmt.Errorf("invalid header name %q", name)
	}
	name = http.CanonicalHeaderKey(name)
	if reservedHeaders[name] {
		return "", fmt.Errorf("header %s can't be set", name)
	}
	if value == "" || len(value) > 8192 || strings.ContainsAny(value, "\r\n") {
		return "", fmt.Errorf("header value must be 1-8192 characters on one line")
	}
	return name, nil
}
