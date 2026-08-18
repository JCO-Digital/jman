package logs

import (
	"strings"
	"testing"
)

func TestHourAccumulator_TruncatesLongKeys(t *testing.T) {
	longPath := "/" + strings.Repeat("a", 500)

	acc := newHourAccumulator("2026-08-17T10:00:00Z")
	acc.Add(Entry{RemoteAddr: "1.1.1.1", Path: longPath, Referer: longPath}, false, "")

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
	acc.Add(Entry{RemoteAddr: "1.1.1.1", Path: "/", Referer: "https://www.example.com/other-page"}, false, "example.com")
	acc.Add(Entry{RemoteAddr: "2.2.2.2", Path: "/", Referer: "https://google.com/search"}, false, "example.com")

	if _, ok := acc.Referrers["https://www.example.com/other-page"]; ok {
		t.Error("expected same-site referrer to be dropped")
	}
	if _, ok := acc.Referrers["https://google.com/search"]; !ok {
		t.Error("expected external referrer to be kept")
	}
	if acc.RequestsTotal != 2 {
		t.Errorf("RequestsTotal = %d, want 2 (internal referrer only affects the referrer list, not request counts)", acc.RequestsTotal)
	}
}
