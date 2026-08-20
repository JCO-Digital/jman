package logs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"

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

// HourAccumulator collects traffic for one period across multiple
// collection cycles. Usually that period is a not-yet-closed hour, only
// finalized (see Finalize) once the wall clock moves past it, at which
// point it's sent exactly once. The same struct also doubles as a whole-day
// accumulator (see newDayAccumulator/FinalizeDaily) for rotated-log backlog
// older than jman-api's hourly retention window — such a file is already a
// complete, immutable past day, so it's always finalized immediately.
type HourAccumulator struct {
	Hour          string          `json:"hour"` // RFC3339 hour; or, for a day accumulator, a YYYY-MM-DD day
	RequestsTotal int             `json:"requests_total"`
	RequestsHuman int             `json:"requests_human"`
	RequestsBot   int             `json:"requests_bot"`
	UniqueIPs     map[string]bool `json:"unique_ips"`
	Pages         map[string]int  `json:"pages"`
	Referrers     map[string]int  `json:"referrers"`
	StatusCodes   map[string]int  `json:"status_codes"`
}

func truncateKey(s string) string {
	if len(s) > maxKeyLength {
		return s[:maxKeyLength]
	}
	return s
}

func newHourAccumulator(hour string) *HourAccumulator {
	return &HourAccumulator{
		Hour:        hour,
		UniqueIPs:   map[string]bool{},
		Pages:       map[string]int{},
		Referrers:   map[string]int{},
		StatusCodes: map[string]int{},
	}
}

// newDayAccumulator creates an accumulator keyed by a whole calendar day
// (YYYY-MM-DD) instead of an hour, for aggregating rotated-log backlog
// older than jman-api's hourly retention window — see FinalizeDaily.
func newDayAccumulator(day string) *HourAccumulator {
	return newHourAccumulator(day)
}

// Add records one classified request into the accumulator. siteDomain is
// the site's own primary domain, used to drop same-site referrers (see
// isInternalReferrer) before they can occupy one of the limited top-N slots.
func (h *HourAccumulator) Add(e Entry, isBot bool, siteDomain string) {
	// A Pending accumulator loaded from a state file persisted by an older
	// jman-agent version (see LoadState) may predate a field added here
	// since, and json.Unmarshal leaves a missing map field as nil rather
	// than initializing it — guard every map against that before any
	// index-assignment below, which would otherwise panic on a nil map.
	if h.UniqueIPs == nil {
		h.UniqueIPs = map[string]bool{}
	}
	if h.Pages == nil {
		h.Pages = map[string]int{}
	}
	if h.Referrers == nil {
		h.Referrers = map[string]int{}
	}
	if h.StatusCodes == nil {
		h.StatusCodes = map[string]int{}
	}

	h.RequestsTotal++
	if isBot {
		h.RequestsBot++
	} else {
		h.RequestsHuman++
	}

	if len(h.UniqueIPs) < maxTrackedKeysPerHour {
		h.UniqueIPs[e.RemoteAddr] = true
	}

	// Status codes are tracked for every request regardless of path or
	// status — the key space is inherently bounded (ParseLine only accepts
	// a 3-digit status), so no maxTrackedKeysPerHour cap is needed here.
	h.StatusCodes[strconv.Itoa(e.Status)]++

	// Only a real page view (status 200, not a WordPress system path)
	// competes for a top-pages slot; a 404/301/500 isn't a "page" someone
	// actually viewed.
	if path := PathWithoutQuery(e.Path); e.Status == 200 && !isExcludedPage(path) {
		page := truncateKey(path)
		if _, ok := h.Pages[page]; ok || len(h.Pages) < maxTrackedKeysPerHour {
			h.Pages[page]++
		}
	}

	if e.Referer != "" && e.Referer != "-" {
		if refHost := normalizeReferrerHost(e.Referer); refHost != "" && !isInternalReferrer(refHost, siteDomain) {
			refHost = truncateKey(refHost)
			if _, ok := h.Referrers[refHost]; ok || len(h.Referrers) < maxTrackedKeysPerHour {
				h.Referrers[refHost]++
			}
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
		StatusCodes:    topN(h.StatusCodes, 20),
	}
}

// FinalizeDaily converts a day accumulator (see newDayAccumulator) into the
// wire format for backlog beyond the hourly retention window. Because it
// ranks the full day's raw counts directly, this is actually more accurate
// than jman-api's own daily rollup, which approximates a day by merging
// each hour's already-truncated top-20 list.
func (h *HourAccumulator) FinalizeDaily() models.TrafficDailyEntry {
	return models.TrafficDailyEntry{
		Day:            h.Hour,
		RequestsTotal:  h.RequestsTotal,
		RequestsHuman:  h.RequestsHuman,
		RequestsBot:    h.RequestsBot,
		UniqueVisitors: len(h.UniqueIPs),
		TopPages:       topN(h.Pages, 20),
		TopReferrers:   topN(h.Referrers, 20),
		StatusCodes:    topN(h.StatusCodes, 20),
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
