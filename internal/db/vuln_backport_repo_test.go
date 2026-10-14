package db

import (
	"errors"
	"testing"

	"github.com/JCO-Digital/jman/internal/models"
)

func TestVulnBackportRoundTrip(t *testing.T) {
	setupTaskRepoTest(t)

	rule := &models.VulnBackport{Type: "plugin", Slug: "woocommerce", VulnUUID: "uuid-1", Branch: "11.1", MinVersion: "11.1.2", Reason: "Backported"}
	if err := CreateVulnBackport(rule, "alice"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if rule.ID == 0 || rule.CreatedBy != "alice" {
		t.Fatalf("create did not populate ID/audit fields: %+v", rule)
	}

	// Same vulnerability, different branch is allowed.
	other := &models.VulnBackport{Type: "plugin", Slug: "woocommerce", VulnUUID: "uuid-1", Branch: "10.9", MinVersion: "10.9.5"}
	if err := CreateVulnBackport(other, "alice"); err != nil {
		t.Fatalf("create second branch: %v", err)
	}

	dup := &models.VulnBackport{Type: "plugin", Slug: "woocommerce", VulnUUID: "uuid-1", Branch: "11.1", MinVersion: "11.1.3"}
	if err := CreateVulnBackport(dup, "bob"); !errors.Is(err, ErrDuplicateVulnBackport) {
		t.Fatalf("duplicate: got %v, want ErrDuplicateVulnBackport", err)
	}

	got, err := GetVulnBackport(rule.ID)
	if err != nil || got == nil || got.MinVersion != "11.1.2" || got.Reason != "Backported" {
		t.Fatalf("get: got %+v, err %v", got, err)
	}

	matcher, err := NewVulnIgnoreMatcher()
	if err != nil {
		t.Fatalf("matcher: %v", err)
	}
	if r := matcher.PatchedBy("uuid-1", "11.1.4"); r == nil || r.ID != rule.ID {
		t.Errorf("PatchedBy(11.1.4) = %+v, want rule %d", r, rule.ID)
	}
	if r := matcher.PatchedBy("uuid-1", "10.9.5"); r == nil || r.ID != other.ID {
		t.Errorf("PatchedBy(10.9.5) = %+v, want rule %d", r, other.ID)
	}
	if r := matcher.PatchedBy("uuid-1", "11.1.1"); r != nil {
		t.Errorf("PatchedBy(11.1.1) = %+v, want nil", r)
	}
	if r := matcher.PatchedBy("uuid-2", "11.1.4"); r != nil {
		t.Errorf("PatchedBy for another UUID = %+v, want nil", r)
	}

	if err := DeleteVulnBackport(rule.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	rules, err := ListVulnBackports()
	if err != nil || len(rules) != 1 || rules[0].ID != other.ID {
		t.Fatalf("list after delete: got %+v, err %v", rules, err)
	}
}
