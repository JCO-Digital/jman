package db

import (
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

func TestReplaceSiteUsers(t *testing.T) {
	setupTaskRepoTest(t)
	site := testSiteID(1)

	if _, found, err := GetSiteUsers(site); err != nil || found {
		t.Fatalf("users before any were stored: found=%v err=%v", found, err)
	}
	if _, found, err := GetSiteUsersObservedAt(site); err != nil || found {
		t.Fatalf("observed time before any users were stored: found=%v err=%v", found, err)
	}

	observed := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	posts := 7
	users := models.SiteUsers{
		Users: []models.SiteUser{
			{ID: 5, Login: "ed", Email: "ed@example.com", DisplayName: "Ed", Roles: "editor,author", Registered: "2021-01-01 00:00:00", PostCount: &posts},
			{ID: 1, Login: "admin", Email: "admin@example.com", Roles: "administrator"},
		},
		ExcludedCounts: map[string]int{"subscriber": 1200, "customer": 3},
		Truncated:      true,
	}
	if err := ReplaceSiteUsers(site, users, observed); err != nil {
		t.Fatal(err)
	}

	got, found, err := GetSiteUsers(site)
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if len(got.Users) != 2 || got.Users[0].ID != 1 || got.Users[1].Roles != "editor,author" || got.Users[1].SiteID != site {
		t.Errorf("users = %+v", got.Users)
	}
	if got.Users[0].PostCount != nil || got.Users[1].PostCount == nil || *got.Users[1].PostCount != 7 {
		t.Errorf("post counts = %v, %v; want unknown and 7", got.Users[0].PostCount, got.Users[1].PostCount)
	}
	if u, found, err := GetSiteUser(site, 5); err != nil || !found || u.Login != "ed" {
		t.Errorf("GetSiteUser = %+v, %v, %v", u, found, err)
	}
	if _, found, err := GetSiteUser(site, 99); err != nil || found {
		t.Errorf("GetSiteUser of a missing user: found=%v err=%v", found, err)
	}
	if got.ExcludedCounts["subscriber"] != 1200 || got.ExcludedCounts["customer"] != 3 || !got.Truncated {
		t.Errorf("summary = %+v", got)
	}
	if at, found, err := GetSiteUsersObservedAt(site); err != nil || !found || !at.Equal(observed) {
		t.Errorf("observed at = %v, %v, %v", at, found, err)
	}
	summaries, err := GetAllSiteUserSummaries()
	if err != nil || len(summaries) != 1 || summaries[0].UpdatedAt != "2026-10-01T12:00:00Z" {
		t.Errorf("summaries = %+v, %v", summaries, err)
	}

	// A replacement with no users to list still records the counts.
	if err := ReplaceSiteUsers(site, models.SiteUsers{ExcludedCounts: map[string]int{"subscriber": 1}}, observed.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, found, _ = GetSiteUsers(site)
	if !found || len(got.Users) != 0 || got.ExcludedCounts["subscriber"] != 1 || got.Truncated {
		t.Errorf("after replacement = %+v (found=%v)", got, found)
	}

	if err := DeleteSiteUsers(site); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := GetSiteUsers(site); found {
		t.Error("users kept after deletion")
	}
	if all, _ := GetAllSiteUsers(); len(all) != 0 {
		t.Errorf("rows kept after deletion: %+v", all)
	}
}
