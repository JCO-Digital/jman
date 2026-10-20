package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

const userTestSite = "11111111-1111-1111-1111-111111111111"

// setupSiteUsers initializes the databases with users stored for
// userTestSite: admin without posts and editor with 3.
func setupSiteUsers(t *testing.T) {
	t.Helper()
	oldDataDir := config.RunData.DataDir
	config.RunData.DataDir = t.TempDir()
	if err := db.InitInventory(); err != nil {
		t.Fatal(err)
	}
	if err := db.InitAPI(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Close()
		config.RunData.DataDir = oldDataDir
	})
	none, three := 0, 3
	users := models.SiteUsers{Users: []models.SiteUser{
		{ID: 1, Login: "admin", Email: "admin@example.com", Roles: "administrator", PostCount: &none},
		{ID: 2, Login: "editor", Email: "editor@example.com", Roles: "editor", PostCount: &three},
	}}
	if err := db.ReplaceSiteUsers(userTestSite, users, time.Now()); err != nil {
		t.Fatal(err)
	}
}

func userCreateRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/sites/"+userTestSite+"/users", strings.NewReader(body))
	req.SetPathValue("id", userTestSite)
	return req
}

func TestCheckNewUser(t *testing.T) {
	const pw = "long-enough-password"
	valid := [][4]string{
		{"jco", "jco@example.com", "administrator", pw},
		{"j.co-admin@x", "j+co@example.com", "shop_manager", pw},
		{"Jane Doe", "jane@example.com", "editor", pw},
	}
	for _, c := range valid {
		if msg := checkNewUser(c[0], c[1], c[2], c[3]); msg != "" {
			t.Errorf("checkNewUser(%q) = %q", c, msg)
		}
	}
	invalid := [][4]string{
		{"", "jco@example.com", "administrator", pw},
		{"--porcelain", "jco@example.com", "administrator", pw},
		{"jco ", "jco@example.com", "administrator", pw},
		{"j<script>", "jco@example.com", "administrator", pw},
		{strings.Repeat("a", 61), "jco@example.com", "administrator", pw},
		{"jco", "not an email", "administrator", pw},
		{"jco", "Jco <jco@example.com>", "administrator", pw},
		{"jco", "-x@example.com", "administrator", pw},
		{"jco", "jco@example.com", "Admin", pw},
		{"jco", "jco@example.com", "-x", pw},
		{"jco", "jco@example.com", "administrator", "short"},
		{"jco", "jco@example.com", "administrator", "has a\nnewline in it"},
	}
	for _, c := range invalid {
		if msg := checkNewUser(c[0], c[1], c[2], c[3]); msg == "" {
			t.Errorf("checkNewUser(%q) accepted", c)
		}
	}
}

func TestSiteUserCreateRejectsExistingUser(t *testing.T) {
	setupSiteUsers(t)
	for _, body := range []string{
		`{"login": "Admin", "email": "new@example.com", "password": "long-enough-password"}`,
		`{"login": "new", "email": "EDITOR@example.com", "password": "long-enough-password"}`,
	} {
		rec := httptest.NewRecorder()
		SiteUserCreateHandler(rec, userCreateRequest(body))
		if rec.Code != http.StatusConflict {
			t.Errorf("%s: status %d, want 409 (body %s)", body, rec.Code, rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	SiteUserCreateHandler(rec, userCreateRequest(`{"login": "new", "email": "new@example.com"}`))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("without a password: status %d, want 400", rec.Code)
	}
}

func userDeleteRequest(userID, reassign string) *http.Request {
	url := "/api/sites/" + userTestSite + "/users/" + userID
	if reassign != "" {
		url += "?reassign=" + reassign
	}
	req := httptest.NewRequest(http.MethodDelete, url, nil)
	req.SetPathValue("id", userTestSite)
	req.SetPathValue("userId", userID)
	return req
}

func TestSiteUserDeleteChecks(t *testing.T) {
	setupSiteUsers(t)
	cases := []struct {
		name, userID, reassign string
		want                   int
	}{
		{"bad ID", "x", "", http.StatusBadRequest},
		{"unknown user", "99", "", http.StatusNotFound},
		{"posts without reassign", "2", "", http.StatusConflict},
		{"reassign to self", "2", "editor@example.com", http.StatusBadRequest},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		SiteUserDeleteHandler(rec, userDeleteRequest(c.userID, c.reassign))
		if rec.Code != c.want {
			t.Errorf("%s: status %d, want %d (body %s)", c.name, rec.Code, c.want, rec.Body.String())
		}
	}

	if err := db.SetSiteWpFlags(userTestSite, true, false, nil); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	SiteUserDeleteHandler(rec, userDeleteRequest("1", ""))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "multisite") {
		t.Errorf("multisite: status %d (body %s), want 409", rec.Code, rec.Body.String())
	}
}
