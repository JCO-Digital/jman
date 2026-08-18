package logs

import (
	"strings"
	"testing"
)

func TestHourAccumulator_TruncatesLongKeys(t *testing.T) {
	longPath := "/" + strings.Repeat("a", 500)

	acc := newHourAccumulator("2026-08-17T10:00:00Z")
	acc.Add(Entry{RemoteAddr: "1.1.1.1", Path: longPath, Referer: longPath}, false)

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
