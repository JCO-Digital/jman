package monitor

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

// withActiveScheduler registers a scheduler holding state as the running
// one for the duration of the test.
func withActiveScheduler(t *testing.T, state *State) {
	t.Helper()
	activeSchedulerMu.Lock()
	activeScheduler = &Scheduler{state: state}
	activeSchedulerMu.Unlock()
	t.Cleanup(func() {
		activeSchedulerMu.Lock()
		activeScheduler = nil
		activeSchedulerMu.Unlock()
	})
}

func TestCloseOutCheckResolvesIncidentAndDropsState(t *testing.T) {
	setupEngineTest(t)

	status := &CheckStatus{CheckID: "check-1", Domain: "down.example", CurrentMode: ModeAlert, IsDown: true}
	if err := SaveCheckStatus(status); err != nil {
		t.Fatal(err)
	}
	inc, err := db.CreateIncident("check-1", "down.example", "HTTP 500", 500, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	state := &State{Checks: map[string]*CheckStatus{"check-1": status}}
	withActiveScheduler(t, state)

	if err := CloseOutCheck("check-1", "", StopPaused, "alice", "maintenance"); err != nil {
		t.Fatalf("CloseOutCheck: %v", err)
	}

	got, err := db.GetIncidentByID(inc.ID)
	if err != nil || got == nil {
		t.Fatalf("incident: %+v, %v", got, err)
	}
	if got.Status != models.IncidentStatusResolved || got.ResolvedBy == nil || *got.ResolvedBy != "alice" {
		t.Errorf("incident not resolved by alice: %+v", got)
	}
	if _, ok := state.Checks["check-1"]; ok {
		t.Errorf("check still in the scheduler state")
	}
	if !status.Stopped {
		t.Errorf("status not marked stopped")
	}
	if records, _ := db.GetAllMonitorStatusRecords(); len(records) != 0 {
		t.Errorf("persisted status not dropped: %+v", records)
	}

	// Without an active incident there is nothing to resolve.
	if err := CloseOutCheck("check-1", "", StopPaused, "alice", ""); err != nil {
		t.Fatalf("second CloseOutCheck: %v", err)
	}
}

func TestStoppedCheckDiscardsResult(t *testing.T) {
	setupEngineTest(t)

	engine := newTestEngine(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	status := &CheckStatus{CheckID: "check-1", Domain: "down.example", CurrentMode: ModeInvestigation, FailureCount: 2, Stopped: true}

	if err := engine.Check(status); err != nil {
		t.Fatal(err)
	}
	if status.CurrentMode != ModeInvestigation || status.FailureCount != 2 {
		t.Errorf("stopped check changed state: %+v", status)
	}
	if inc, _ := db.GetActiveIncidentByCheck("check-1"); inc != nil {
		t.Errorf("stopped check opened an incident: %+v", inc)
	}
	if records, _ := db.GetAllMonitorStatusRecords(); len(records) != 0 {
		t.Errorf("stopped check saved its status: %+v", records)
	}
}

func TestCloseOutMessage(t *testing.T) {
	cases := []struct {
		reason StopReason
		note   string
		want   []string
	}{
		{StopPaused, "maintenance", []string{"⏸️", "PAUSED by alice", "incident #7 closed", "Reason: maintenance"}},
		{StopEdited, "", []string{"✏️", "EDITED by alice", "incident #7 closed", "re-checked"}},
		{StopOrphaned, "", []string{"⏸️", "STOPPED: its site was removed", "incident #7 closed"}},
	}
	for _, tc := range cases {
		msg := closeOutMessage("shop.example", tc.reason, "alice", tc.note, 7)
		for _, w := range tc.want {
			if !strings.Contains(msg, w) {
				t.Errorf("message %q lacks %q", msg, w)
			}
		}
		if strings.Contains(msg, "back up") {
			t.Errorf("message %q reads as a recovery", msg)
		}
	}
}

func TestOrphaningClosesOutIncident(t *testing.T) {
	setupEngineTest(t)

	gone := createCheck(t, models.MonitorCheck{SiteID: "site-gone", IsDefault: true, Enabled: true})
	inc, err := db.CreateIncident(gone.ID, "gone.example", "HTTP 500", 500, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	checks, err := db.ListMonitorChecks()
	if err != nil {
		t.Fatal(err)
	}
	reconcileDefaultChecks(checks, map[string]monitoredSite{}, true)

	got, err := db.GetIncidentByID(inc.ID)
	if err != nil || got == nil || got.Status != models.IncidentStatusResolved {
		t.Errorf("orphaned check's incident not resolved: %+v, %v", got, err)
	}
}
