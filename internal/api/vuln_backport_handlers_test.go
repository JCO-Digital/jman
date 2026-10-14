package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

func setupVulnBackportTest(t *testing.T) context.Context {
	t.Helper()
	dir := t.TempDir()
	old := config.RunData
	config.RunData.DataDir = dir
	config.RunData.CacheDir = dir
	if err := db.InitInventory(); err != nil {
		t.Fatalf("init inventory: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		config.RunData = old
	})

	// Seed the plugin's vulnerability cache so validation never fetches.
	min, minOp, max, maxOp := "9.8.0", "ge", "11.2.0", "lt"
	resp := models.VulnResponse{Data: &models.VulnData{Vulnerability: []models.Vulnerability{{
		Uuid:     "woo-1",
		Operator: models.Operator{MinVersion: &min, MinOperator: &minOp, MaxVersion: &max, MaxOperator: &maxOp},
	}}}}
	if err := cache.WriteJSONCache("vulnerabilities/woocommerce", resp); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	if err := db.SaveSitePlugin(models.WPPlugin{SiteID: "a", Name: "woocommerce", Status: "active", Version: "11.1.2"}); err != nil {
		t.Fatalf("seed plugin: %v", err)
	}

	return contextWithClaims(context.Background(), &AuthClaims{Username: "alice", Level: config.LevelEdit})
}

func postBackport(ctx context.Context, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/api/vuln-backports", bytes.NewReader([]byte(body))).WithContext(ctx)
	w := httptest.NewRecorder()
	CreateVulnBackportHandler(w, req)
	return w
}

func TestCreateVulnBackportHandler(t *testing.T) {
	ctx := setupVulnBackportTest(t)

	valid := `{"type":"plugin","slug":"woocommerce","vuln_uuid":"woo-1","branch":"11.1","min_version":"11.1.2","reason":"Backported in 11.1.2"}`
	w := postBackport(ctx, valid)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: got %d: %s", w.Code, w.Body.String())
	}
	var resp CreateVulnBackportResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Rule.ID == 0 || resp.Rule.CreatedBy != "alice" || resp.Rule.MatchedSites != 1 || len(resp.Warnings) != 0 {
		t.Errorf("unexpected response %+v", resp)
	}

	if w := postBackport(ctx, valid); w.Code != http.StatusConflict {
		t.Errorf("duplicate: got %d, want 409", w.Code)
	}

	malformed := `{"type":"plugin","slug":"woocommerce","vuln_uuid":"woo-1","branch":"11.1","min_version":"11.10.2","reason":"x"}`
	if w := postBackport(ctx, malformed); w.Code != http.StatusBadRequest {
		t.Errorf("malformed: got %d, want 400", w.Code)
	}

	// An unknown UUID is a warning, not an error.
	unknown := `{"type":"plugin","slug":"woocommerce","vuln_uuid":"nope","branch":"11.1","min_version":"11.1.2","reason":"x"}`
	w = postBackport(ctx, unknown)
	if w.Code != http.StatusCreated {
		t.Fatalf("unknown uuid: got %d: %s", w.Code, w.Body.String())
	}
	resp = CreateVulnBackportResponse{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || len(resp.Warnings) != 1 {
		t.Errorf("unknown uuid: want one warning, got %+v (err %v)", resp, err)
	}
}

func TestListAndDeleteVulnBackportHandlers(t *testing.T) {
	ctx := setupVulnBackportTest(t)
	w := postBackport(ctx, `{"type":"plugin","slug":"woocommerce","vuln_uuid":"woo-1","branch":"11.1","min_version":"11.1.2","reason":"x"}`)
	var created CreateVulnBackportResponse
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/vuln-backports", nil).WithContext(ctx)
	w = httptest.NewRecorder()
	ListVulnBackportsHandler(w, req)
	var listings []models.VulnBackportListing
	if err := json.Unmarshal(w.Body.Bytes(), &listings); err != nil || len(listings) != 1 || listings[0].MatchedSites != 1 {
		t.Fatalf("list: got %s (err %v)", w.Body.String(), err)
	}

	del := func(id string) int {
		req := httptest.NewRequest("DELETE", "/api/vuln-backports/"+id, nil).WithContext(ctx)
		req.SetPathValue("id", id)
		w := httptest.NewRecorder()
		DeleteVulnBackportHandler(w, req)
		return w.Code
	}
	id := strconv.Itoa(created.Rule.ID)
	if code := del(id); code != http.StatusNoContent {
		t.Errorf("delete: got %d, want 204", code)
	}
	if code := del(id); code != http.StatusNotFound {
		t.Errorf("delete again: got %d, want 404", code)
	}
	if code := del("abc"); code != http.StatusBadRequest {
		t.Errorf("delete bad id: got %d, want 400", code)
	}
}
