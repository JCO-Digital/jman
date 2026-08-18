package logs

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
)

// rotatedAccessLogRe matches SpinupWP's rotated access log naming:
// access.log-YYYYMMDD.gz. Rotated files are immutable once created, so each
// is processed exactly once and never revisited.
var rotatedAccessLogRe = regexp.MustCompile(`^access\.log-\d{8}\.gz$`)

// hourlyRetentionWindow mirrors jman-api's site_traffic_hourly retention
// (siteTrafficHourlyRetention in internal/tasks/scheduler.go): a rotated
// log day older than this would arrive, get rolled into a daily rollup, and
// have its hourly rows pruned within about an hour of being received.
// Sending it as up to 24 individual hourly entries just to have jman-api
// immediately discard the hourly detail wastes report budget and
// bandwidth, so such a day is aggregated into a single TrafficDailyEntry
// instead (see FinalizeDaily).
const hourlyRetentionWindow = 48 * time.Hour

// Collect processes new lines from a site's access logs (live + any
// not-yet-processed rotated files) and returns any hours (and, for backlog
// past hourlyRetentionWindow, whole days) that are now fully elapsed and
// ready to send.
//
// maxFinalized bounds how many finalized entries (hourly and daily
// combined) this call may accumulate in total (from both rotated files and
// the live file) before it stops, leaving the rest for later cycles —
// without this, a large backlog (e.g. months of untouched rotated logs, or
// simply many hours already elapsed in today's not-yet-rotated live file,
// on the very first run of this feature) would get flushed into a single
// report far larger than jman-api's request body limit. Pass a very large
// value (e.g. math.MaxInt) for "no cap", e.g. in tests.
//
// state is mutated in place with candidate progress (new tail offset/inode,
// processed-rotated-file markers, in-progress hour accumulator). Callers
// must NOT persist it until the resulting report has been sent
// successfully — on failure, discard the mutated state and retry from the
// last-saved one next cycle, which simply re-reads the same log range.
//
// The returned bool reports whether this call stopped early due to
// maxFinalized with real work still left undone (more rotated files, or
// more live-log lines) — callers can use this to schedule the next
// collection cycle sooner than usual to drain a backlog faster, without
// raising maxFinalized itself and risking an oversized report.
func Collect(logsDir string, state *FileState, now time.Time, maxFinalized int, siteDomain string) ([]models.TrafficHourlyEntry, []models.TrafficDailyEntry, bool, error) {
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return nil, nil, false, fmt.Errorf("failed to read logs directory %s: %w", logsDir, err)
	}

	var rotated []string
	for _, e := range entries {
		if !e.IsDir() && rotatedAccessLogRe.MatchString(e.Name()) && !state.ProcessedRotated[e.Name()] {
			rotated = append(rotated, e.Name())
		}
	}
	// Date-suffixed names sort chronologically; reversed so the most recent
	// backlogged day is processed first (see priority note below).
	sort.Sort(sort.Reverse(sort.StringSlice(rotated)))

	currentHour := now.UTC().Truncate(time.Hour)
	cutoffDay := now.Add(-hourlyRetentionWindow).UTC().Truncate(24 * time.Hour)
	var finalized []models.TrafficHourlyEntry
	var finalizedDaily []models.TrafficDailyEntry

	// The live file (today's traffic) is processed BEFORE any rotated
	// backlog, and rotated files are walked most-recent-first: an operator
	// deploying this feature onto a server with months of untouched logs
	// should see current/recent traffic within the first few cycles, not
	// after the entire historical backlog has drained (which, at a small
	// per-cycle budget, could otherwise take days). Older history still
	// backfills — just at lower priority, in the background. The live file
	// always represents "today", which can never be past
	// hourlyRetentionWindow, so it's always processed hourly.
	hasMore, err := processLiveFile(filepath.Join(logsDir, "access.log"), state, currentHour, &finalized, maxFinalized, siteDomain)
	if err != nil {
		return finalized, finalizedDaily, hasMore, fmt.Errorf("failed to tail access.log: %w", err)
	}

	for _, name := range rotated {
		if len(finalized)+len(finalizedDaily) >= maxFinalized {
			verb.LogPrintf(verb.Normal, "Deferring remaining rotated logs in %s to a later cycle (per-report budget reached)", logsDir)
			hasMore = true
			break
		}
		if err := processRotatedFile(filepath.Join(logsDir, name), &finalized, &finalizedDaily, cutoffDay, siteDomain); err != nil {
			verb.LogPrintf(verb.Normal, "Failed to process rotated log %s: %v", name, err)
			continue // leave unmarked; retry next cycle
		}
		state.ProcessedRotated[name] = true
	}

	return finalized, finalizedDaily, hasMore, nil
}

