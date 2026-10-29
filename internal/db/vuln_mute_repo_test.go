package db

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

func TestVulnMuteRepo(t *testing.T) {
	setupTaskRepoTest(t)

	review := time.Date(2027, 1, 27, 12, 0, 0, 0, time.UTC)
	m := models.VulnMute{
		Kind: models.VulnExposurePlugin, Slug: "gigpress", SiteID: "site-1", Reason: "Client keeps it",
		Status: models.VulnMuteActive, FixStateAtMute: models.VulnNoFix, VulnUUIDsAtMute: []string{"a"},
		PossibleFixAtMute: "2.4.0", ReviewAt: review, AuditFields: models.AuditFields{CreatedBy: "niklas"},
	}
	if err := InsertVulnMute(&m); err != nil {
		t.Fatal(err)
	}

	dup := m
	if err := InsertVulnMute(&dup); !errors.Is(err, ErrVulnMuteExists) {
		t.Errorf("duplicate open mute: err = %v, want ErrVulnMuteExists", err)
	}

	got, err := GetVulnMute(m.ID)
	if err != nil || got == nil {
		t.Fatalf("get: %+v, %v", got, err)
	}
	if got.Reason != m.Reason || !reflect.DeepEqual(got.VulnUUIDsAtMute, []string{"a"}) || got.PossibleFixAtMute != "2.4.0" ||
		!got.ReviewAt.Equal(review) || got.CreatedBy != "niklas" || got.UpdatedBy != "niklas" || got.LiftedAt != nil {
		t.Errorf("read back %+v", got)
	}

	open, err := ListOpenVulnMutes()
	if err != nil || len(open) != 1 {
		t.Fatalf("open = %+v, %v", open, err)
	}

	now := time.Now()
	got.Status, got.LiftedAt, got.LiftedReason, got.UpdatedBy = models.VulnMuteLifted, &now, "No longer affected", "system"
	if err := UpdateVulnMute(got); err != nil {
		t.Fatal(err)
	}
	if open, _ := ListOpenVulnMutes(); len(open) != 0 {
		t.Errorf("open after lift = %+v", open)
	}
	lifted, err := ListVulnMutesLiftedSince(now.Add(-time.Minute))
	if err != nil || len(lifted) != 1 || lifted[0].LiftedReason != "No longer affected" || lifted[0].UpdatedBy != "system" {
		t.Errorf("lifted = %+v, %v", lifted, err)
	}

	// Once lifted, the site's plugin can be muted again.
	again := m
	again.ID = 0
	if err := InsertVulnMute(&again); err != nil {
		t.Errorf("mute after lift: %v", err)
	}
}
