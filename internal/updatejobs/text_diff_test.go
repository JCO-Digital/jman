package updatejobs

import (
	"strings"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/textdiff"
	"github.com/JCO-Digital/jman/internal/wpcli"
)

// fakeSite serves canned pages by URL: each fetch of a URL returns the
// next page in its list (the last one repeats).
type fakeSite struct {
	pages   map[string][]textdiff.Page
	fetches map[string]int
	urls    []string
}

func (f *fakeSite) fetch(pageURL string, _ map[string]string) textdiff.Page {
	f.urls = append(f.urls, pageURL)
	list := f.pages[pageURL]
	if len(list) == 0 {
		return textdiff.Page{Error: "HTTP 404"}
	}
	i := min(f.fetches[pageURL], len(list)-1)
	f.fetches[pageURL]++
	return list[i]
}

func page(lines ...string) textdiff.Page {
	return textdiff.Page{Status: 200, FinalURL: "https://example.com/", Lines: lines}
}

func TestRunComparesTextAroundPluginUpdate(t *testing.T) {
	setupRunnerTest(t)
	site := &fakeSite{fetches: map[string]int{}, pages: map[string][]textdiff.Page{
		"https://example.com/": {
			// Baseline: the quote changes between the two fetches.
			page("Menu", "EN", "Quote one", "Footer note", "Footer"),
			page("Menu", "EN", "Quote two", "Footer note", "Footer"),
			// After the update: the switcher broke, the quote changed
			// again (noise) and the footer note changed (muted).
			page("Menu", `EN" >`, "Quote two", "Footer note 2", "Footer"),
		},
		"https://example.com/en/": {
			page("Hello"),
			page("Hello"),
			{Error: "WordPress error page (HTTP 500)"},
		},
		// /broken/ fails before the update: no baseline.
	}}
	fetchPage = site.fetch
	textDiffPaths = func(string) ([]string, error) { return []string{"/", "/en/", "/broken/"}, nil }
	textDiffMutes = func(string) ([]models.TextDiffMute, error) {
		return []models.TextDiffMute{
			{Path: "/", Direction: models.TextDiffRemoved, Line: "Footer note"},
			{Path: "/", Direction: models.TextDiffAdded, Line: "Footer note #"},
		}, nil
	}

	var updated bool
	updatePlugins = func(models.CliSite, []string, *[]models.WPCLICall) ([]wpcli.UpdateResult, error) {
		// Both baseline fetches of every page happen before the update.
		if n := len(site.urls); n != 6 {
			t.Errorf("%d fetches before the update, want 6", n)
		}
		updated = true
		return []wpcli.UpdateResult{{Name: "polylang", OldVersion: "1.0", NewVersion: "1.1", Status: "Updated"}}, nil
	}

	job := pluginsJob(models.UpdateJobKindPlugins, models.PluginUpdateRequest{Name: "polylang", OldVersion: "1.0"})
	if err := db.CreateUpdateJob(&job); err != nil {
		t.Fatal(err)
	}
	run(job)
	if !updated {
		t.Fatal("plugins were not updated")
	}

	stored, err := db.GetUpdateJob(job.ID)
	if err != nil || stored == nil || stored.TextDiff == nil {
		t.Fatalf("stored job = %+v, %v", stored, err)
	}
	d := stored.TextDiff
	if d.Status != models.TextDiffDone || len(d.Pages) != 3 {
		t.Fatalf("text diff = %+v", d)
	}

	front := d.Pages[0]
	if front.Outcome != models.TextDiffChanged || len(front.Hunks) != 1 || front.Hidden != 4 { // quote removed+added, footer note removed+added
		t.Fatalf("frontpage = %+v", front)
	}
	var changed []string
	for _, l := range front.Hunks[0].Lines {
		if l.Kind != models.TextDiffContext {
			changed = append(changed, l.Kind+":"+l.Text)
		}
	}
	if strings.Join(changed, "|") != `removed:EN|added:EN" >` {
		t.Errorf("frontpage changes = %q, want only the switcher", changed)
	}

	if en := d.Pages[1]; en.Outcome != models.TextDiffError || en.Error != "WordPress error page (HTTP 500)" {
		t.Errorf("/en/ = %+v, want an error after the update", en)
	}
	if br := d.Pages[2]; br.Outcome != models.TextDiffNoBaseline || br.Error != "HTTP 404" {
		t.Errorf("/broken/ = %+v, want no baseline", br)
	}

	want := models.TextDiffSummary{Status: models.TextDiffDone, Pages: 3, Changed: 1, Errors: 1, NoBaseline: 1}
	if stored.TextDiffSummary == nil || *stored.TextDiffSummary != want {
		t.Errorf("summary = %+v, want %+v", stored.TextDiffSummary, want)
	}
}

func TestComparePageRedirectChange(t *testing.T) {
	bp := basePage{path: "/en/", page: page("Hello")}
	bp.page.FinalURL = "https://example.com/en/"
	after := page("Hello")
	p := comparePage(bp, after, nil)
	if p.Outcome != models.TextDiffChanged || p.FinalURLBefore != "https://example.com/en/" || p.FinalURLAfter != "https://example.com/" {
		t.Errorf("page = %+v, want changed by the redirect", p)
	}
	if len(p.Hunks) != 0 {
		t.Errorf("hunks = %+v, want none", p.Hunks)
	}
}

