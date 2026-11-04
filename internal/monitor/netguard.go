package monitor

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"sort"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/slack"
	"github.com/JCO-Digital/jman/internal/verb"
)

// The network guard tells an outage of jman's own network apart from the
// sites': when checks of sites on many different addresses all fail to
// connect within a few minutes, the problem is almost certainly jman's
// (or its provider's) network, not every one of those sites. While such a
// network problem lasts, connection failures don't count towards a site's
// outage, so it doesn't send a down alert per site; one Slack message says
// what's going on instead. Sites that answer with an HTTP error still
// count, since jman reached them.

const (
	// guardWindow is how far back connection failures count.
	guardWindow = 5 * time.Minute
	// guardMinAddresses is how many different addresses must fail to
	// connect within guardWindow for a network problem. Sites on one
	// server share an address, so a server going down stays a site
	// outage.
	guardMinAddresses = 5
	// guardMaxHold is how long a network problem may hold back site
	// alerts; after it, sites are alerted on as usual again, so a real
	// outage of many servers can't stay hidden.
	guardMaxHold = 30 * time.Minute
	// guardLookupTimeout bounds resolving a failing site's address.
	guardLookupTimeout = 2 * time.Second
)

type guardFailure struct {
	at      time.Time
	address string
}

// netGuard tracks connection failures by check.
type netGuard struct {
	mu       sync.Mutex
	failures map[string]guardFailure // by check ID
	active   bool
	since    time.Time
	// expired is set when an active problem passed guardMaxHold; it then
	// holds nothing until the problem clears.
	expired bool
}

func newNetGuard() *netGuard {
	return &netGuard{failures: map[string]guardFailure{}}
}

// guard is the network guard of the running monitor.
var guard = newNetGuard()

// Seams for tests.
var (
	guardNow         = time.Now
	guardResolve     = resolveAddress
	sendGuardMessage = func(msg string) {
		verb.LogPrintf(verb.Normal, "%s\n", msg)
		if err := slack.SendMessageToChannel(msg, monitorSlackChannel(), true); err != nil {
			verb.LogPrintf(verb.Normal, "Failed to send the network problem message: %v\n", err)
		}
	}
)

// guardEvent is a change in the network problem state, for its Slack
// message.
type guardEvent struct {
	started, ended, expired bool
	sites, addresses        int
	duration                time.Duration
}

// prune drops failures older than guardWindow. Called with g.mu held.
func (g *netGuard) prune(now time.Time) {
	for id, f := range g.failures {
		if now.Sub(f.at) >= guardWindow {
			delete(g.failures, id)
		}
	}
}

// addresses counts the different addresses failing. Called with g.mu held.
func (g *netGuard) addresses() int {
	seen := map[string]bool{}
	for _, f := range g.failures {
		seen[f.address] = true
	}
	return len(seen)
}

// update starts, ends or expires the network problem as the failures now
// stand. Called with g.mu held.
func (g *netGuard) update(now time.Time) guardEvent {
	g.prune(now)
	n := g.addresses()
	switch {
	case !g.active && n >= guardMinAddresses:
		g.active, g.since, g.expired = true, now, false
		return guardEvent{started: true, sites: len(g.failures), addresses: n}
	case g.active && n < guardMinAddresses:
		ev := guardEvent{ended: true, duration: now.Sub(g.since), expired: g.expired}
		g.active, g.expired = false, false
		return ev
	case g.active && !g.expired && now.Sub(g.since) >= guardMaxHold:
		g.expired = true
		return guardEvent{expired: true, sites: len(g.failures), addresses: n, duration: now.Sub(g.since)}
	}
	return guardEvent{}
}

// failed records that a check couldn't connect to address and reports
// whether the failure should be held back (not count towards the site's
// outage) because of a network problem.
func (g *netGuard) failed(checkID, address string) (hold bool, ev guardEvent) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := guardNow()
	g.failures[checkID] = guardFailure{at: now, address: address}
	ev = g.update(now)
	return g.active && !g.expired, ev
}

// reached records that a check reached its site (whatever it answered).
func (g *netGuard) reached(checkID string) guardEvent {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.failures, checkID)
	return g.update(guardNow())
}

// isConnectionFailure reports whether a probe result means jman couldn't
// get a response at all (timeouts, refused or failed connections, DNS),
// as opposed to the site answering with an error.
func isConnectionFailure(r ProbeResult) bool {
	return !r.Up && r.Code == 0
}

// resolveAddress returns the first IP address of the probe URL's host, or
// the host itself when it can't be resolved (a DNS failure on jman's side
// then still counts each host as its own address).
func resolveAddress(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return rawURL
	}
	host := u.Hostname()
	if net.ParseIP(host) != nil {
		return host
	}
	ctx, cancel := context.WithTimeout(context.Background(), guardLookupTimeout)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil || len(addrs) == 0 {
		return host
	}
	sort.Strings(addrs)
	return addrs[0]
}

// announce sends the Slack message for a network problem event.
func announce(ev guardEvent) {
	switch {
	case ev.started:
		sendGuardMessage(fmt.Sprintf("⚠️ *jman can't connect to its monitored sites*: %d sites on %d different addresses failed to connect within %d minutes. "+
			"This looks like a problem with jman's own network or its provider, not the sites. Their down alerts are held until jman can connect again (for up to %d minutes).",
			ev.sites, ev.addresses, int(guardWindow.Minutes()), int(guardMaxHold.Minutes())))
	case ev.ended:
		msg := fmt.Sprintf("✅ *jman can connect to its monitored sites again* after about %d minutes; uptime checks count as usual.", minutesRounded(ev.duration))
		if ev.expired {
			msg = fmt.Sprintf("✅ *jman can connect to its monitored sites again* after about %d minutes.", minutesRounded(ev.duration))
		}
		sendGuardMessage(msg)
	case ev.expired:
		sendGuardMessage(fmt.Sprintf("🚨 *jman still can't connect to its monitored sites* after %d minutes (%d sites on %d addresses). Down alerts are no longer held, so sites that stay unreachable will be alerted on.",
			minutesRounded(ev.duration), ev.sites, ev.addresses))
	}
}

func minutesRounded(d time.Duration) int {
	return max(1, int(d.Round(time.Minute).Minutes()))
}
