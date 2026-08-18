package logs

import (
	"compress/gzip"
	"math"
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

	finalized, _, err := Collect(dir, state, now, math.MaxInt)
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
	finalized, _, err := Collect(dir, state, stillWithinHour, math.MaxInt)
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
	finalized, _, err = Collect(dir, state, pastHour, math.MaxInt)
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

func TestCollect_BudgetDefersRemainingBacklog(t *testing.T) {
	dir := t.TempDir()
	// Three separate rotated days, one hour of traffic each.
	writeGzip(t, filepath.Join(dir, "access.log-20260814.gz"), `1.1.1.1 - - [14/Aug/2026:10:00:00 +0000] "GET / HTTP/2.0" 200 100 "-" "Mozilla/5.0"`+"\n")
	writeGzip(t, filepath.Join(dir, "access.log-20260815.gz"), `1.1.1.1 - - [15/Aug/2026:10:00:00 +0000] "GET / HTTP/2.0" 200 100 "-" "Mozilla/5.0"`+"\n")
	writeGzip(t, filepath.Join(dir, "access.log-20260816.gz"), `1.1.1.1 - - [16/Aug/2026:10:00:00 +0000] "GET / HTTP/2.0" 200 100 "-" "Mozilla/5.0"`+"\n")
	if err := os.WriteFile(filepath.Join(dir, "access.log"), nil, 0644); err != nil {
		t.Fatal(err)
	}

	state := &FileState{ProcessedRotated: map[string]bool{}}
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)

	// Budget of 2: only two of the three backlogged days should be
	// processed and marked, leaving the third for a later cycle.
	finalized, hasMore, err := Collect(dir, state, now, 2)
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if len(finalized) != 2 {
		t.Fatalf("expected 2 finalized hours (budget-capped), got %d", len(finalized))
	}
	if !hasMore {
		t.Error("expected hasMore=true with a rotated day still deferred")
	}
	processedCount := 0
	for _, name := range []string{"access.log-20260814.gz", "access.log-20260815.gz", "access.log-20260816.gz"} {
		if state.ProcessedRotated[name] {
			processedCount++
		}
	}
	if processedCount != 2 {
		t.Fatalf("expected exactly 2 rotated files marked processed, got %d", processedCount)
	}

	// A later cycle with no cap should pick up the deferred day, and report
	// no more backlog remaining.
	finalized, hasMore, err = Collect(dir, state, now, math.MaxInt)
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if len(finalized) != 1 {
		t.Fatalf("expected the remaining 1 backlogged hour on the follow-up cycle, got %d", len(finalized))
	}
	if hasMore {
		t.Error("expected hasMore=false once the backlog is fully drained")
	}
}

// TestCollect_LiveFileBudgetCap reproduces the scenario that actually broke
// in production: on the very first run (fresh state, offset 0), the live
// access.log already contains many hours' worth of *today's* traffic that
// elapsed before the agent started collecting (rotation only happens once
// a day). Without a budget on live-file processing too, all of it would be
// finalized and sent in one oversized report.
func TestCollect_LiveFileBudgetCap(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "access.log")
	hour08 := `1.1.1.1 - - [17/Aug/2026:08:00:00 +0000] "GET / HTTP/2.0" 200 100 "-" "Mozilla/5.0"` + "\n"
	hour09 := `1.1.1.1 - - [17/Aug/2026:09:00:00 +0000] "GET / HTTP/2.0" 200 100 "-" "Mozilla/5.0"` + "\n"
	hour10 := `1.1.1.1 - - [17/Aug/2026:10:00:00 +0000] "GET / HTTP/2.0" 200 100 "-" "Mozilla/5.0"` + "\n"
	if err := os.WriteFile(logPath, []byte(hour08+hour09+hour10), 0644); err != nil {
		t.Fatal(err)
	}

	state := &FileState{ProcessedRotated: map[string]bool{}}
	now := time.Date(2026, 8, 17, 14, 0, 0, 0, time.UTC) // well past all three hours

	finalized, hasMore, err := Collect(dir, state, now, 2)
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if len(finalized) != 2 {
		t.Fatalf("expected exactly 2 finalized hours (budget-capped), got %d", len(finalized))
	}
	if !hasMore {
		t.Error("expected hasMore=true with the live file's third hour still unfinalized")
	}
	// The third hour's line was already read (a line can't be "un-read"
	// once processed) so it's sitting as the new pending accumulator —
	// just not finalized/sent yet, which is what the budget actually guards.
	if state.Pending == nil || state.Pending.Hour != "2026-08-17T10:00:00Z" || state.Pending.RequestsTotal != 1 {
		t.Fatalf("expected hour 10 to be pending (read but not finalized), got %+v", state.Pending)
	}

	// A follow-up cycle with no cap should finalize that pending hour (the
	// wall clock is still past it) without re-reading or double-counting it.
	finalized, hasMore, err = Collect(dir, state, now, math.MaxInt)
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if len(finalized) != 1 {
		t.Fatalf("expected the remaining 1 hour on the follow-up cycle, got %d", len(finalized))
	}
	if finalized[0].RequestsTotal != 1 {
		t.Errorf("RequestsTotal = %d, want 1 (no duplication of the already-read line)", finalized[0].RequestsTotal)
	}
	if hasMore {
		t.Error("expected hasMore=false once the pending hour is finalized")
	}
	if state.Pending != nil {
		t.Errorf("expected pending to be cleared after finalization, got %+v", state.Pending)
	}
}

