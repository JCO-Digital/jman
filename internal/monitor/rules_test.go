package monitor

import (
	"testing"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

func TestNormalizeRule(t *testing.T) {
	r := models.MonitorCheckRule{MatchType: models.MonitorRuleMatchHost, Value: " *.Staging.* "}
	if err := NormalizeRule(&r); err != nil || r.Value != "*.staging.*" || r.Action != models.MonitorRuleActionStartPaused {
		t.Fatalf("NormalizeRule = %v, %+v", err, r)
	}
	for _, bad := range []models.MonitorCheckRule{
		{MatchType: models.MonitorRuleMatchHost, Value: ""},
		{MatchType: models.MonitorRuleMatchHost, Value: "[staging"},
		{MatchType: models.MonitorRuleMatchHost, Value: "staging/x"},
		{MatchType: "site", Value: "x"},
		{MatchType: models.MonitorRuleMatchServer, Value: "x", Action: "mute"},
	} {
		if err := NormalizeRule(&bad); err == nil {
			t.Errorf("NormalizeRule(%+v) accepted", bad)
		}
	}
}

func TestRuleMatches(t *testing.T) {
	site := monitoredSite{domain: "Shop.Staging.example.fi", serverID: "srv-1"}
	cases := []struct {
		rule models.MonitorCheckRule
		want bool
	}{
		{models.MonitorCheckRule{MatchType: models.MonitorRuleMatchServer, Value: "srv-1"}, true},
		{models.MonitorCheckRule{MatchType: models.MonitorRuleMatchServer, Value: "srv-2"}, false},
		{models.MonitorCheckRule{MatchType: models.MonitorRuleMatchHost, Value: "*.staging.*"}, true},
		{models.MonitorCheckRule{MatchType: models.MonitorRuleMatchHost, Value: "staging.*"}, false},
		{models.MonitorCheckRule{MatchType: models.MonitorRuleMatchHost, Value: "*.example.fi"}, true},
	}
	for _, c := range cases {
		if got := ruleMatches(c.rule, site); got != c.want {
			t.Errorf("ruleMatches(%s %s) = %v, want %v", c.rule.MatchType, c.rule.Value, got, c.want)
		}
	}
	if ruleMatches(models.MonitorCheckRule{MatchType: models.MonitorRuleMatchServer, Value: ""}, monitoredSite{domain: "x"}) {
		t.Error("an empty server rule matched a site without a server")
	}
}

func TestReconcileStartsMatchingChecksPaused(t *testing.T) {
	setupEngineTest(t)

	rule := models.MonitorCheckRule{MatchType: models.MonitorRuleMatchServer, Value: "srv-staging"}
	if err := db.CreateMonitorCheckRule(&rule, "tester"); err != nil {
		t.Fatal(err)
	}
	existing := createCheck(t, models.MonitorCheck{SiteID: "site-old", IsDefault: true, Enabled: true})
	sites := map[string]monitoredSite{
		"site-old":  {domain: "old.example", serverID: "srv-staging", active: true},
		"site-new":  {domain: "new.example", serverID: "srv-staging", active: true},
		"site-prod": {domain: "prod.example", serverID: "srv-prod", active: true},
	}

	checks, err := db.ListMonitorChecks()
	if err != nil {
		t.Fatal(err)
	}
	reconcileDefaultChecks(checks, sites, true, []models.MonitorCheckRule{rule})

	stored, err := db.ListMonitorChecks()
	if err != nil {
		t.Fatal(err)
	}
	bySite := map[string]models.MonitorCheck{}
	for _, c := range stored {
		bySite[c.SiteID] = c
	}
	if c := bySite["site-new"]; c.Enabled || c.PausedByRuleID != rule.ID {
		t.Errorf("new staging check = enabled %v, paused by %q; want paused by %s", c.Enabled, c.PausedByRuleID, rule.ID)
	}
	if c := bySite["site-prod"]; !c.Enabled || c.PausedByRuleID != "" {
		t.Errorf("new prod check = enabled %v, paused by %q; want running", c.Enabled, c.PausedByRuleID)
	}
	if c := bySite["site-old"]; c.ID != existing.ID || !c.Enabled {
		t.Errorf("existing staging check changed: %+v", c)
	}

	// Resuming clears the rule; deleting the rule detaches what it paused.
	newID := bySite["site-new"].ID
	if err := db.SetMonitorCheckEnabled(newID, true, "tester"); err != nil {
		t.Fatal(err)
	}
	if c, _ := db.GetMonitorCheck(newID); !c.Enabled || c.PausedByRuleID != "" {
		t.Errorf("resumed check = %+v", c)
	}
	if err := db.PauseMonitorCheckByRule(newID, rule.ID, "tester"); err != nil {
		t.Fatal(err)
	}
	if found, err := db.DeleteMonitorCheckRule(rule.ID); err != nil || !found {
		t.Fatalf("DeleteMonitorCheckRule = %v, %v", found, err)
	}
	if c, _ := db.GetMonitorCheck(newID); c.Enabled || c.PausedByRuleID != "" {
		t.Errorf("check after its rule was deleted = enabled %v, paused by %q; want paused, no rule", c.Enabled, c.PausedByRuleID)
	}
}