// processRotatedFile fully parses a complete, immutable rotated log file.
// Since the file represents an entire past day, every hour found in it is
// necessarily already closed and is finalized immediately — as individual
// hourly entries if the day falls within hourlyRetentionWindow, or as a
// single daily entry (see FinalizeDaily) if it's older.
func processRotatedFile(path string, finalized *[]models.TrafficHourlyEntry, finalizedDaily *[]models.TrafficDailyEntry, cutoffDay time.Time, siteDomain string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("failed to open gzip stream: %w", err)
	}
	defer gz.Close()

	hourBuckets := map[string]*HourAccumulator{}
	dayBuckets := map[string]*HourAccumulator{}
	scanner := bufio.NewScanner(gz)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		accumulateLine(scanner.Text(), hourBuckets, dayBuckets, cutoffDay, siteDomain)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading gzip content: %w", err)
	}

	for _, acc := range hourBuckets {
		*finalized = append(*finalized, acc.Finalize())
	}
	for _, acc := range dayBuckets {
		*finalizedDaily = append(*finalizedDaily, acc.FinalizeDaily())
	}
	return nil
}

// accumulateLine routes a parsed log line into either hourBuckets or
// dayBuckets, depending on whether its calendar day falls before cutoffDay
// (see hourlyRetentionWindow). Only used for rotated (already-closed) files
// — the live file is always "today" and always uses hourBuckets via
// processLiveLine instead.
func accumulateLine(line string, hourBuckets, dayBuckets map[string]*HourAccumulator, cutoffDay time.Time, siteDomain string) {
	entry, err := ParseLine(line)
	if err != nil || IsInternalTraffic(entry) {
		return
	}
	entryTime := entry.Time.UTC()
	if entryTime.Truncate(24 * time.Hour).Before(cutoffDay) {
		day := entryTime.Truncate(24 * time.Hour).Format("2006-01-02")
		acc, ok := dayBuckets[day]
		if !ok {
			acc = newDayAccumulator(day)
			dayBuckets[day] = acc
		}
		acc.Add(entry, IsBotUserAgent(entry.UserAgent), siteDomain)
		return
	}
	hour := entryTime.Truncate(time.Hour).Format(time.RFC3339)
	acc, ok := hourBuckets[hour]
	if !ok {
		acc = newHourAccumulator(hour)
		hourBuckets[hour] = acc
	}
	acc.Add(entry, IsBotUserAgent(entry.UserAgent), siteDomain)
}

// processLiveFile incrementally tails the actively-written access.log,
// advancing state.Offset only past complete (newline-terminated) lines so
// a line mid-write is picked up whole next cycle rather than split. Stops
// early once maxFinalized is reached (e.g. many hours already elapsed in
// today's not-yet-rotated file on the very first run) — unread lines and
// any in-progress hour are simply picked up next cycle. Returns whether it
// stopped early with unread data still remaining.
func processLiveFile(path string, state *FileState, currentHour time.Time, finalized *[]models.TrafficHourlyEntry, maxFinalized int, siteDomain string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return false, err
	}
	inode := inodeOf(info)

	offset := state.Offset
	if state.Inode != 0 && state.Inode != inode {
		// Rotation happened since we last looked: this is a new file. Any
		// trailing data from the old file arrives via its rotated .gz once
		// compression finishes (picked up by Collect on a later cycle).
		offset = 0
	}
	if offset > info.Size() {
		offset = 0 // file was truncated/replaced out from under us
	}

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return false, fmt.Errorf("failed to seek: %w", err)
	}

	hasMore := false
	reader := bufio.NewReaderSize(f, 64*1024)
	for {
		if len(*finalized) >= maxFinalized {
			verb.LogPrintf(verb.Normal, "Deferring remaining live-log lines in %s to a later cycle (per-report budget reached)", path)
			hasMore = true
			break
		}
		lineBytes, readErr := reader.ReadBytes('\n')
		if len(lineBytes) > 0 && lineBytes[len(lineBytes)-1] == '\n' {
			line := strings.TrimRight(string(lineBytes), "\r\n")
			processLiveLine(line, state, finalized, siteDomain)
			offset += int64(len(lineBytes))
		}
		if readErr != nil {
			break // io.EOF (normal), or a partial trailing line left for next cycle
		}
	}

	state.Inode = inode
	state.Offset = offset

	// Close out the pending hour if the wall clock has moved past it, even
	// if no new line has arrived to reveal that (e.g. a quiet period) — but
	// only if there's still budget, so a cycle that stopped early above
	// doesn't sneak one more entry in past the cap.
	if len(*finalized) < maxFinalized && state.Pending != nil && state.Pending.Hour < currentHour.Format(time.RFC3339) {
		*finalized = append(*finalized, state.Pending.Finalize())
		state.Pending = nil
	}

	return hasMore, nil
}

func processLiveLine(line string, state *FileState, finalized *[]models.TrafficHourlyEntry, siteDomain string) {
	entry, err := ParseLine(line)
	if err != nil || IsInternalTraffic(entry) {
		return
	}

	hour := entry.Time.UTC().Truncate(time.Hour).Format(time.RFC3339)

	if state.Pending != nil && state.Pending.Hour != hour {
		*finalized = append(*finalized, state.Pending.Finalize())
		state.Pending = nil
	}
	if state.Pending == nil {
		state.Pending = newHourAccumulator(hour)
	}
	state.Pending.Add(entry, IsBotUserAgent(entry.UserAgent), siteDomain)
}

func inodeOf(info os.FileInfo) uint64 {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return stat.Ino
	}
	return 0
}
