package sitestate

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

const site = "11111111-1111-1111-1111-111111111111"

func setup(t *testing.T) {
	t.Helper()
	old := config.RunData.DataDir
	config.RunData.DataDir = t.TempDir()
	if err := db.InitInventory(); err != nil {
		t.Fatal(err)
	}
	if err := db.InitAPI(); err != nil {
		t.Fatal(err)
	}
	suspect = map[string]string{}
	t.Cleanup(func() {
		db.Close()
		config.RunData.DataDir = old
	})
}

func plugin(name, status, version string) models.WPPlugin {
	return models.WPPlugin{Name: name, Status: status, Version: version}
}

// ledger returns the site's detected-change entries, oldest first.
func ledger(t *testing.T) []map[string]any {
	t.Helper()
	entries, err := db.GetSiteUpdateLedger(site)
	if err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if e.Status != models.LedgerDetected {
			continue
		}
		var data map[string]any
		if err := json.Unmarshal([]byte(e.DataJSON), &data); err != nil {
			t.Fatal(err)
		}
		data["update_type"] = e.UpdateType
		data["updated_by"] = e.UpdatedBy
		out = append(out, data)
	}
	return out
}

func apply(t *testing.T, src Source, at time.Time, plugins ...models.WPPlugin) bool {
	t.Helper()
	ok, err := ApplyPlugins(site, plugins, src, at)
	if err != nil {
		t.Fatal(err)
	}
	return ok
}

func TestApplyPluginsDetectsChanges(t *testing.T) {
	setup(t)
	t0 := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

	// The first observation is the baseline, not a change.
	apply(t, SourceSSH, t0,
		plugin("akismet", "active", "5.0"),
		plugin("hello", "inactive", "1.7"),
		plugin("jetpack", "active", "13.0"),
		plugin("woocommerce", "active", "9.3.0"),
		plugin("object-cache.php", "dropin", ""),
		plugin("mu-thing", "must-use", "1.0"),
	)
	if got := ledger(t); len(got) != 0 {
		t.Fatalf("baseline logged %v", got)
	}

	apply(t, SourceAgent, t0.Add(time.Hour),
		plugin("akismet", "inactive", "5.0"),     // deactivated
		plugin("hello", "inactive", "1.7"),       // unchanged
		plugin("jetpack", "active", "12.9"),      // downgraded
		plugin("woocommerce", "active", "9.3.2"), // updated
		plugin("wordfence", "active", "8.0"),     // installed
		plugin("mu-thing", "must-use", "2.0"),    // must-use: ignored
		// object-cache.php removed: drop-in, ignored
	)

	got := ledger(t)
	if len(got) != 1 {
		t.Fatalf("got %d detected entries, want 1: %v", len(got), got)
	}
	entry := got[0]
	if entry["update_type"] != "plugin" || entry["source"] != "agent" || entry["updated_by"] != "detected (agent)" {
		t.Errorf("entry = %v", entry)
	}
	changes := entry["changes"].([]any)
	want := []string{
		"akismet deactivated",
		"jetpack downgraded 13.0→12.9",
		"woocommerce updated 9.3.0→9.3.2",
		"wordfence installed →8.0",
	}
	var gotChanges []string
	for _, c := range changes {
		m := c.(map[string]any)
		s := m["plugin"].(string) + " " + m["change"].(string)
		if m["change"] != "activated" && m["change"] != "deactivated" {
			old, _ := m["old_version"].(string)
			s += " " + old + "→" + m["new_version"].(string)
		}
		gotChanges = append(gotChanges, s)
	}
	if len(gotChanges) != len(want) {
		t.Fatalf("changes = %v, want %v", gotChanges, want)
	}
	for i := range want {
		if gotChanges[i] != want[i] {
			t.Errorf("change %d = %q, want %q", i, gotChanges[i], want[i])
		}
	}
}

func TestApplyPluginsJobChangesAreNotLoggedAgain(t *testing.T) {
	setup(t)
	t0 := time.Now().Add(-time.Hour)
	apply(t, SourceSSH, t0, plugin("akismet", "active", "5.0"))
	apply(t, SourceJob, t0.Add(time.Minute), plugin("akismet", "active", "5.1"))
	// A later read of the same state finds nothing new.
	apply(t, SourceAgent, t0.Add(2*time.Minute), plugin("akismet", "active", "5.1"))
	if got := ledger(t); len(got) != 0 {
		t.Errorf("job changes were logged as detected: %v", got)
	}
}

func TestApplyPluginsSkipsOlderObservations(t *testing.T) {
	setup(t)
	t0 := time.Now().Add(-time.Hour)
	apply(t, SourceJob, t0, plugin("akismet", "active", "5.1"))
	// An agent snapshot taken before the job's refresh arrives late.
	if apply(t, SourceAgent, t0.Add(-10*time.Minute), plugin("akismet", "active", "5.0")) {
		t.Error("an older observation replaced a newer one")
	}
	plugins, _ := db.GetSitePlugins(site)
	if len(plugins) != 1 || plugins[0].Version != "5.1" {
		t.Errorf("stored plugins = %+v", plugins)
	}
	if got := ledger(t); len(got) != 0 {
		t.Errorf("a stale observation was logged: %v", got)
	}
}

