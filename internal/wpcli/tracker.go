package wpcli

import (
	"sync"
)

var (
	siteTrackerMu sync.RWMutex
	// siteFailures maps site IDs to the number of consecutive connection failures.
	siteFailures = make(map[int]int)
)

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
