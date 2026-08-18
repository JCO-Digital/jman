package logs

import (
	"net"
	"net/url"
	"strings"
)

// isInternalReferrer reports whether referer points at the site's own
// primary domain, ignoring a leading "www." on either side, so that
// same-site navigation doesn't crowd out genuinely external referrers out of
// the fixed-size top-N list. Only the primary domain is checked — a site's
// additional/alias domains aren't visible to jman-agent (see
// models.AgentManifestSite), so a referral from an alias is treated as
// external rather than internal.
func isInternalReferrer(referer, siteDomain string) bool {
	if siteDomain == "" {
		return false
	}
	u, err := url.Parse(referer)
	if err != nil || u.Host == "" {
		return false
	}
	return normalizeHost(u.Host) == normalizeHost(siteDomain)
}

func normalizeHost(host string) string {
	host = strings.ToLower(host)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.TrimPrefix(host, "www.")
}