func TestApplyPluginsHoldsBackSuspectLists(t *testing.T) {
	setup(t)
	t0 := time.Now().Add(-time.Hour)
	apply(t, SourceSSH, t0,
		plugin("a", "active", "1"), plugin("b", "active", "1"),
		plugin("c", "active", "1"), plugin("d", "active", "1"),
	)

	// An empty list (a failed read) is held back...
	if apply(t, SourceAgent, t0.Add(time.Minute)) {
		t.Error("an empty plugin list was accepted")
	}
	// ...and so is one missing most plugins, the first time.
	partial := []models.WPPlugin{plugin("a", "active", "1")}
	if apply(t, SourceAgent, t0.Add(2*time.Minute), partial...) {
		t.Error("a list missing 3 of 4 plugins was accepted the first time")
	}
	if got := ledger(t); len(got) != 0 {
		t.Fatalf("suspect lists were logged: %v", got)
	}
	// Seen again, it's believed and logged.
	if !apply(t, SourceAgent, t0.Add(3*time.Minute), partial...) {
		t.Fatal("a suspect list seen twice was not accepted")
	}
	got := ledger(t)
	if len(got) != 1 || len(got[0]["changes"].([]any)) != 3 {
		t.Errorf("ledger = %v, want one entry with 3 removals", got)
	}
}

// Losing a plugin or two isn't suspect, and neither is any change on a
// site with only a few plugins.
func TestApplyPluginsAcceptsOrdinaryRemovals(t *testing.T) {
	setup(t)
	t0 := time.Now().Add(-time.Hour)
	apply(t, SourceSSH, t0,
		plugin("a", "active", "1"), plugin("b", "active", "1"),
		plugin("c", "active", "1"), plugin("d", "active", "1"),
	)
	if !apply(t, SourceAgent, t0.Add(time.Minute), plugin("a", "active", "1"), plugin("b", "active", "1")) {
		t.Error("removing half of the plugins was held back")
	}

	setup(t)
	apply(t, SourceSSH, t0, plugin("a", "active", "1"), plugin("b", "active", "1"))
	if !apply(t, SourceAgent, t0.Add(time.Minute), plugin("c", "active", "1")) {
		t.Error("a change on a site with two plugins was held back")
	}
}

func TestApplyCoreLogsOnlyMajorChanges(t *testing.T) {
	setup(t)
	t0 := time.Now().Add(-time.Hour)
	core := func(v string) models.SiteCore { return models.SiteCore{Version: v} }

	for i, step := range []struct {
		version string
		src     Source
	}{
		{"6.6.1", SourceSSH},   // baseline
		{"6.6.2", SourceAgent}, // minor auto-update: not logged
		{"6.7.0", SourceJob},   // jman's own major update: logged by the job
		{"6.8.1", SourceAgent}, // major change from outside jman: logged
	} {
		if _, err := ApplyCore(site, core(step.version), step.src, t0.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}

	got := ledger(t)
	if len(got) != 1 {
		t.Fatalf("got %d detected entries, want 1: %v", len(got), got)
	}
	if got[0]["update_type"] != "core" || got[0]["old_version"] != "6.7.0" || got[0]["new_version"] != "6.8.1" || got[0]["target"] != "major" {
		t.Errorf("entry = %v", got[0])
	}
}

func TestStoredHashMatchesAgentHash(t *testing.T) {
	setup(t)
	if h, _ := StoredHash(site); h != "" {
		t.Errorf("hash with nothing stored = %q, want empty", h)
	}
	plugins := []models.WPPlugin{plugin("b", "active", "2"), plugin("a", "inactive", "1")}
	core := models.SiteCore{Version: "6.6.2", MajorUpdate: "6.7"}
	apply(t, SourceAgent, time.Now(), plugins...)
	if _, err := ApplyCore(site, core, SourceAgent, time.Now()); err != nil {
		t.Fatal(err)
	}
	// The agent hashes its own list, without site IDs and in its order.
	agentPlugins := []models.WPPlugin{plugin("a", "inactive", "1"), plugin("b", "active", "2")}
	if got, want := mustHash(t), models.WPDataHash(agentPlugins, core); got != want {
		t.Errorf("stored hash %s != agent hash %s", got, want)
	}
}

func mustHash(t *testing.T) string {
	t.Helper()
	h, err := StoredHash(site)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestMajorChange(t *testing.T) {
	cases := map[[2]string]bool{
		{"6.6.1", "6.6.2"}:   false,
		{"6.6.2", "6.7"}:     true,
		{"6.6", "6.6.1"}:     false,
		{"6.9.4", "7.0"}:     true,
		{"6.7-RC1", "6.7.0"}: false,
		{"6.6.2", "6.6.2"}:   false,
		{"", "6.6.2"}:        false,
		{"6.7.1", "6.6.2"}:   true, // a downgrade across branches
	}
	for c, want := range cases {
		if got := majorChange(c[0], c[1]); got != want {
			t.Errorf("majorChange(%q, %q) = %v, want %v", c[0], c[1], got, want)
		}
	}
}
