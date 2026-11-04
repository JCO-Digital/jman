// Package threatintel fetches public exploitation data for CVEs: CISA's
// Known Exploited Vulnerabilities (KEV) catalog and FIRST's Exploit
// Prediction Scoring System (EPSS). Only CVE IDs are sent. Both are cached
// for a day in jman's cache directory.
package threatintel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
)

// Feed URLs, variables for tests.
var (
	KEVURL  = "https://www.cisa.gov/sites/default/files/feeds/known_exploited_vulnerabilities.json"
	EPSSURL = "https://api.first.org/data/v1/epss"
)

const (
	cacheTTL      = 24 * time.Hour
	epssBatchSize = 50
)

var (
	httpClient = &http.Client{Timeout: 30 * time.Second}
	mu         sync.Mutex
	cveRe      = regexp.MustCompile(`CVE-\d{4}-\d{4,}`)
)

// KEVEntry is a CVE in CISA's KEV catalog: it is known to be exploited.
type KEVEntry struct {
	CVE       string `json:"cve"`
	DateAdded string `json:"date_added"`
	// KnownRansomware is set when the CVE is known to be used in
	// ransomware campaigns.
	KnownRansomware bool `json:"known_ransomware"`
}

// EPSS is a CVE's probability of exploitation in the next 30 days, and its
// percentile among all scored CVEs.
type EPSS struct {
	CVE        string  `json:"cve"`
	Score      float64 `json:"score"`
	Percentile float64 `json:"percentile"`
	Date       string  `json:"date"`
}

// CVEIDs returns the distinct CVE IDs found in texts, sorted.
func CVEIDs(texts ...string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range texts {
		for _, id := range cveRe.FindAllString(strings.ToUpper(t), -1) {
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	sort.Strings(out)
	return out
}

// KEV returns the KEV catalog by CVE ID.
func KEV(ctx context.Context) (map[string]KEVEntry, error) {
	mu.Lock()
	defer mu.Unlock()

	path := cachePath("kev.json")
	var cached struct {
		FetchedAt time.Time           `json:"fetched_at"`
		Entries   map[string]KEVEntry `json:"entries"`
	}
	if readJSON(path, &cached) == nil && time.Since(cached.FetchedAt) < cacheTTL {
		return cached.Entries, nil
	}

	var feed struct {
		Vulnerabilities []struct {
			CveID      string `json:"cveID"`
			DateAdded  string `json:"dateAdded"`
			Ransomware string `json:"knownRansomwareCampaignUse"`
		} `json:"vulnerabilities"`
	}
	if err := getJSON(ctx, KEVURL, &feed); err != nil {
		if cached.Entries != nil {
			return cached.Entries, nil
		}
		return nil, fmt.Errorf("failed to fetch the CISA KEV catalog: %w", err)
	}
	entries := make(map[string]KEVEntry, len(feed.Vulnerabilities))
	for _, v := range feed.Vulnerabilities {
		entries[v.CveID] = KEVEntry{CVE: v.CveID, DateAdded: v.DateAdded,
			KnownRansomware: strings.EqualFold(v.Ransomware, "Known")}
	}
	cached.FetchedAt, cached.Entries = time.Now(), entries
	_ = writeJSON(path, cached)
	return entries, nil
}

// EPSSScores returns the EPSS scores of cves that have one. Scores older
// than a day are fetched again; when that fails the old ones are used.
func EPSSScores(ctx context.Context, cves []string) (map[string]EPSS, error) {
	mu.Lock()
	defer mu.Unlock()

	path := cachePath("epss.json")
	type entry struct {
		FetchedAt time.Time `json:"fetched_at"`
		EPSS      *EPSS     `json:"epss"` // nil: the CVE has no score
	}
	cache := map[string]entry{}
	_ = readJSON(path, &cache)

	var missing []string
	for _, cve := range cves {
		if e, ok := cache[cve]; !ok || time.Since(e.FetchedAt) >= cacheTTL {
			missing = append(missing, cve)
		}
	}
	var fetchErr error
	for start := 0; start < len(missing); start += epssBatchSize {
		batch := missing[start:min(start+epssBatchSize, len(missing))]
		scores, err := fetchEPSS(ctx, batch)
		if err != nil {
			fetchErr = err
			break
		}
		now := time.Now()
		for _, cve := range batch {
			e := entry{FetchedAt: now}
			if s, ok := scores[cve]; ok {
				e.EPSS = &s
			}
			cache[cve] = e
		}
	}
	if len(missing) > 0 && fetchErr == nil {
		_ = writeJSON(path, cache)
	}

	out := map[string]EPSS{}
	for _, cve := range cves {
		if e, ok := cache[cve]; ok && e.EPSS != nil {
			out[cve] = *e.EPSS
		}
	}
	if fetchErr != nil && len(out) == 0 {
		return nil, fmt.Errorf("failed to fetch EPSS scores: %w", fetchErr)
	}
	return out, nil
}

func fetchEPSS(ctx context.Context, cves []string) (map[string]EPSS, error) {
	u := EPSSURL + "?" + url.Values{"cve": {strings.Join(cves, ",")}, "limit": {strconv.Itoa(len(cves))}}.Encode()
	var resp struct {
		Data []struct {
			CVE        string `json:"cve"`
			EPSS       string `json:"epss"`
			Percentile string `json:"percentile"`
			Date       string `json:"date"`
		} `json:"data"`
	}
	if err := getJSON(ctx, u, &resp); err != nil {
		return nil, err
	}
	out := make(map[string]EPSS, len(resp.Data))
	for _, d := range resp.Data {
		score, err1 := strconv.ParseFloat(d.EPSS, 64)
		pct, err2 := strconv.ParseFloat(d.Percentile, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		out[d.CVE] = EPSS{CVE: d.CVE, Score: score, Percentile: pct, Date: d.Date}
	}
	return out, nil
}

func getJSON(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "jman-api")
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: HTTP %d", req.URL.Host, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func cachePath(name string) string {
	return filepath.Join(config.RunData.CacheDir, "threatintel", name)
}

func readJSON(path string, out any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

func writeJSON(path string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
