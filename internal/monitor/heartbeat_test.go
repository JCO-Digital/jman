package monitor

import (
	"strings"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

func newHeartbeatStatus(threshold int) *CheckStatus {
	return &CheckStatus{
		CheckID:     "hb-1",
		Type:        models.MonitorCheckTypeHeartbeat,
		CurrentMode: ModeNormal,
		Heartbeat:   HeartbeatSettings{Interval: time.Hour, Grace: 10 * time.Minute, FailThreshold: threshold},
	}
}

func TestConfirmHeartbeat(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	s := newHeartbeatStatus(2)

	// Waiting for the first check-in: the sweep never alerts.
	if oc := confirmHeartbeat(s, "", now); oc != outcomeNone || !s.NextCheckAt.Equal(waitingUntil) {
		t.Fatalf("waiting sweep: %v, next %v", oc, s.NextCheckAt)
	}

	if oc := confirmHeartbeat(s, SignalOK, now); oc != outcomeNone || !s.NextCheckAt.Equal(now.Add(70*time.Minute)) {
		t.Fatalf("first ok: %v, next %v", oc, s.NextCheckAt)
	}
	if oc := confirmHeartbeat(s, "", now.Add(69*time.Minute)); oc != outcomeNone {
		t.Errorf("sweep before the deadline: %v", oc)
	}
	if oc := confirmHeartbeat(s, "", now.Add(70*time.Minute)); oc != outcomeDown || s.CurrentMode != ModeAlert {
		t.Fatalf("sweep at the deadline: %v, mode %s", oc, s.CurrentMode)
	}
	if oc := confirmHeartbeat(s, "", now.Add(71*time.Minute)); oc != outcomeStillDown {
		t.Errorf("sweep while down: %v", oc)
	}
	later := now.Add(2 * time.Hour)
	if oc := confirmHeartbeat(s, SignalOK, later); oc != outcomeRecovered || s.CurrentMode != ModeNormal || !s.NextCheckAt.Equal(later.Add(70*time.Minute)) {
		t.Fatalf("ok after an outage: %v, mode %s, next %v", oc, s.CurrentMode, s.NextCheckAt)
	}

	// Two failures in a row with a threshold of 2.
	if oc := confirmHeartbeat(s, SignalFail, later.Add(time.Hour)); oc != outcomeNone || s.FailureCount != 1 {
		t.Errorf("first fail: %v, count %d", oc, s.FailureCount)
	}
	if oc := confirmHeartbeat(s, SignalFail, later.Add(2*time.Hour)); oc != outcomeDown {
		t.Errorf("second fail: %v", oc)
	}
	if oc := confirmHeartbeat(s, SignalFail, later.Add(3*time.Hour)); oc != outcomeStillDown {
		t.Errorf("fail while down: %v", oc)
	}
	if oc := confirmHeartbeat(s, SignalOK, later.Add(4*time.Hour)); oc != outcomeRecovered || s.FailureCount != 0 {
		t.Errorf("ok after failures: %v, count %d", oc, s.FailureCount)
	}

	// An ok between failures resets the count.
	s2 := newHeartbeatStatus(2)
	confirmHeartbeat(s2, SignalFail, now)
	confirmHeartbeat(s2, SignalOK, now.Add(time.Hour))
	if oc := confirmHeartbeat(s2, SignalFail, now.Add(2*time.Hour)); oc != outcomeNone {
		t.Errorf("fail after an ok: %v, want no alert", oc)
	}
}

func createHeartbeat(t *testing.T, threshold int) models.MonitorCheck {
	t.Helper()
	c := models.MonitorCheck{Type: models.MonitorCheckTypeHeartbeat, Name: "Nightly backup", Enabled: true,
		IntervalSeconds: 86400, GraceSeconds: 3600, FailThreshold: threshold}
	if err := db.CreateMonitorCheck(&c, "test"); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCheckInFlow(t *testing.T) {
	setupEngineTest(t)
	c := createHeartbeat(t, 1)

	if ok, err := CheckIn(c, SignalStart, ""); err != nil || !ok {
		t.Fatalf("start: %v, %v", ok, err)
	}
	records, _ := db.GetAllMonitorStatusRecords()
	if len(records) != 1 || records[0].LastStartedAt.IsZero() || !records[0].LastChecked.IsZero() {
		t.Fatalf("after start: %+v", records)
	}

	if _, err := CheckIn(c, SignalFail, "disk full"); err != nil {
		t.Fatal(err)
	}
	inc, _ := db.GetActiveIncidentByCheck(c.ID)
	if inc == nil || inc.ErrorMessage != "disk full" || inc.Domain != "Nightly backup" {
		t.Fatalf("incident after a failed check-in: %+v", inc)
	}

	if _, err := CheckIn(c, SignalOK, ""); err != nil {
		t.Fatal(err)
	}
	if inc, _ := db.GetActiveIncidentByCheck(c.ID); inc != nil {
		t.Errorf("incident still open after an ok check-in: %+v", inc)
	}
	records, _ = db.GetAllMonitorStatusRecords()
	if records[0].IsDown || records[0].LastChecked.IsZero() {
		t.Errorf("status after recovery: %+v", records[0])
	}

	c.Enabled = false
	if ok, err := CheckIn(c, SignalFail, "ignored"); err != nil || ok {
		t.Errorf("check-in to a paused check: accepted %v, %v", ok, err)
	}

	http := models.MonitorCheck{ID: "x", Type: models.MonitorCheckTypeHTTP}
	if _, err := CheckIn(http, SignalOK, ""); err != ErrNotHeartbeat {
		t.Errorf("check-in to an HTTP check: %v", err)
	}
}

func TestSweepHeartbeatAlertsOnMissedCheckIn(t *testing.T) {
	setupEngineTest(t)
	c := createHeartbeat(t, 1)
	status := &CheckStatus{}
	*status = CheckStatus{CheckID: c.ID, CurrentMode: ModeNormal}
	tg := heartbeatTarget(c)
	status.Type, status.Domain, status.Label, status.Heartbeat = tg.Type, tg.Domain, tg.Label, tg.Heartbeat
	status.LastChecked = time.Now().Add(-26 * time.Hour)

	engine := NewEngine()
	if err := engine.Check(status); err != nil {
		t.Fatal(err)
	}
	if status.CurrentMode != ModeAlert || !status.IsDown {
		t.Fatalf("missed deadline left %+v", status)
	}
	inc, _ := db.GetActiveIncidentByCheck(c.ID)
	if inc == nil || !strings.HasPrefix(inc.ErrorMessage, "No check-in since") {
		t.Errorf("incident for a missed check-in: %+v", inc)
	}
}

func TestHeartbeatNextCheck(t *testing.T) {
	now := time.Now()
	s := newHeartbeatStatus(1)
	if got := heartbeatNextCheck(s, now); !got.Equal(waitingUntil) {
		t.Errorf("waiting: %v", got)
	}
	s.LastChecked = now
	if got := heartbeatNextCheck(s, now); !got.Equal(now.Add(70 * time.Minute)) {
		t.Errorf("after a check-in: %v", got)
	}
	s.CurrentMode = ModeAlert
	s.NextCheckAt = now.Add(30 * time.Second)
	if got := heartbeatNextCheck(s, now); !got.Equal(s.NextCheckAt) {
		t.Errorf("while down: %v, want the repeat time kept", got)
	}
}
