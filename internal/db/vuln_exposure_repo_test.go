package db

import (
	"reflect"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

func TestVulnExposureRepo(t *testing.T) {
	setupTaskRepoTest(t)

	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	cvss := 7.5
	e := models.VulnExposure{
		Kind: models.VulnExposurePlugin, Slug: "gigpress", SiteID: "site-1", Version: "2.3.28",
		VulnUUIDs: []string{"a", "b"}, MaxCvss: &cvss, FixState: models.VulnFixAvailable, FixedIn: "2.3.29",
		FixAvailableSince: &now, FirstSeenAt: now, LastSeenAt: now,
	}
	if err := InsertVulnExposure(&e); err != nil {
		t.Fatal(err)
	}

	open, err := ListOpenVulnExposures()
	if err != nil || len(open) != 1 {
		t.Fatalf("open = %+v, %v", open, err)
	}
	got := open[0]
	if got.ID != e.ID || !reflect.DeepEqual(got.VulnUUIDs, e.VulnUUIDs) || *got.MaxCvss != cvss ||
		got.FixedIn != "2.3.29" || !got.FixAvailableSince.Equal(now) || !got.FirstSeenAt.Equal(now) || got.ResolvedAt != nil {
		t.Errorf("read back %+v, want %+v", got, e)
	}

	// A second open exposure for the same plugin and site is rejected.
	dup := e
	if err := InsertVulnExposure(&dup); err == nil {
		t.Error("duplicate open exposure was inserted")
	}

	got.FixState, got.FixedIn, got.FixAvailableSince, got.MaxCvss = models.VulnNoFix, "", nil, nil
	got.PossibleFixVersion = "2.4.0"
	if err := UpdateVulnExposure(got); err != nil {
		t.Fatal(err)
	}
	if err := ResolveVulnExposures([]int64{got.ID}, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if open, _ := ListOpenVulnExposures(); len(open) != 0 {
		t.Errorf("still open after resolve: %+v", open)
	}

	resolved, err := ListVulnExposuresResolvedSince(now)
	if err != nil || len(resolved) != 1 {
		t.Fatalf("resolved = %+v, %v", resolved, err)
	}
	r := resolved[0]
	if r.FixState != models.VulnNoFix || r.MaxCvss != nil || r.FixAvailableSince != nil || r.PossibleFixVersion != "2.4.0" ||
		r.ResolvedAt == nil || !r.ResolvedAt.Equal(now.Add(time.Hour)) {
		t.Errorf("resolved exposure = %+v", r)
	}
	if later, _ := ListVulnExposuresResolvedSince(now.Add(2 * time.Hour)); len(later) != 0 {
		t.Errorf("resolved since a later time = %+v, want none", later)
	}

	// Once resolved, the same plugin and site can be opened again.
	again := e
	again.ID = 0
	if err := InsertVulnExposure(&again); err != nil {
		t.Errorf("reinsert after resolve: %v", err)
	}
}
