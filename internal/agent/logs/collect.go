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

// Collect processes new lines from a site's access logs (live + any
// not-yet-processed rotated files) and returns any hours that are now
// fully elapsed and ready to send.
//
// state is mutated in place with candidate progress (new tail offset/inode,
// processed-rotated-file markers, in-progress hour accumulator). Callers
// must NOT persist it until the resulting report has been sent
// successfully — on failure, discard the mutated state and retry from the
// last-saved one next cycle, which simply re-reads the same log range.
func Collect(logsDir string, state *FileState, now time.Time) ([]models.TrafficHourlyEntry, error) {
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read logs directory %s: %w", logsDir, err)
	}

	var rotated []string
	for _, e := range entries {
		if !e.IsDir() && rotatedAccessLogRe.MatchString(e.Name()) && !state.ProcessedRotated[e.Name()] {
			rotated = append(rotated, e.Name())
		}
	}
	sort.Strings(rotated) // date-suffixed names sort chronologically

	currentHour := now.UTC().Truncate(time.Hour)
	var finalized []models.TrafficHourlyEntry

	for _, name := range rotated {
		if err := processRotatedFile(filepath.Join(logsDir, name), &finalized); err != nil {
			verb.LogPrintf(verb.Normal, "Failed to process rotated log %s: %v", name, err)
			continue // leave unmarked; retry next cycle
		}
		state.ProcessedRotated[name] = true
	}

	if err := processLiveFile(filepath.Join(logsDir, "access.log"), state, currentHour, &finalized); err != nil {
		return finalized, fmt.Errorf("failed to tail access.log: %w", err)
	}

	return finalized, nil
}

// processRotatedFile fully parses a complete, immutable rotated log file.
// Since the file represents an entire past day, every hour found in it is
// necessarily already closed and is finalized immediately.
func processRotatedFile(path string, finalized *[]models.TrafficHourlyEntry) error {
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

	buckets := map[string]*HourAccumulator{}
	scanner := bufio.NewScanner(gz)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		accumulateLine(scanner.Text(), buckets)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading gzip content: %w", err)
	}

	for _, acc := range buckets {
		*finalized = append(*finalized, acc.Finalize())
	}
	return nil
}

func accumulateLine(line string, buckets map[string]*HourAccumulator) {
	entry, err := ParseLine(line)
	if err != nil || IsInternalTraffic(entry) {
		return
	}
	hour := entry.Time.UTC().Truncate(time.Hour).Format(time.RFC3339)
	acc, ok := buckets[hour]
	if !ok {
		acc = newHourAccumulator(hour)
		buckets[hour] = acc
	}
	acc.Add(entry, IsBotUserAgent(entry.UserAgent))
}

// processLiveFile incrementally tails the actively-written access.log,
// advancing state.Offset only past complete (newline-terminated) lines so
// a line mid-write is picked up whole next cycle rather than split.
func processLiveFile(path string, state *FileState, currentHour time.Time, finalized *[]models.TrafficHourlyEntry) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
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
		return fmt.Errorf("failed to seek: %w", err)
	}

	reader := bufio.NewReaderSize(f, 64*1024)
	for {
		lineBytes, readErr := reader.ReadBytes('\n')
		if len(lineBytes) > 0 && lineBytes[len(lineBytes)-1] == '\n' {
			line := strings.TrimRight(string(lineBytes), "\r\n")
			processLiveLine(line, state, finalized)
			offset += int64(len(lineBytes))
		}
		if readErr != nil {
			break // io.EOF (normal), or a partial trailing line left for next cycle
		}
	}

	state.Inode = inode
	state.Offset = offset

	// Close out the pending hour if the wall clock has moved past it, even
	// if no new line has arrived to reveal that (e.g. a quiet period).
	if state.Pending != nil && state.Pending.Hour < currentHour.Format(time.RFC3339) {
		*finalized = append(*finalized, state.Pending.Finalize())
		state.Pending = nil
	}

	return nil
}

func processLiveLine(line string, state *FileState, finalized *[]models.TrafficHourlyEntry) {
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
	state.Pending.Add(entry, IsBotUserAgent(entry.UserAgent))
}

func inodeOf(info os.FileInfo) uint64 {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return stat.Ino
	}
	return 0
}
