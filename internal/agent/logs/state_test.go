package logs

import (
	"strings"
	"testing"
)

func TestHourAccumulator_TruncatesLongKeys(t *testing.T) {
	longPath := "/" + strings.Repeat("a", 500)
	// No "/", "?", or "#" in this one, so normalizeReferrerHost can't cut it
	// down to a short host — it stays long and must be truncated same as a
	// page key would be.
	longHost := strings.Repeat("b", 500)

	acc := newHourAccumulator("2026-08-17T10:00:00Z")
	acc.Add(Entry{RemoteAddr: "1.1.1.1", Path: longPath, Status: 200, Referer: longHost}, false, "")

	for key := range acc.Pages {
		if len(key) > maxKeyLength {
			t.Errorf("page key length = %d, want <= %d", len(key), maxKeyLength)
		}
	}
	for key := range acc.Referrers {
		if len(key) > maxKeyLength {
			t.Errorf("referrer key length = %d, want <= %d", len(key), maxKeyLength)
		}
	}
}

func TestHourAccumulator_DropsInternalReferrers(t *testing.T) {
	acc := newHourAccumulator("2026-08-17T10:00:00Z")
	acc.Add(Entry{RemoteAddr: "1.1.1.1", Path: "/", Status: 200, Referer: "https://www.example.com/other-page"}, false, "example.com")
	acc.Add(Entry{RemoteAddr: "2.2.2.2", Path: "/", Status: 200, Referer: "https://google.com/search"}, false, "example.com")

	if _, ok := acc.Referrers["example.com"]; ok {
		t.Error("expected same-site referrer to be dropped")
	}
	if _, ok := acc.Referrers["google.com"]; !ok {
		t.Error("expected external referrer to be kept, normalized to its bare host")
	}
	if acc.RequestsTotal != 2 {
		t.Errorf("RequestsTotal = %d, want 2 (internal referrer only affects the referrer list, not request counts)", acc.RequestsTotal)
	}
}

func TestHourAccumulator_NormalizesReferrerVariants(t *testing.T) {
	acc := newHourAccumulator("2026-08-17T10:00:00Z")
	acc.Add(Entry{RemoteAddr: "1.1.1.1", Path: "/", Status: 200, Referer: "https://google.com/search?q=a"}, false, "example.com")
	acc.Add(Entry{RemoteAddr: "2.2.2.2", Path: "/", Status: 200, Referer: "http://google.com"}, false, "example.com")
	acc.Add(Entry{RemoteAddr: "3.3.3.3", Path: "/", Status: 200, Referer: "www.google.com"}, false, "example.com")

	if len(acc.Referrers) != 1 {
		t.Fatalf("expected all 3 referrer variants to merge into 1 entry, got %d: %+v", len(acc.Referrers), acc.Referrers)
	}
	if acc.Referrers["google.com"] != 3 {
		t.Errorf("Referrers[\"google.com\"] = %d, want 3", acc.Referrers["google.com"])
	}
}

func TestHourAccumulator_DropsExcludedPages(t *testing.T) {
	acc := newHourAccumulator("2026-08-17T10:00:00Z")
	acc.Add(Entry{RemoteAddr: "1.1.1.1", Path: "/wp-admin/edit.php?post=1", Status: 200}, false, "")
	acc.Add(Entry{RemoteAddr: "2.2.2.2", Path: "/wp-json/wp/v2/posts", Status: 200}, false, "")
	acc.Add(Entry{RemoteAddr: "4.4.4.4", Path: "/wp-content/uploads/x.jpg", Status: 200}, false, "")
	acc.Add(Entry{RemoteAddr: "3.3.3.3", Path: "/about", Status: 200}, false, "")

	if _, ok := acc.Pages["/wp-admin/edit.php"]; ok {
		t.Error("expected /wp-admin page to be excluded from top pages")
	}
	if _, ok := acc.Pages["/wp-json/wp/v2/posts"]; ok {
		t.Error("expected /wp-json page to be excluded from top pages")
	}
	if _, ok := acc.Pages["/wp-content/uploads/x.jpg"]; ok {
		t.Error("expected /wp-content page to be excluded from top pages")
	}
	if _, ok := acc.Pages["/about"]; !ok {
		t.Error("expected regular page to be kept")
	}
	if acc.RequestsTotal != 4 {
		t.Errorf("RequestsTotal = %d, want 4 (excluded pages only affect the top-pages list, not request counts)", acc.RequestsTotal)
	}
}

func TestHourAccumulator_OnlyCountsStatus200InTopPages(t *testing.T) {
	acc := newHourAccumulator("2026-08-17T10:00:00Z")
	acc.Add(Entry{RemoteAddr: "1.1.1.1", Path: "/about", Status: 200}, false, "")
	acc.Add(Entry{RemoteAddr: "2.2.2.2", Path: "/missing", Status: 404}, false, "")
	acc.Add(Entry{RemoteAddr: "3.3.3.3", Path: "/moved", Status: 301}, false, "")

	if _, ok := acc.Pages["/about"]; !ok {
		t.Error("expected the 200 page to be kept")
	}
	if _, ok := acc.Pages["/missing"]; ok {
		t.Error("expected the 404 page to be excluded from top pages")
	}
	if _, ok := acc.Pages["/moved"]; ok {
		t.Error("expected the 301 page to be excluded from top pages")
	}
	if acc.RequestsTotal != 3 {
		t.Errorf("RequestsTotal = %d, want 3 (status filtering only affects the top-pages list, not request counts)", acc.RequestsTotal)
	}
}

func TestHourAccumulator_TracksStatusCodesForEveryRequest(t *testing.T) {
	acc := newHourAccumulator("2026-08-17T10:00:00Z")
	acc.Add(Entry{RemoteAddr: "1.1.1.1", Path: "/about", Status: 200}, false, "")
	acc.Add(Entry{RemoteAddr: "2.2.2.2", Path: "/other", Status: 200}, false, "")
	acc.Add(Entry{RemoteAddr: "3.3.3.3", Path: "/missing", Status: 404}, false, "")
	acc.Add(Entry{RemoteAddr: "4.4.4.4", Path: "/wp-admin/edit.php", Status: 200}, false, "") // excluded from Pages, must still count here

	if acc.StatusCodes["200"] != 3 {
		t.Errorf("StatusCodes[200] = %d, want 3 (includes the excluded /wp-admin hit)", acc.StatusCodes["200"])
	}
	if acc.StatusCodes["404"] != 1 {
		t.Errorf("StatusCodes[404] = %d, want 1", acc.StatusCodes["404"])
	}
}
