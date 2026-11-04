package monitor

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/db"
)

// stubGuard gives the test a fresh network guard with a clock it moves,
// addresses by URL host (each host its own address unless mapped), and
// the Slack messages it would send.
func stubGuard(t *testing.T, addresses map[string]string) (now *time.Time, sent *[]string) {
	t.Helper()
	oldGuard, oldNow, oldResolve, oldSend := guard, guardNow, guardResolve, sendGuardMessage
	t.Cleanup(func() { guard, guardNow, guardResolve, sendGuardMessage = oldGuard, oldNow, oldResolve, oldSend })

	clock := time.Date(2026, 11, 4, 17, 0, 0, 0, time.UTC)
	now, sent = &clock, &[]string{}
	guard = newNetGuard()
	guardNow = func() time.Time { return *now }
	guardResolve = func(rawURL string) string {
		host := strings.TrimPrefix(rawURL, "https://")
		host = strings.SplitN(host, "/", 2)[0]
		if a, ok := addresses[host]; ok {
			return a
		}
		return host
	}
	sendGuardMessage = func(msg string) { *sent = append(*sent, msg) }
	return now, sent
}

func TestNetGuard(t *testing.T) {
	now, _ := stubGuard(t, nil)
	g := guard

	for i := 1; i <= 4; i++ {
		if hold, ev := g.failed(fmt.Sprintf("c%d", i), fmt.Sprintf("10.0.0.%d", i)); hold || ev.started {
			t.Fatalf("failure %d: held with only %d addresses", i, i)
		}
	}
	// More sites on an address that already failed don't add to it.
	if hold, _ := g.failed("c4b", "10.0.0.4"); hold {
		t.Fatal("held with 4 addresses")
	}
	hold, ev := g.failed("c5", "10.0.0.5")
	if !hold || !ev.started || ev.addresses != 5 || ev.sites != 6 {
		t.Fatalf("5th address: hold %v, event %+v; want a started problem of 6 sites on 5 addresses", hold, ev)
	}

	// Reaching sites again ends it once fewer than 5 addresses fail.
	if ev := g.reached("c1"); !ev.ended {
		t.Fatalf("event after reaching c1 = %+v, want ended", ev)
	}
	if hold, _ := g.failed("c1", "10.0.0.1"); !hold {
		t.Fatal("not held after the problem started again")
	}

	// Failures older than the window stop counting.
	*now = now.Add(guardWindow)
	if ev := g.reached("unrelated"); !ev.ended {
		t.Fatalf("event after the window = %+v, want ended", ev)
	}
}

func TestNetGuardMaxHold(t *testing.T) {
	now, _ := stubGuard(t, nil)
	g := guard
	fail := func() (bool, guardEvent) {
		var hold bool
		var last guardEvent
		for i := 1; i <= 5; i++ {
			h, ev := g.failed(fmt.Sprintf("c%d", i), fmt.Sprintf("10.0.0.%d", i))
			hold = h
			if ev != (guardEvent{}) {
				last = ev
			}
		}
		return hold, last
	}
	if hold, ev := fail(); !hold || !ev.started {
		t.Fatalf("hold %v, event %+v; want a started problem", hold, ev)
	}
	for m := 1; m < int(guardMaxHold/time.Minute); m++ {
		*now = now.Add(time.Minute)
		if hold, _ := fail(); !hold {
			t.Fatalf("not held after %d minutes", m)
		}
	}
	*now = now.Add(time.Minute)
	hold, ev := fail()
	if hold || !ev.expired {
		t.Fatalf("after %s: hold %v, event %+v; want expired and not held", guardMaxHold, hold, ev)
	}
	*now = now.Add(time.Minute)
	if hold, ev := fail(); hold || ev != (guardEvent{}) {
		t.Fatalf("after expiry: hold %v, event %+v; want not held and no new event", hold, ev)
	}
}

// failingEngine's requests all fail to connect, as during the outage.
func failingEngine() *Engine {
	return &Engine{client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("dial tcp 65.108.199.45:443: i/o timeout")
	})}}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func httpCheck(id string) *CheckStatus {
	return &CheckStatus{CheckID: id, Domain: id, Probe: HTTPProbe{URL: "https://" + id + "/"}, CurrentMode: ModeNormal}
}

// Sites on many servers failing to connect at once: no site is alerted
// on, one message says why, and another when it's over.
func TestCheckHoldsAlertsDuringNetworkProblem(t *testing.T) {
	setupEngineTest(t)
	_, sent := stubGuard(t, nil)
	down := failingEngine()

	var checks []*CheckStatus
	for i := 1; i <= 6; i++ {
		checks = append(checks, httpCheck(fmt.Sprintf("site%d.example.com", i)))
	}
	for round := 0; round < 4; round++ {
		for _, c := range checks {
			_ = down.Check(c)
		}
	}
	for _, c := range checks {
		if c.CurrentMode == ModeAlert || c.IsDown {
			t.Errorf("%s alerted during the network problem (mode %s)", c.Domain, c.CurrentMode)
		}
		if inc, _ := db.GetActiveIncidentByCheck(c.CheckID); inc != nil {
			t.Errorf("%s got an incident during the network problem", c.Domain)
		}
	}
	if len(*sent) != 1 || !strings.Contains((*sent)[0], "on 5 different addresses") {
		t.Fatalf("messages = %q, want one about 5 addresses", *sent)
	}

	// The sites connect again: the problem ends with one message.
	up := newTestEngine(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<!DOCTYPE html><html><body>OK</body></html>"))
	})
	for _, c := range checks {
		_ = up.Check(c)
	}
	if len(*sent) != 2 || !strings.Contains((*sent)[1], "can connect to its monitored sites again") {
		t.Fatalf("messages = %q, want the end message second", *sent)
	}
}

// Sites on one server (one address) failing together is a real outage.
func TestCheckAlertsWhenOneServerIsDown(t *testing.T) {
	setupEngineTest(t)
	addresses := map[string]string{}
	for i := 1; i <= 6; i++ {
		addresses[fmt.Sprintf("site%d.example.com", i)] = "65.108.199.45"
	}
	_, sent := stubGuard(t, addresses)
	down := failingEngine()

	var checks []*CheckStatus
	for i := 1; i <= 6; i++ {
		checks = append(checks, httpCheck(fmt.Sprintf("site%d.example.com", i)))
	}
	for round := 0; round < 4; round++ {
		for _, c := range checks {
			_ = down.Check(c)
		}
	}
	for _, c := range checks {
		if c.CurrentMode != ModeAlert {
			t.Errorf("%s: mode %s, want alert", c.Domain, c.CurrentMode)
		}
	}
	if len(*sent) != 0 {
		t.Errorf("network problem messages = %q, want none", *sent)
	}
}

// A site that answers with an HTTP error is alerted on even during a
// network problem: jman reached it.
func TestCheckAlertsOnHTTPErrorsDuringNetworkProblem(t *testing.T) {
	setupEngineTest(t)
	_, _ = stubGuard(t, nil)
	down := failingEngine()
	for i := 1; i <= 5; i++ {
		_ = down.Check(httpCheck(fmt.Sprintf("site%d.example.com", i)))
	}
	if !guard.active {
		t.Fatal("network problem not detected")
	}

	broken := newTestEngine(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	c := httpCheck("broken.example.com")
	for i := 0; i < 4; i++ {
		_ = broken.Check(c)
	}
	if c.CurrentMode != ModeAlert {
		t.Errorf("mode %s, want alert", c.CurrentMode)
	}
}
