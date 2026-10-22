package monitor

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

func TestCheckAssertion(t *testing.T) {
	var doc any
	if err := json.Unmarshal([]byte(`{"ok":true,"status":"true","count":3,"items":[{"state":"up"}],"empty":null}`), &doc); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		path, op, want string
		pass           bool
	}{
		{"ok", models.MonitorAssertEquals, `true`, true},
		{"ok", models.MonitorAssertEquals, `"true"`, false}, // typed: bool vs string
		{"status", models.MonitorAssertEquals, `"true"`, true},
		{"status", models.MonitorAssertEquals, `true`, false},
		{"count", models.MonitorAssertEquals, `3.0`, true},
		{"items.0.state", models.MonitorAssertEquals, `"up"`, true},
		{"items.1.state", models.MonitorAssertExists, ``, false},
		{"empty", models.MonitorAssertExists, ``, true},
		{"missing", models.MonitorAssertExists, ``, false},
		{"ok.deeper", models.MonitorAssertExists, ``, false},
	}
	for _, tc := range cases {
		msg := checkAssertion(doc, tc.path, tc.op, json.RawMessage(tc.want))
		if (msg == "") != tc.pass {
			t.Errorf("%s %s %s: got %q, want pass=%v", tc.path, tc.op, tc.want, msg, tc.pass)
		}
	}
	if msg := checkAssertion(doc, "ok", models.MonitorAssertEquals, json.RawMessage(`false`)); msg != "Assertion failed: ok is true, want false" {
		t.Errorf("failure message = %q", msg)
	}
}

func TestProbeHTTPJSON(t *testing.T) {
	body := `{"ok":true}`
	engine := newTestEngine(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})

	probe := HTTPProbe{URL: "https://mcp.example/healthz", Expect: models.MonitorExpectJSON,
		AssertPath: "ok", AssertOp: models.MonitorAssertEquals, AssertValue: json.RawMessage(`true`)}
	if r := engine.probeHTTP(probe); !r.Up {
		t.Errorf("healthy JSON reported down: %+v", r)
	}

	body = `{"ok":false}`
	if r := engine.probeHTTP(probe); r.Up || !strings.Contains(r.Message, "ok is false") {
		t.Errorf("failing assertion = %+v", r)
	}

	body = `<html>maintenance</html>`
	if r := engine.probeHTTP(probe); r.Up || r.Message != "Invalid JSON body" {
		t.Errorf("non-JSON body = %+v", r)
	}

	// JSON without an assertion only needs to parse.
	body = `[1,2]`
	if r := engine.probeHTTP(HTTPProbe{URL: "https://mcp.example/", Expect: models.MonitorExpectJSON}); !r.Up {
		t.Errorf("valid JSON without assertion reported down: %+v", r)
	}
}

func TestProbeHTTPSecretHeaders(t *testing.T) {
	var seen []string
	engine := newTestEngine(t, func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path+"="+r.Header.Get("X-Api-Key"))
		switch r.URL.Path {
		case "/same":
			http.Redirect(w, r, "https://api.example/landing", http.StatusFound)
		case "/away":
			http.Redirect(w, r, "https://elsewhere.example/landing", http.StatusFound)
		default:
			_, _ = w.Write([]byte("<html></html>"))
		}
	})
	headers := map[string]string{"X-Api-Key": "s3cret"}

	engine.probeHTTP(HTTPProbe{URL: "https://api.example/same", Expect: models.MonitorExpectHTML, Headers: headers})
	engine.probeHTTP(HTTPProbe{URL: "https://api.example/away", Expect: models.MonitorExpectHTML, Headers: headers})

	want := []string{"/same=s3cret", "/landing=s3cret", "/away=s3cret", "/landing="}
	if strings.Join(seen, " ") != strings.Join(want, " ") {
		t.Errorf("requests = %v, want %v (secret dropped only on the cross-host redirect)", seen, want)
	}
}

func TestNewHTTPProbe(t *testing.T) {
	on := true
	c := models.MonitorCheck{Path: "/realms/taika/.well-known/openid-configuration", Expect: models.MonitorExpectJSON, CacheBypass: &on}
	p := NewHTTPProbe(c, "auth.example", nil)
	if p.URL != "https://auth.example/realms/taika/.well-known/openid-configuration" || !p.CacheBypass || p.Expect != models.MonitorExpectJSON {
		t.Errorf("probe = %+v", p)
	}
	if l := CheckLabel(c, "auth.example"); l != "auth.example/realms/taika/.well-known/openid-configuration" {
		t.Errorf("label = %q", l)
	}
	c.Name = "Keycloak"
	if l := CheckLabel(c, "auth.example"); l != "Keycloak" {
		t.Errorf("named label = %q", l)
	}
}

func TestConfirmHTTPWaiting(t *testing.T) {
	state := &State{Checks: map[string]*CheckStatus{}}
	s := state.GetStatus(Target{CheckID: "check-new", Domain: "new.example"})
	if s.CurrentMode != ModeWaiting {
		t.Fatalf("new status mode = %q, want %q", s.CurrentMode, ModeWaiting)
	}

	// A site that isn't serving yet never alerts while waiting.
	for i := range 5 {
		if oc, next := confirmHTTP(s, false); oc != outcomeNone || s.CurrentMode != ModeWaiting || next != 5*time.Minute {
			t.Fatalf("failure %d while waiting: %v, mode %s, next %v", i+1, oc, s.CurrentMode, next)
		}
	}

	// Two successes are not enough; a failure in between starts over.
	confirmHTTP(s, true)
	confirmHTTP(s, true)
	confirmHTTP(s, false)
	for i := range 2 {
		if oc, next := confirmHTTP(s, true); oc != outcomeNone || s.CurrentMode != ModeWaiting || next != time.Minute {
			t.Fatalf("success %d while waiting: %v, mode %s, next %v", i+1, oc, s.CurrentMode, next)
		}
	}
	if oc, _ := confirmHTTP(s, true); oc != outcomeNone || s.CurrentMode != ModeNormal {
		t.Fatalf("third success: %v, mode %s", oc, s.CurrentMode)
	}

	// Once up, failures alert as usual.
	confirmHTTP(s, false)
	confirmHTTP(s, false)
	if oc, _ := confirmHTTP(s, false); oc != outcomeDown || s.CurrentMode != ModeAlert {
		t.Fatalf("third failure after waiting: %v, mode %s", oc, s.CurrentMode)
	}
}
