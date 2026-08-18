package logs

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Entry is a single parsed access log line.
type Entry struct {
	RemoteAddr string
	Time       time.Time
	Method     string
	Path       string // request path, including query string, as logged
	Status     int
	Referer    string
	UserAgent  string
}

// combinedLogRe matches nginx/Apache's standard "combined" log format:
//
//	$remote_addr - $remote_user [$time_local] "$request" $status $body_bytes_sent "$http_referer" "$http_user_agent"
var combinedLogRe = regexp.MustCompile(`^(\S+) \S+ \S+ \[([^\]]+)\] "(\S+) (\S+)[^"]*" (\d{3}) (?:\d+|-) "([^"]*)" "([^"]*)"`)

// nginxTimeLayout matches nginx's $time_local format, e.g. "17/Aug/2026:00:16:22 +0300".
const nginxTimeLayout = "02/Jan/2006:15:04:05 -0700"

// ParseLine parses a single combined-format access log line.
func ParseLine(line string) (Entry, error) {
	m := combinedLogRe.FindStringSubmatch(line)
	if m == nil {
		return Entry{}, fmt.Errorf("line does not match combined log format")
	}

	t, err := time.Parse(nginxTimeLayout, m[2])
	if err != nil {
		return Entry{}, fmt.Errorf("failed to parse timestamp %q: %w", m[2], err)
	}

	status, err := strconv.Atoi(m[5])
	if err != nil {
		return Entry{}, fmt.Errorf("failed to parse status %q: %w", m[5], err)
	}

	return Entry{
		RemoteAddr: m[1],
		Time:       t,
		Method:     m[3],
		Path:       m[4],
		Status:     status,
		Referer:    m[6],
		UserAgent:  m[7],
	}, nil
}

// PathWithoutQuery strips the query string from a request path, so
// "/?jman_cache_bypass=123" and "/" group together for top-pages purposes.
func PathWithoutQuery(path string) string {
	if i := strings.IndexByte(path, '?'); i >= 0 {
		return path[:i]
	}
	return path
}
