package vuln

import (
	"fmt"
	"time"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
)

// SnapshotMute records the exposure a mute is made or confirmed for, and
// (re)starts its review period: the mute is active for
// models.VulnMuteReviewDays from now.
func SnapshotMute(m *models.VulnMute, e models.VulnExposure, now time.Time) {
	m.FixStateAtMute = e.FixState
	m.VulnUUIDsAtMute = append([]string(nil), e.VulnUUIDs...)
	m.PossibleFixAtMute = e.PossibleFixVersion
	m.ReviewAt = now.AddDate(0, 0, models.VulnMuteReviewDays)
	m.Status = models.VulnMuteActive
}

// muteLiftReason says why a mute no longer holds for its exposure, or ""
// if it still does. Any vulnerability that wasn't there when the mute was
// made lifts it. A mute made while there was no fix is also lifted when a
// fix, or a newer version outside the affected ranges, shows up; a mute
// made while a fix was available isn't, since new releases keep coming.
func muteLiftReason(m models.VulnMute, e models.VulnExposure) string {
	known := make(map[string]bool, len(m.VulnUUIDsAtMute))
	for _, u := range m.VulnUUIDsAtMute {
		known[u] = true
	}
	for _, u := range e.VulnUUIDs {
		if !known[u] {
			return "A new vulnerability applies"
		}
	}
	if m.FixStateAtMute != models.VulnNoFix {
		return ""
	}
	if e.FixState == models.VulnFixAvailable {
		return fmt.Sprintf("A fix is available (%s)", e.FixedIn)
	}
	if e.PossibleFixVersion != "" && e.PossibleFixVersion != m.PossibleFixAtMute {
		return fmt.Sprintf("Version %s may fix it", e.PossibleFixVersion)
	}
	return ""
}

// planMuteLifecycle returns the open mutes that change at now: lifted
// because muteLiftReason says so or because their exposure is gone (and
// wasn't resolved recently enough to be reopened), or moved to needs
// review once their review date has passed.
func planMuteLifecycle(mutes []models.VulnMute, open map[exposureKey]models.VulnExposure,
	recentlyResolved map[exposureKey]bool, now time.Time) []models.VulnMute {
	var changed []models.VulnMute
	lift := func(m models.VulnMute, reason string) {
		t := now
		m.Status, m.LiftedAt, m.LiftedReason, m.UpdatedBy = models.VulnMuteLifted, &t, reason, "system"
		changed = append(changed, m)
	}
	for _, m := range mutes {
		if !m.Open() {
			continue
		}
		k := exposureKey{m.Kind, m.Slug, m.SiteID}
		e, ok := open[k]
		switch {
		case !ok && recentlyResolved[k]:
			// It may come back within the reopen window.
		case !ok:
			lift(m, "No longer affected")
		case muteLiftReason(m, e) != "":
			lift(m, muteLiftReason(m, e))
		case m.Status == models.VulnMuteActive && !now.Before(m.ReviewAt):
			m.Status, m.UpdatedBy = models.VulnMuteNeedsReview, "system"
			changed = append(changed, m)
		}
	}
	return changed
}

// syncVulnMutes applies planMuteLifecycle to the stored mutes, against the
// exposures as they are after a sync.
func syncVulnMutes(now time.Time) error {
	mutes, err := db.ListOpenVulnMutes()
	if err != nil || len(mutes) == 0 {
		return err
	}
	exposures, err := db.ListOpenVulnExposures()
	if err != nil {
		return err
	}
	open := make(map[exposureKey]models.VulnExposure, len(exposures))
	for _, e := range exposures {
		open[keyOf(e)] = e
	}
	resolved, err := db.ListVulnExposuresResolvedSince(now.Add(-exposureReopenWindow))
	if err != nil {
		return err
	}
	recent := make(map[exposureKey]bool, len(resolved))
	for _, e := range resolved {
		recent[keyOf(e)] = true
	}

	for _, m := range planMuteLifecycle(mutes, open, recent, now) {
		if err := db.UpdateVulnMute(&m); err != nil {
			return err
		}
		if m.Status == models.VulnMuteLifted {
			verb.LogPrintf(verb.Normal, "Lifted vulnerability mute %d (%s %s on %s): %s\n", m.ID, m.Kind, m.Slug, m.SiteID, m.LiftedReason)
		}
	}
	return nil
}
