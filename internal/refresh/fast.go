package refresh

import (
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/cache"
)

// ManualRefreshMinInterval is how soon after a successful servers/sites
// refresh a manually requested one is skipped. The SpinupWP fetch takes a
// few seconds, and the data doesn't change faster than this.
const ManualRefreshMinInterval = 60 * time.Second

// FastResult describes a servers/sites refresh.
type FastResult struct {
	Servers int `json:"servers"`
	Sites   int `json:"sites"`
	// RefreshedAt is when the last successful refresh finished.
	RefreshedAt time.Time `json:"refreshed_at"`
	// Skipped is true when a manual refresh was skipped because the last
	// one finished less than ManualRefreshMinInterval ago.
	Skipped bool `json:"skipped"`
}

// fastCall is one servers/sites refresh in progress. Callers that arrive
// while it runs wait on done and share its result.
type fastCall struct {
	done   chan struct{}
	result FastResult
	err    error
}

var fast struct {
	mu       sync.Mutex
	inflight *fastCall
	last     FastResult // last successful refresh
}

// refreshServersAndSites is the fetch itself, swappable in tests.
var refreshServersAndSites = func() (int, int, error) {
	servers, sites, err := cache.RefreshServersAndSites(0)
	return len(servers), len(sites), err
}

// RefreshServersAndSites fetches servers and sites from SpinupWP. If a
// refresh is already running, it waits for that one and returns its result
// instead of starting another, so the scheduler and manual requests never
// fetch concurrently.
func RefreshServersAndSites() (FastResult, error) {
	fast.mu.Lock()
	if call := fast.inflight; call != nil {
		fast.mu.Unlock()
		<-call.done
		return call.result, call.err
	}
	call := &fastCall{done: make(chan struct{})}
	fast.inflight = call
	fast.mu.Unlock()

	servers, sites, err := refreshServersAndSites()

	fast.mu.Lock()
	if err == nil {
		fast.last = FastResult{Servers: servers, Sites: sites, RefreshedAt: time.Now().UTC()}
		call.result = fast.last
	} else {
		call.result = fast.last
		call.err = err
	}
	fast.inflight = nil
	fast.mu.Unlock()
	close(call.done)

	return call.result, call.err
}

// RequestRefresh is a user-requested servers/sites refresh. It is skipped
// when the last successful refresh finished less than
// ManualRefreshMinInterval ago; otherwise it runs, or joins, a refresh.
func RequestRefresh() (FastResult, error) {
	fast.mu.Lock()
	last := fast.last
	running := fast.inflight != nil
	fast.mu.Unlock()

	if !running && !last.RefreshedAt.IsZero() && time.Since(last.RefreshedAt) < ManualRefreshMinInterval {
		last.Skipped = true
		return last, nil
	}
	return RefreshServersAndSites()
}

// LastRefresh returns when servers and sites were last refreshed
// successfully, or the zero time if they haven't been since start-up.
func LastRefresh() time.Time {
	fast.mu.Lock()
	defer fast.mu.Unlock()
	return fast.last.RefreshedAt
}
