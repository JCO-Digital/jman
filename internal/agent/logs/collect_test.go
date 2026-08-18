package logs

import (
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeGzip(t *testing.T, path, content string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	if _, err := gz.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
}

const line1 = `1.1.1.1 - - [17/Aug/2026:10:00:00 +0000] "GET / HTTP/2.0" 200 100 "-" "Mozilla/5.0"` + "\n"
const line2 = `2.2.2.2 - - [17/Aug/2026:10:05:00 +0000] "GET /about HTTP/2.0" 200 100 "-" "Mozilla/5.0"` + "\n"
const botLine = `3.3.3.3 - - [17/Aug/2026:10:06:00 +0000] "GET / HTTP/2.0" 200 100 "-" "SomeCrawlerBot/1.0"` + "\n"
const internalLine = `4.4.4.4 - - [17/Aug/2026:10:07:00 +0000] "GET /?jman_cache_bypass=1 HTTP/2.0" 200 0 "-" "jman/1.0 (WordPress Management Tool)"` + "\n"

func TestCollect_RotatedFileFinalizesImmediately(t *testing.T) {
	dir := t.TempDir()
	writeGzip(t, filepath.Join(dir, "access.log-20260816.gz"), line1+line2+botLine+internalLine)
	if err := os.WriteFile(filepath.Join(dir, "access.log"), nil, 0644); err != nil {
		t.Fatal(err)
	}

	state := &FileState{ProcessedRotated: map[string]bool{}}
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)

	finalized, err := Collect(dir, state, now)
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if len(finalized) != 1 {
		t.Fatalf("expected 1 finalized hour, got %d", len(finalized))
	}

	hour := finalized[0]
	if hour.RequestsTotal != 3 {
		t.Errorf("RequestsTotal = %d, want 3 (excluding internal jman traffic)", hour.RequestsTotal)
	}
	if hour.RequestsBot != 1 {
		t.Errorf("RequestsBot = %d, want 1", hour.RequestsBot)
	}
	if hour.RequestsHuman != 2 {
		t.Errorf("RequestsHuman = %d, want 2", hour.RequestsHuman)
	}
	if hour.UniqueVisitors != 3 {
		t.Errorf("UniqueVisitors = %d, want 3", hour.UniqueVisitors)
	}
	if !state.ProcessedRotated["access.log-20260816.gz"] {
		t.Error("expected rotated file to be marked processed")
	}
}

func TestCollect_LiveFileIncrementalTail(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "access.log")
	if err := os.WriteFile(logPath, []byte(line1), 0644); err != nil {
		t.Fatal(err)
	}

	state := &FileState{ProcessedRotated: map[string]bool{}}
	stillWithinHour := time.Date(2026, 8, 17, 10, 30, 0, 0, time.UTC)

	// First cycle: one line, hour not yet elapsed (now is still within it) -> nothing finalized, pending set.
	finalized, err := Collect(dir, state, stillWithinHour)
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if len(finalized) != 0 {
		t.Fatalf("expected no finalized hours yet, got %d", len(finalized))
	}
	if state.Pending == nil || state.Pending.RequestsTotal != 1 {
		t.Fatalf("expected pending hour with 1 request, got %+v", state.Pending)
	}
	offsetAfterFirst := state.Offset

	// Append a second line without touching the first (simulates the live server appending).
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(line2); err != nil {
		t.Fatal(err)
	}
	f.Close()

	// Second cycle, now well past the hour -> should finalize with both lines counted.
	pastHour := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	finalized, err = Collect(dir, state, pastHour)
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if state.Offset <= offsetAfterFirst {
		t.Errorf("expected offset to advance past %d, got %d", offsetAfterFirst, state.Offset)
	}
	if len(finalized) != 1 {
		t.Fatalf("expected 1 finalized hour, got %d", len(finalized))
	}
	if finalized[0].RequestsTotal != 2 {
		t.Errorf("RequestsTotal = %d, want 2 (accumulated across cycles)", finalized[0].RequestsTotal)
	}
	if state.Pending != nil {
		t.Errorf("expected pending to be cleared after finalization")
	}
}

func TestCollect_PartialTrailingLineNotConsumed(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "access.log")
	partial := `5.5.5.5 - - [17/Aug/2026:10:00:00 +0000] "GET /still-writing`
	if err := os.WriteFile(logPath, []byte(line1+partial), 0644); err != nil {
		t.Fatal(err)
	}

	state := &FileState{ProcessedRotated: map[string]bool{}}
	now := time.Date(2026, 8, 17, 10, 1, 0, 0, time.UTC)

	if _, err := Collect(dir, state, now); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	if int(state.Offset) != len(line1) {
		t.Errorf("offset = %d, want %d (partial trailing line must not be consumed)", state.Offset, len(line1))
	}
}
