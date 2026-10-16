package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JCO-Digital/jman/internal/db"
)

// checkIn posts a check-in through a mux, as the public route does.
func checkIn(t *testing.T, limiter *LoginRateLimiter, signal, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/checkin", CheckInHandler(limiter))
	mux.HandleFunc("POST /api/checkin/{signal}", CheckInHandler(limiter))
	url := "/api/checkin"
	if signal != "" {
		url += "/" + signal
	}
	req := httptest.NewRequest("POST", url, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func createHeartbeatViaAPI(t *testing.T) MonitorCheckResponse {
	t.Helper()
	w := doCheckRequest(t, CreateMonitorCheckHandler, "POST", "/api/monitor/checks",
		`{"type":"heartbeat","name":"Nightly backup","interval_seconds":86400,"grace_seconds":3600}`, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("create heartbeat: %d %s", w.Code, w.Body.String())
	}
	var got MonitorCheckResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestCreateHeartbeatCheck(t *testing.T) {
	setupIncidentTest(t)

	bad := []string{
		`{"type":"heartbeat","interval_seconds":3600}`,                                 // no name
		`{"type":"heartbeat","name":"x","interval_seconds":30}`,                        // interval too short
		`{"type":"heartbeat","name":"x","interval_seconds":3600,"host":"a.example"}`,   // HTTP setting
		`{"type":"heartbeat","name":"x","interval_seconds":3600,"fail_threshold":0.5}`, // not an int
		`{"type":"heartbeat","name":"x","interval_seconds":3600,"fail_threshold":101}`,
		`{"type":"ping","name":"x"}`,
		`{"host":"a.example","interval_seconds":3600}`, // interval on an HTTP check
	}
	for _, body := range bad {
		if w := doCheckRequest(t, CreateMonitorCheckHandler, "POST", "/", body, nil); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s, want 400", body, w.Code, w.Body.String())
		}
	}

	got := createHeartbeatViaAPI(t)
	if got.Type != "heartbeat" || got.IntervalSeconds != 86400 || got.GraceSeconds != 3600 || got.FailThreshold != 1 ||
		!strings.HasPrefix(got.Token, db.CheckInTokenPrefix) || !strings.HasPrefix(got.Token, got.TokenPrefix) {
		t.Errorf("created heartbeat = %+v", got)
	}

	// The token is shown once: reading the check doesn't return it.
	w := doCheckRequest(t, GetMonitorCheckHandler, "GET", "/", "", map[string]string{"id": got.ID})
	if strings.Contains(w.Body.String(), got.Token) || !strings.Contains(w.Body.String(), got.TokenPrefix) {
		t.Errorf("get leaks or lacks the token: %s", w.Body.String())
	}

	if w := doCheckRequest(t, UpdateMonitorCheckHandler, "PATCH", "/", `{"type":"http"}`, map[string]string{"id": got.ID}); w.Code != http.StatusBadRequest {
		t.Errorf("changing type: %d, want 400", w.Code)
	}
	if w := doCheckRequest(t, TestMonitorCheckHandler, "POST", "/", `{"id":"`+got.ID+`"}`, nil); w.Code != http.StatusBadRequest {
		t.Errorf("testing a heartbeat: %d, want 400", w.Code)
	}
}

func TestCheckInEndpoint(t *testing.T) {
	setupIncidentTest(t)
	limiter := NewLoginRateLimiter(false)
	t.Cleanup(limiter.Stop)
	check := createHeartbeatViaAPI(t)

	if w := checkIn(t, limiter, "start", check.Token, ""); w.Code != http.StatusOK {
		t.Fatalf("start: %d %s", w.Code, w.Body.String())
	}
	if w := checkIn(t, limiter, "fail", check.Token, "restic: exit 3\nrepository locked"); w.Code != http.StatusOK {
		t.Fatalf("fail: %d %s", w.Code, w.Body.String())
	}
	inc, _ := db.GetActiveIncidentByCheck(check.ID)
	if inc == nil || inc.ErrorMessage != "restic: exit 3 repository locked" {
		t.Fatalf("incident after fail: %+v", inc)
	}
	w := checkIn(t, limiter, "", check.Token, "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"accepted":true`) {
		t.Fatalf("ok: %d %s", w.Code, w.Body.String())
	}
	if inc, _ := db.GetActiveIncidentByCheck(check.ID); inc != nil {
		t.Errorf("incident still open after ok: %+v", inc)
	}

	if w := checkIn(t, limiter, "done", check.Token, ""); w.Code != http.StatusNotFound {
		t.Errorf("unknown signal: %d, want 404", w.Code)
	}

	// Rotating the token retires the old one.
	rotated := doCheckRequest(t, IssueMonitorCheckTokenHandler, "POST", "/", "", map[string]string{"id": check.ID})
	var resp MonitorCheckResponse
	_ = json.Unmarshal(rotated.Body.Bytes(), &resp)
	if rotated.Code != http.StatusOK || resp.Token == "" || resp.Token == check.Token {
		t.Fatalf("rotate: %d %s", rotated.Code, rotated.Body.String())
	}
	if w := checkIn(t, limiter, "ok", resp.Token, ""); w.Code != http.StatusOK {
		t.Errorf("new token: %d", w.Code)
	}

	// Invalid tokens count towards the rate limit: the old token, then
	// garbage, until the client is locked out.
	if w := checkIn(t, limiter, "ok", check.Token, ""); w.Code != http.StatusUnauthorized {
		t.Errorf("old token: %d, want 401", w.Code)
	}
	for i := 0; i < 10; i++ {
		checkIn(t, limiter, "ok", "nope", "")
	}
	if w := checkIn(t, limiter, "ok", resp.Token, ""); w.Code != http.StatusTooManyRequests {
		t.Errorf("after repeated invalid tokens: %d, want 429", w.Code)
	}
}

func TestCleanCheckInMessage(t *testing.T) {
	long := strings.Repeat("x", checkInMessageLimit+10)
	cases := map[string]string{
		"  exit 3\n\nlocked\t ": "exit 3 locked",
		"bad\x00\x07byte":       "badbyte",
		"\xff\xfeok":            "ok",
		long:                    strings.Repeat("x", checkInMessageLimit) + "…",
	}
	for in, want := range cases {
		if got := cleanCheckInMessage([]byte(in)); got != want {
			t.Errorf("cleanCheckInMessage(%q) = %q, want %q", in, got, want)
		}
	}
}
