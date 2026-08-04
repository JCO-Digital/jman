package wpcli

import (
	"sync"
	"time"
)

var (
	siteTrackerMu sync.RWMutex
	// siteFailures maps site IDs to the number of consecutive connection failures.
	siteFailures = make(map[int]int)

	updateRefreshMu sync.Mutex
	// lastUpdateRefresh maps site IDs to the last time their WordPress
	// plugin-update transient was forcibly refreshed.
	lastUpdateRefresh = make(map[int]time.Time)
)

// updateRefreshDebounce bounds how often we force a WordPress plugin-update
// transient refresh per site. The transient is site-wide, so one refresh
// covers every plugin in a batch of update calls; this just avoids paying
// for a redundant extra wp-cli round trip on every single call.
const updateRefreshDebounce = 60 * time.Second

// shouldRefreshUpdateCache reports whether enough time has passed since the
// last forced update-cache refresh for this site to warrant doing it again,
// and if so, records that a refresh is about to happen.
func shouldRefreshUpdateCache(siteID int) bool {
	updateRefreshMu.Lock()
	defer updateRefreshMu.Unlock()
	if last, ok := lastUpdateRefresh[siteID]; ok && time.Since(last) < updateRefreshDebounce {
		return false
	}
	lastUpdateRefresh[siteID] = time.Now()
	return true
}

// RecordFailure increments the failure count for a specific site.
func RecordFailure(siteID int) {
	siteTrackerMu.Lock()
	defer siteTrackerMu.Unlock()
	siteFailures[siteID]++
}

// RecordSuccess resets the failure count for a specific site.
func RecordSuccess(siteID int) {
	siteTrackerMu.Lock()
	defer siteTrackerMu.Unlock()
	delete(siteFailures, siteID)
}

// GetFailureCount returns the number of recorded failures for a site.
func GetFailureCount(siteID int) int {
	siteTrackerMu.RLock()
	defer siteTrackerMu.RUnlock()
	return siteFailures[siteID]
}

// IsSiteHealthy returns true if a site has no recorded failures.
func IsSiteHealthy(siteID int) bool {
	return GetFailureCount(siteID) == 0
}