func TestRunSkipsTextDiffForOtherJobs(t *testing.T) {
	setupRunnerTest(t)
	fetches := 0
	fetchPage = func(string, map[string]string) textdiff.Page { fetches++; return page("x") }
	pluginAction = func(models.CliSite, string, []string) error { return nil }

	job := pluginsJob(models.UpdateJobKindDeactivate, models.PluginUpdateRequest{Name: "a"})
	if err := db.CreateUpdateJob(&job); err != nil {
		t.Fatal(err)
	}
	run(job)
	stored, _ := db.GetUpdateJob(job.ID)
	if fetches != 0 || stored.TextDiff != nil {
		t.Errorf("deactivate job fetched %d pages, text diff %+v", fetches, stored.TextDiff)
	}
}

func TestStartMarksRunningTextDiffInterrupted(t *testing.T) {
	setupRunnerTest(t)
	job := pluginsJob(models.UpdateJobKindPlugins, models.PluginUpdateRequest{Name: "a"})
	if err := db.CreateUpdateJob(&job); err != nil {
		t.Fatal(err)
	}
	if err := db.FinishUpdateJob(job.ID, models.UpdateJobDone, nil, nil, ""); err != nil {
		t.Fatal(err)
	}
	if err := db.SetUpdateJobTextDiff(job.ID, models.TextDiff{Status: models.TextDiffRunning, Pages: []models.TextDiffPage{{Path: "/"}}}); err != nil {
		t.Fatal(err)
	}
	if err := db.InterruptRunningTextDiffs(); err != nil {
		t.Fatal(err)
	}
	stored, _ := db.GetUpdateJob(job.ID)
	if stored.TextDiff == nil || stored.TextDiff.Status != models.TextDiffInterrupted || len(stored.TextDiff.Pages) != 1 {
		t.Errorf("text diff = %+v, want interrupted with its pages kept", stored.TextDiff)
	}
}

func TestValidTextDiffPath(t *testing.T) {
	ok := []string{"/", "/en/", "/shop/cart?x=1", " /fi/ "}
	bad := []string{"", "en/", "//evil.com/", "https://evil.com/", "/a b", "/a#b", `/a\b`, "/" + strings.Repeat("a", 600)}
	for _, p := range ok {
		if _, valid := ValidTextDiffPath(p); !valid {
			t.Errorf("ValidTextDiffPath(%q) = invalid", p)
		}
	}
	for _, p := range bad {
		if _, valid := ValidTextDiffPath(p); valid {
			t.Errorf("ValidTextDiffPath(%q) = valid", p)
		}
	}
}

func TestTextDiffMuteExpiry(t *testing.T) {
	setupRunnerTest(t)
	now := time.Now()
	m := models.TextDiffMute{SiteID: testSite, Path: "/", Direction: models.TextDiffAdded, Line: "x", ExpiresAt: now.Add(time.Hour), AuditFields: models.AuditFields{CreatedBy: "alice"}}
	if err := db.InsertTextDiffMute(&m); err != nil {
		t.Fatal(err)
	}
	if active, _ := db.ListActiveTextDiffMutes(testSite, now); len(active) != 1 {
		t.Fatalf("active mutes = %+v", active)
	}
	if active, _ := db.ListActiveTextDiffMutes(testSite, now.Add(2*time.Hour)); len(active) != 0 {
		t.Errorf("mute still active after it expired: %+v", active)
	}
	if err := db.ExpireTextDiffMute(m.ID, "bob"); err != nil {
		t.Fatal(err)
	}
	if active, _ := db.ListActiveTextDiffMutes(testSite, time.Now()); len(active) != 0 {
		t.Errorf("removed mute still active: %+v", active)
	}
	all, _ := db.ListTextDiffMutes(testSite)
	if len(all) != 1 || all[0].UpdatedBy != "bob" {
		t.Errorf("mutes = %+v, want the removed one kept", all)
	}
}

func TestTextDiffQuery(t *testing.T) {
	setupRunnerTest(t)
	cases := []struct {
		setting string
		path    string
		want    string
	}{
		{"", "/", "https://example.com/"},
		{"bojaco_staging_bypass=1", "/", "https://example.com/?bojaco_staging_bypass=1"},
		{"?bojaco_staging_bypass=1", "/en/", "https://example.com/en/?bojaco_staging_bypass=1"},
		{"a=1&b=2", "/shop/?orderby=price", "https://example.com/shop/?orderby=price&a=1&b=2"},
		{"bad value#x", "/", "https://example.com/"},
		{"%zz", "/", "https://example.com/"},
	}
	for _, c := range cases {
		textDiffQuery = func() string { return c.setting }
		b := &textBaseline{domain: "example.com", query: extraQuery(1)}
		if got := b.url(c.path); got != c.want {
			t.Errorf("setting %q, path %q: url = %q, want %q", c.setting, c.path, got, c.want)
		}
	}

	// The fetches of a job use it.
	textDiffQuery = func() string { return "bojaco_staging_bypass=1" }
	site := &fakeSite{fetches: map[string]int{}, pages: map[string][]textdiff.Page{
		"https://example.com/?bojaco_staging_bypass=1": {page("Hello")},
	}}
	fetchPage = site.fetch
	updatePlugins = func(models.CliSite, []string, *[]models.WPCLICall) ([]wpcli.UpdateResult, error) { return nil, nil }
	job := pluginsJob(models.UpdateJobKindPlugins, models.PluginUpdateRequest{Name: "a"})
	if err := db.CreateUpdateJob(&job); err != nil {
		t.Fatal(err)
	}
	run(job)
	stored, _ := db.GetUpdateJob(job.ID)
	if len(site.urls) != 3 || stored.TextDiff == nil || stored.TextDiff.Pages[0].Outcome != models.TextDiffUnchanged {
		t.Errorf("fetched %v, diff %+v; want three fetches with the query and an unchanged page", site.urls, stored.TextDiff)
	}
}
