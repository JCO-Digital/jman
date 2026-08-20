package logs

import (
	"net"
	"strings"
)

// normalizeReferrerHost extracts and normalizes the hostname a Referer
// header points at, unifying "https://example.com", "http://example.com",
// a bare "example.com", and "www.example.com" (with or without a
// path/query/port, and regardless of case) into the same "example.com" key
// — so the top-N referrer ranking groups by actual referring site rather
// than being split across superficially different spellings of the same
// host. Returns "" if no usable host can be extracted (e.g. an empty or
// malformed referer).
//
// A plain strings.Cut on "://" (rather than net/url.Parse) is used to
// extract the host, since url.Parse treats a schemeless string like
// "example.com" as an opaque path rather than a host — exactly the bare-domain
// form this function needs to handle.
func normalizeReferrerHost(referer string) string {
	host := strings.TrimSpace(referer)
	if _, rest, ok := strings.Cut(host, "://"); ok {
		host = rest
	}
	if i := strings.IndexAny(host, "/?#"); i >= 0 {
		host = host[:i]
	}
	if at := strings.LastIndex(host, "@"); at >= 0 { // strip userinfo, e.g. "user@host"
		host = host[at+1:]
	}
	host = strings.ToLower(host)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.TrimPrefix(host, "www.")
	return host
}

// isInternalReferrer reports whether refHost (already normalized via
// normalizeReferrerHost) is the site's own primary domain. Only the primary
// domain is checked — a site's additional/alias domains aren't visible to
// jman-agent (see models.AgentManifestSite), so a referral from an alias is
// treated as external rather than internal.
func isInternalReferrer(refHost, siteDomain string) bool {
	if refHost == "" || siteDomain == "" {
		return false
	}
	return refHost == normalizeReferrerHost(siteDomain)
}
