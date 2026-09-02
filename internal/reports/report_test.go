package reports

import (
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
)

// setupReportsTest gives each test its own on-disk sqlite DB, mirroring
// internal/db's own test setup (that helper is unexported to package db, so
// reports tests need their own copy).
func setupReportsTest(t *testing.T) {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "jman-reports-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	oldDataDir := config.RunData.DataDir
	config.RunData.DataDir = tempDir

	if err := db.InitInventory(); err != nil {
		t.Fatalf("failed to init inventory DB: %v", err)
	}
	if err := db.InitAPI(); err != nil {
		t.Fatalf("failed to init api DB: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
		os.RemoveAll(tempDir)
		config.RunData.DataDir = oldDataDir
	})
}

func TestParseDateRange(t *testing.T) {
	q := url.Values{"start": {"2026-01-01"}, "end": {"2026-01-31"}}
	start, end, err := ParseDateRange(q, 366)
	if err != nil {
		t.Fatalf("ParseDateRange() error = %v", err)
	}
	if start != "2026-01-01" || end != "2026-01-31" {
		t.Errorf("ParseDateRange() = (%q, %q), want (2026-01-01, 2026-01-31)", start, end)
	}
}

func TestParseDateRange_DefaultsWhenAbsent(t *testing.T) {
	start, end, err := ParseDateRange(url.Values{}, 366)
	if err != nil {
		t.Fatalf("ParseDateRange() error = %v", err)
	}
	if start == "" || end == "" {
		t.Fatalf("ParseDateRange() with no params should default, got start=%q end=%q", start, end)
	}
	startT, _ := time.Parse("2006-01-02", start)
	endT, _ := time.Parse("2006-01-02", end)
	if got := endT.Sub(startT).Hours() / 24; got != defaultDateRangeDays {
		t.Errorf("default range span = %v days, want %d", got, defaultDateRangeDays)
	}
}

func TestParseDateRange_RejectsInvalidDate(t *testing.T) {
	q := url.Values{"start": {"not-a-date"}}
	if _, _, err := ParseDateRange(q, 366); err == nil {
		t.Error("ParseDateRange() with invalid start date should error")
	}
}

func TestParseDateRange_RejectsStartAfterEnd(t *testing.T) {
	q := url.Values{"start": {"2026-02-01"}, "end": {"2026-01-01"}}
	if _, _, err := ParseDateRange(q, 366); err == nil {
		t.Error("ParseDateRange() with start after end should error")
	}
}

func TestParseDateRange_RejectsRangeTooLarge(t *testing.T) {
	q := url.Values{"start": {"2026-01-01"}, "end": {"2027-01-01"}}
	if _, _, err := ParseDateRange(q, 30); err == nil {
		t.Error("ParseDateRange() spanning more than maxRangeDays should error")
	}
}

func TestRegistryContainsBuiltInReports(t *testing.T) {
	if _, ok := Get("traffic"); !ok {
		t.Error(`expected "traffic" report to be registered`)
	}
	if _, ok := Get("asset-billing"); !ok {
		t.Error(`expected "asset-billing" report to be registered`)
	}
	if _, ok := Get("does-not-exist"); ok {
		t.Error("Get() should report unknown IDs as not found")
	}
}

func TestAllMeta_SortedByID(t *testing.T) {
	setupReportsTest(t)

	meta := AllMeta()
	for i := 1; i < len(meta); i++ {
		if meta[i-1].ID > meta[i].ID {
			t.Errorf("AllMeta() not sorted by ID: %q came before %q", meta[i-1].ID, meta[i].ID)
		}
	}
}
