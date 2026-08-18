package logs

import "strings"

// botUserAgentSubstrings is a static, dependency-free list of common
// crawler/bot user-agent substrings, checked case-insensitively. This is a
// deliberately simple v1 (no fingerprinting, no rate-based heuristics) —
// it won't catch a bot spoofing a real browser UA.
var botUserAgentSubstrings = []string{
	"bot", "spider", "crawl", "slurp", "facebookexternalhit",
	"ia_archiver", "pingdom", "uptimerobot", "mj12bot", "dotbot",
	"petalbot", "bytespider", "duckduckbot", "applebot",
	"google-inspectiontool", "adsbot",
}

// IsBotUserAgent reports whether ua looks like a known crawler/bot.
func IsBotUserAgent(ua string) bool {
	lower := strings.ToLower(ua)
	for _, s := range botUserAgentSubstrings {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}

// IsInternalTraffic reports whether an entry is jman's own synthetic
// traffic (jman-monitor's uptime checks) rather than a real visitor or a
// third-party bot — these would otherwise inflate every site's traffic
// counts by however often jman-monitor pings it. Excluded entirely from
// both human and bot counts.
func IsInternalTraffic(e Entry) bool {
	if strings.HasPrefix(strings.ToLower(e.UserAgent), "jman/") {
		return true
	}
	return strings.Contains(e.Path, "jman_cache_bypass=")
}