func TestCollect_PrioritizesRecentRotatedDayOverOldBacklog(t *testing.T) {
	dir := t.TempDir()
	writeGzip(t, filepath.Join(dir, "access.log-20260601.gz"), `1.1.1.1 - - [01/Jun/2026:10:00:00 +0000] "GET / HTTP/2.0" 200 100 "-" "Mozilla/5.0"`+"\n")
	writeGzip(t, filepath.Join(dir, "access.log-20260817.gz"), `1.1.1.1 - - [17/Aug/2026:10:00:00 +0000] "GET / HTTP/2.0" 200 100 "-" "Mozilla/5.0"`+"\n")
	if err := os.WriteFile(filepath.Join(dir, "access.log"), nil, 0644); err != nil {
		t.Fatal(err)
	}

	state := &FileState{ProcessedRotated: map[string]bool{}}
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)

	// Budget for only one day — the recent one must win, not the old one.
	finalized, _, err := Collect(dir, state, now, 1)
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if len(finalized) != 1 {
		t.Fatalf("expected 1 finalized hour, got %d", len(finalized))
	}
	if finalized[0].Hour != "2026-08-17T10:00:00Z" {
		t.Errorf("expected the most recent rotated day to be processed first, got hour %s", finalized[0].Hour)
	}
	if !state.ProcessedRotated["access.log-20260817.gz"] {
		t.Error("expected the most recent rotated file to be marked processed")
	}
	if state.ProcessedRotated["access.log-20260601.gz"] {
		t.Error("expected the older rotated file to be deferred, not processed")
	}
}

func TestCollect_LiveFileTakesPriorityOverRotatedBacklog(t *testing.T) {
	dir := t.TempDir()
	writeGzip(t, filepath.Join(dir, "access.log-20260601.gz"), `1.1.1.1 - - [01/Jun/2026:10:00:00 +0000] "GET / HTTP/2.0" 200 100 "-" "Mozilla/5.0"`+"\n")
	liveLine := `2.2.2.2 - - [18/Aug/2026:10:00:00 +0000] "GET / HTTP/2.0" 200 100 "-" "Mozilla/5.0"` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "access.log"), []byte(liveLine), 0644); err != nil {
		t.Fatal(err)
	}

	state := &FileState{ProcessedRotated: map[string]bool{}}
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)

	// Budget for only one hour total — today's live traffic must win.
	finalized, _, err := Collect(dir, state, now, 1)
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if len(finalized) != 1 {
		t.Fatalf("expected 1 finalized hour, got %d", len(finalized))
	}
	if finalized[0].Hour != "2026-08-18T10:00:00Z" {
		t.Errorf("expected today's live-file hour to take priority over old backlog, got hour %s", finalized[0].Hour)
	}
	if state.ProcessedRotated["access.log-20260601.gz"] {
		t.Error("expected the old rotated backlog to be deferred in favor of live/recent data")
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

	if _, _, err := Collect(dir, state, now, math.MaxInt); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	if int(state.Offset) != len(line1) {
		t.Errorf("offset = %d, want %d (partial trailing line must not be consumed)", state.Offset, len(line1))
	}
}
