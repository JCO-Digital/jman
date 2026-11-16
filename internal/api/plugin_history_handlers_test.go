package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

func TestPluginHistory(t *testing.T) {
	setupZipTest(t)
	site := "33333333-3333-3333-3333-333333333333"
	for _, e := range []struct{ updateType, status, data string }{
		// Oldest first.
		{"plugin", "detected", `{"detected":true,"source":"agent","changes":[{"plugin":"paid","change":"installed","new_version":"1.0"},{"plugin":"other","change":"installed","new_version":"2.0"}]}`},
		{"plugin", "partial", `{"plugin":"paid","old_version":"1.0","new_version":"1.0","skipped":true}`},
		{"plugin", "partial", `{"summary":"Bulk","updates":[{"plugin":"other","old_version":"2.0","new_version":"2.1","status":"success"},{"plugin":"paid","old_version":"1.0","new_version":"1.1","status":"failed","error":"boom"}]}`},
		{"plugin", "partial", `{"plugin":"paid","old_version":"1.0","new_version":"1.2","note":"Copied from alpha"}`},
		{"plugin", "deactivated", `{"action":"deactivate","plugins":[{"plugin":"paid","status":"success","version":"1.2"}]}`},
		{"plugin", "detected", `{"detected":true,"source":"ssh","changes":[{"plugin":"paid","change":"downgraded","old_version":"1.2","new_version":"1.1"},{"plugin":"paid","change":"activated","new_version":"1.1"}]}`},
		{"plugin", "full", `{"plugin":"paid-pro","old_version":"1","new_version":"2"}`},
		{"plugin", "full", `{"note":"manual, mentions \"paid\""}`},
		{"core", "full", `{"plugin":"paid","new_version":"6.8"}`},
	} {
		if err := db.SaveSiteUpdateLedgerEntry(&models.SiteUpdateLedgerEntry{SiteID: site, UpdateType: e.updateType, Status: e.status, DataJSON: e.data, UpdatedBy: "alice"}); err != nil {
			t.Fatal(err)
		}
	}

	get := func(query string) (events []PluginHistoryEvent, next *int, code int) {
		req := httptest.NewRequest(http.MethodGet, "/api/plugins/paid/history"+query, nil)
		req.SetPathValue("slug", "paid")
		rec := httptest.NewRecorder()
		PluginHistoryHandler(rec, req)
		var body struct {
			Events     []PluginHistoryEvent `json:"events"`
			NextBefore *int                 `json:"next_before"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		return body.Events, body.NextBefore, rec.Code
	}

	events, next, code := get("")
	if code != http.StatusOK || next != nil {
		t.Fatalf("status %d, next %v", code, next)
	}
	type want struct {
		action, outcome, oldV, newV string
		detected                    bool
	}
	wants := []want{
		{"update", "success", "1.2", "1.1", true},
		{"activate", "success", "", "1.1", true},
		{"deactivate", "success", "", "1.2", false},
		{"update", "success", "1.0", "1.2", false},
		{"update", "failed", "1.0", "1.1", false},
		{"update", "skipped", "1.0", "1.0", false},
		{"install", "success", "", "1.0", true},
	}
	if len(events) != len(wants) {
		t.Fatalf("got %d events, want %d: %+v", len(events), len(wants), events)
	}
	for i, w := range wants {
		e := events[i]
		if e.Action != w.action || e.Outcome != w.outcome || e.OldVersion != w.oldV || e.NewVersion != w.newV || e.Detected != w.detected || e.SiteID != site {
			t.Errorf("event %d = %+v, want %+v", i, e, w)
		}
	}
	if events[3].Note != "Copied from alpha" || events[4].Error != "boom" || events[0].By != "alice" {
		t.Errorf("note/error/by lost: %+v", events)
	}

	// Paging by ledger entries.
	page, next, _ := get("?limit=2")
	if len(page) != 3 || next == nil {
		t.Fatalf("page 1: %d events, next %v", len(page), next)
	}
	page, _, _ = get("?limit=10&before=" + strconv.Itoa(*next))
	if len(page) != 4 || page[0].Action != "update" || page[0].NewVersion != "1.2" {
		t.Errorf("page 2 = %+v", page)
	}

	for _, q := range []string{"?limit=0", "?limit=x", "?before=-1"} {
		if _, _, code := get(q); code != http.StatusBadRequest {
			t.Errorf("%s: status %d", q, code)
		}
	}
}
