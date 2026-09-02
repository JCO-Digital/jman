package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

func setupSettingsTest(t *testing.T) {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "jman-settings-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Store old data dir to restore later
	oldDataDir := config.RunData.DataDir
	config.RunData.DataDir = tempDir

	if err := db.InitAPI(); err != nil {
		t.Fatalf("Failed to init DB: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
		os.RemoveAll(tempDir)
		config.RunData.DataDir = oldDataDir
	})
}

func TestListSettingsHandler(t *testing.T) {
	setupSettingsTest(t)
	username := "listuser"
	claims := &AuthClaims{Username: username, Level: config.LevelBasic}
	ctx := contextWithClaims(context.Background(), claims)

	// Initially empty list
	req := httptest.NewRequest("GET", "/api/settings", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	ListSettingsHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d", w.Code)
	}
	var settings []models.Setting
	if err := json.Unmarshal(w.Body.Bytes(), &settings); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if len(settings) != 0 {
		t.Errorf("Expected 0 settings, got %d", len(settings))
	}

	// Add a setting and check again
	_, err := db.SaveSetting(username, "theme", map[string]string{"color": "blue"})
	if err != nil {
		t.Fatalf("Failed to save setting: %v", err)
	}

	w = httptest.NewRecorder()
	ListSettingsHandler(w, req)
	if err := json.Unmarshal(w.Body.Bytes(), &settings); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if len(settings) != 1 {
		t.Errorf("Expected 1 setting, got %d", len(settings))
	}
}

func TestGetSettingHandler(t *testing.T) {
	setupSettingsTest(t)
	username := "getuser"
	key := "my-preference"
	claims := &AuthClaims{Username: username, Level: config.LevelBasic}
	ctx := contextWithClaims(context.Background(), claims)

	// Test 404
	req := httptest.NewRequest("GET", "/api/settings/"+key, nil)
	req.SetPathValue("key", key)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	GetSettingHandler(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404 Not Found, got %d", w.Code)
	}

	// Test Success
	_, _ = db.SaveSetting(username, key, "some-value")
	w = httptest.NewRecorder()
	GetSettingHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d", w.Code)
	}
	var s models.Setting
	if err := json.Unmarshal(w.Body.Bytes(), &s); err != nil {
		t.Fatalf("Failed to decode: %v", err)
	}
	if s.Key != key {
		t.Errorf("Expected key %s, got %s", key, s.Key)
	}
}

func TestSaveSettingHandler(t *testing.T) {
	setupSettingsTest(t)
	username := "saveuser"
	key := "ui-config"
	claims := &AuthClaims{Username: username, Level: config.LevelBasic}
	ctx := contextWithClaims(context.Background(), claims)

	val := map[string]any{"sidebar": "collapsed", "items": 10}
	body, _ := json.Marshal(val)
	req := httptest.NewRequest("POST", "/api/settings/"+key, bytes.NewBuffer(body))
	req.SetPathValue("key", key)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	SaveSettingHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d", w.Code)
	}

	var saved models.Setting
	json.Unmarshal(w.Body.Bytes(), &saved)
	if saved.Key != key {
		t.Errorf("Expected key %s, got %s", key, saved.Key)
	}

	// Verify persistence
	dbSetting, _ := db.GetSetting(username, key)
	if dbSetting == nil {
		t.Fatal("Setting was not found in database")
	}
}

func TestPatchSettingHandler(t *testing.T) {
	setupSettingsTest(t)
	username := "patchuser"
	key := "merge-test"
	claims := &AuthClaims{Username: username, Level: config.LevelBasic}
	ctx := contextWithClaims(context.Background(), claims)

	// 1. Patch non-existent setting should return 404
	patchVal := map[string]any{"new": "data"}
	body, _ := json.Marshal(patchVal)
	req := httptest.NewRequest("PATCH", "/api/settings/"+key, bytes.NewBuffer(body))
	req.SetPathValue("key", key)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	PatchSettingHandler(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404 for non-existent patch, got %d", w.Code)
	}

	// 2. Create and then patch (merge objects)
	initial := map[string]any{"a": 1, "b": 2}
	db.SaveSetting(username, key, initial)

	patch := map[string]any{"b": 20, "c": 30}
	pBody, _ := json.Marshal(patch)
	req = httptest.NewRequest("PATCH", "/api/settings/"+key, bytes.NewBuffer(pBody))
	req.SetPathValue("key", key)
	req = req.WithContext(ctx)
	w = httptest.NewRecorder()
	PatchSettingHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d", w.Code)
	}

	var patched models.Setting
	json.Unmarshal(w.Body.Bytes(), &patched)
	m, ok := patched.Value.(map[string]any)
	if !ok {
		t.Fatalf("Value is not a map: %T", patched.Value)
	}
	if m["a"].(float64) != 1 || m["b"].(float64) != 20 || m["c"].(float64) != 30 {
		t.Errorf("Merge result incorrect: %+v", m)
	}
}

func TestDeleteSettingHandler(t *testing.T) {
	setupSettingsTest(t)
	username := "deluser"
	key := "ephemeral"
	db.SaveSetting(username, key, "temporary")

	req := httptest.NewRequest("DELETE", "/api/settings/"+key, nil)
	req.SetPathValue("key", key)
	claims := &AuthClaims{Username: username, Level: config.LevelBasic}
	req = req.WithContext(contextWithClaims(context.Background(), claims))
	w := httptest.NewRecorder()

	DeleteSettingHandler(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("Expected 204 No Content, got %d", w.Code)
	}

	// Verify it's gone
	s, _ := db.GetSetting(username, key)
	if s != nil {
		t.Error("Setting still exists in database after DELETE")
	}
}
