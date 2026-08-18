package logs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/JCO-Digital/jman/internal/models"
)

// maxTrackedKeysPerHour bounds how many distinct pages/referrers/IPs an
// hour's in-progress accumulator will track, as a safety valve against
// unbounded memory/state-file growth from bot scanning traffic hitting many
// distinct garbage URLs within a single hour. Once the cap is hit, already-
// tracked keys keep incrementing but new keys are dropped — this only
// affects the tail of the top-N ranking, not the request/bot/human totals.
const maxTrackedKeysPerHour = 2000

// maxKeyLength truncates pathological long page/referrer keys (e.g. a
// crafted scanner URL, or a referrer with a large tracking query string)
// before using them as map keys, bounding worst-case entry size regardless
// of how unusual the traffic is.
const maxKeyLength = 300

// FileState is one access log file's persisted tailing state, stored
// locally by jman-agent (never sent to jman-api). It's only advanced after
// a successful report send, so a failed send simply results in re-reading
// the same log range next cycle rather than losing data.
type FileState struct {
	Inode            uint64           `json:"inode"`
	Offset           int64            `json:"offset"`
	ProcessedRotated map[string]bool  `json:"processed_rotated,omitempty"`
	Pending          *HourAccumulator `json:"pending,omitempty"`
}

// HourAccumulator collects traffic for one not-yet-closed hour across
// multiple collection cycles. It's only finalized (see Finalize) once the
// wall clock moves past it, at which point it's sent exactly once.
type HourAccumulator struct {
	Hour          string          `json:"hour"` // RFC3339, truncated to the hour, UTC
	RequestsTotal int             `json:"requests_total"`
	RequestsHuman int             `json:"requests_human"`
	RequestsBot   int             `json:"requests_bot"`
	UniqueIPs     map[string]bool `json:"unique_ips"`
	Pages         map[string]int  `json:"pages"`
	Referrers     map[string]int  `json:"referrers"`
}

func truncateKey(s string) string {
	if len(s) > maxKeyLength {
		return s[:maxKeyLength]
	}
	return s
}

func newHourAccumulator(hour string) *HourAccumulator {
	return &HourAccumulator{
		Hour:      hour,
		UniqueIPs: map[string]bool{},
		Pages:     map[string]int{},
		Referrers: map[string]int{},
	}
}

// Add records one classified request into the accumulator.
func (h *HourAccumulator) Add(e Entry, isBot bool) {
	h.RequestsTotal++
	if isBot {
		h.RequestsBot++
	} else {
		h.RequestsHuman++
	}

	if len(h.UniqueIPs) < maxTrackedKeysPerHour {
		h.UniqueIPs[e.RemoteAddr] = true
	}

	page := truncateKey(PathWithoutQuery(e.Path))
	if _, ok := h.Pages[page]; ok || len(h.Pages) < maxTrackedKeysPerHour {
		h.Pages[page]++
	}

	if e.Referer != "" && e.Referer != "-" {
		referrer := truncateKey(e.Referer)
		if _, ok := h.Referrers[referrer]; ok || len(h.Referrers) < maxTrackedKeysPerHour {
			h.Referrers[referrer]++
		}
	}
}

// Finalize converts the accumulator into the wire format sent to jman-api,
// truncating pages/referrers to their top entries.
func (h *HourAccumulator) Finalize() models.TrafficHourlyEntry {
	return models.TrafficHourlyEntry{
		Hour:           h.Hour,
		RequestsTotal:  h.RequestsTotal,
		RequestsHuman:  h.RequestsHuman,
		RequestsBot:    h.RequestsBot,
		UniqueVisitors: len(h.UniqueIPs),
		TopPages:       topN(h.Pages, 20),
		TopReferrers:   topN(h.Referrers, 20),
	}
}

func topN(counts map[string]int, n int) []models.TrafficTopEntry {
	entries := make([]models.TrafficTopEntry, 0, len(counts))
	for key, count := range counts {
		entries = append(entries, models.TrafficTopEntry{Key: key, Count: count})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Count != entries[j].Count {
			return entries[i].Count > entries[j].Count
		}
		return entries[i].Key < entries[j].Key
	})
	if len(entries) > n {
		entries = entries[:n]
	}
	return entries
}

// statePath returns the state file path for a given site.
func statePath(stateDir string, siteID int) string {
	return filepath.Join(stateDir, fmt.Sprintf("site-%d.json", siteID))
}

// LoadState reads a site's persisted log-tailing state, returning a fresh
// zero-value state (not an error) if none exists yet.
func LoadState(stateDir string, siteID int) (*FileState, error) {
	data, err := os.ReadFile(statePath(stateDir, siteID))
	if err != nil {
		if os.IsNotExist(err) {
			return &FileState{ProcessedRotated: map[string]bool{}}, nil
		}
		return nil, fmt.Errorf("failed to read log state: %w", err)
	}

	var state FileState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse log state: %w", err)
	}
	if state.ProcessedRotated == nil {
		state.ProcessedRotated = map[string]bool{}
	}
	return &state, nil
}

// SaveState persists a site's log-tailing state.
func SaveState(stateDir string, siteID int, state *FileState) error {
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to encode log state: %w", err)
	}

	path := statePath(stateDir, siteID)
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write log state: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to save log state: %w", err)
	}
	return nil
}
