package commands

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func callback(h http.Handler, query string) int {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/callback?"+query, nil))
	return w.Code
}

func TestLoginCallbackHandler(t *testing.T) {
	t.Run("rejects a wrong state", func(t *testing.T) {
		results := make(chan loginCallback, 1)
		h := loginCallbackHandler("s1", results)
		if code := callback(h, "state=other&code=abc"); code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", code)
		}
		if len(results) != 0 {
			t.Error("a wrong state produced a result")
		}
	})

	t.Run("delivers the code once", func(t *testing.T) {
		results := make(chan loginCallback, 1)
		h := loginCallbackHandler("s1", results)
		if code := callback(h, "state=s1&code=abc"); code != http.StatusOK {
			t.Fatalf("expected 200, got %d", code)
		}
		callback(h, "state=s1&code=def")
		if r := <-results; r.code != "abc" || r.err != nil {
			t.Errorf("unexpected result %+v", r)
		}
		if len(results) != 0 {
			t.Error("a second callback produced another result")
		}
	})

	t.Run("reports a denial", func(t *testing.T) {
		results := make(chan loginCallback, 1)
		callback(loginCallbackHandler("s1", results), "state=s1&error=access_denied")
		if r := <-results; r.err == nil || r.code != "" {
			t.Errorf("expected a denial, got %+v", r)
		}
	})
}

func TestTopLevelName(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"plugin", "list"})
	if err != nil {
		t.Fatal(err)
	}
	if got := topLevelName(cmd); got != "plugin" {
		t.Errorf("topLevelName = %q", got)
	}
	if got := topLevelName(loginCmd); got != "login" {
		t.Errorf("topLevelName = %q", got)
	}
}
