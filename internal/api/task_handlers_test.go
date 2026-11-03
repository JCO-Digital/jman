package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListTasksHandlerEmpty(t *testing.T) {
	setupSettingsTest(t)

	rr := httptest.NewRecorder()
	ListTasksHandler(rr, httptest.NewRequest(http.MethodGet, "/api/tasks", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if body := strings.TrimSpace(rr.Body.String()); body != "[]" {
		t.Errorf("body = %s, want []", body)
	}
}
