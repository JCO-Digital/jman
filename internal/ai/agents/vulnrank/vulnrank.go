// Package vulnrank is the vuln-rank AI agent: it ranks the open
// vulnerability exposure groups by how urgently they need work.
//
// The model gets only per-group facts: the plugin or core version, its
// vulnerabilities (feed data, CISA KEV and EPSS) and counts of affected
// sites by environment, fix state, overdue and muted. No site names,
// domains or IDs are sent.
package vulnrank

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/ai"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/threatintel"
	"github.com/JCO-Digital/jman/internal/utils"
	"github.com/JCO-Digital/jman/internal/vuln"
)

// Name is the agent's name in the API.
const Name = "vuln-rank"

//go:embed prompt.md
var systemPrompt string

const (
	maxKeys        = 500
	maxDescription = 1500
)

var keyRe = regexp.MustCompile(`^(plugin|core):[A-Za-z0-9._-]+$`)

// Agent ranks vulnerability exposure groups. Its fields are the data
// sources, replaced in tests.
type Agent struct {
	groups func(now time.Time) ([]models.VulnExposureGroup, error)
	envs   func() (map[string]string, error)
	vulns  func(kind, slug, coreVersion string) map[string]models.Vulnerability
	kev    func(ctx context.Context) (map[string]threatintel.KEVEntry, error)
	epss   func(ctx context.Context, cves []string) (map[string]threatintel.EPSS, error)
	now    func() time.Time
	warnf  func(format string, args ...any)
}

// New returns the agent reading jman-api's data.
func New() *Agent {
	return &Agent{
		groups: func(now time.Time) ([]models.VulnExposureGroup, error) {
			exposures, err := db.ListOpenVulnExposures()
			if err != nil {
				return nil, err
			}
			return vuln.BuildExposureGroups(exposures, now), nil
		},
		envs:  siteEnvironments,
		vulns: vuln.GroupVulnerabilities,
		kev:   threatintel.KEV,
		epss:  threatintel.EPSSScores,
		now:   time.Now,
		warnf: func(format string, args ...any) { log.Printf("vuln-rank: "+format, args...) },
	}
}

// siteEnvironments maps site IDs to their environment as /api/sites shows
// it: the site's own, unless it has been classified.
func siteEnvironments() (map[string]string, error) {
	sites, err := db.ListManagedSites()
	if err != nil {
		return nil, err
	}
	classified, err := db.GetAllSiteEnvironments()
	if err != nil {
		return nil, err
	}
	return mergeEnvironments(sites, classified), nil
}

func mergeEnvironments(sites []models.ManagedSite, classified map[string]string) map[string]string {
	envs := make(map[string]string, len(sites)+len(classified))
	for _, s := range sites {
		if s.Environment != "" {
			envs[s.ID] = string(s.Environment)
		}
	}
	maps.Copy(envs, classified)
	return envs
}

// input is the run input: the groups to rank, all open ones when empty.
type input struct {
	Keys []string `json:"keys,omitempty"`
}

var inputSchema = json.RawMessage(`{"type":"object","properties":{"keys":{"type":"array","items":{"type":"string"},` +
	`"description":"Exposure group keys to rank (\"plugin:<slug>\" or \"core:<version>\"); all open groups when empty."}},` +
	`"additionalProperties":false}`)

var outputSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []string{"rankings"},
	"properties": map[string]any{
		"rankings": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"key", "priority", "score", "action", "rationale", "confidence"},
				"properties": map[string]any{
					"key":      map[string]any{"type": "string", "description": "The group's key, exactly as given."},
					"priority": map[string]any{"type": "string", "enum": []string{"P1", "P2", "P3", "P4"}},
					"score":    map[string]any{"type": "integer", "description": "0 to 100; higher is more urgent."},
					"action": map[string]any{"type": "string", "enum": []string{
						models.AIActionUpdateNow, models.AIActionUpdateThisWeek, models.AIActionRemovePlugin,
						models.AIActionMuteCandidate, models.AIActionInvestigate}},
					"rationale":  map[string]any{"type": "string", "description": "One or two short sentences citing the deciding facts."},
					"confidence": map[string]any{"type": "string", "enum": []string{"low", "medium", "high"}},
				},
			},
		},
	},
}

// Spec implements ai.Agent.
func (a *Agent) Spec() ai.Spec {
	return ai.Spec{
		Name:    Name,
		Version: "1",
		Description: "Ranks the open vulnerability exposure groups (plugins and core versions) by urgency, " +
			"using the feed data, CISA KEV, EPSS and how many production sites each affects.",
		Effort:         "high",
		System:         systemPrompt,
		InputSchema:    inputSchema,
		OutputSchema:   outputSchema,
		MaxTokens:      64000,
		ReuseFor:       24 * time.Hour,
		ScheduleEvery:  24 * time.Hour,
		ScheduledInput: json.RawMessage(`{}`),
		Timeout:        15 * time.Minute,
		// The data holds no personal data: per-group facts from public
		// feeds and site counts, with email addresses removed (cleanText).
		Batchable: true,
	}
}

// ValidateInput implements ai.Agent.
func (a *Agent) ValidateInput(raw json.RawMessage) (json.RawMessage, error) {
	var in input
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		return nil, err
	}
	if len(in.Keys) > maxKeys {
		return nil, fmt.Errorf("at most %d keys", maxKeys)
	}
	seen := map[string]bool{}
	keys := []string{}
	for _, k := range in.Keys {
		if !keyRe.MatchString(k) {
			return nil, fmt.Errorf("key %q must be plugin:<slug> or core:<version>", k)
		}
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	in.Keys = keys
	if len(in.Keys) == 0 {
		in.Keys = nil
	}
	return json.Marshal(in)
}

// groupData is what the model gets about one exposure group.
type groupData struct {
	Key           string `json:"key"`
	Kind          string `json:"kind"`
	Name          string `json:"name"`
	ClosedOnWPOrg bool   `json:"closed_on_wordpress_org,omitempty"`
	ClosedReason  string `json:"closed_reason,omitempty"`
	ClosedDate    string `json:"closed_date,omitempty"`
	LastUpdated   string `json:"last_updated,omitempty"`
	// MaxCVSS is the highest CVSS among the sites' vulnerabilities, kept
	// even when the feed data of the vulnerabilities is missing.
	MaxCVSS            *float64 `json:"max_cvss,omitempty"`
	Sites              counts   `json:"sites"`
	InstalledVersions  []string `json:"installed_versions"`
	FixedIn            []string `json:"fixed_in,omitempty"`
	PossibleFixVersion string   `json:"possible_fix_version,omitempty"`
	DaysOpen           int      `json:"days_open"`
	// DaysFixAvailable is how long the oldest unmuted fix has been
	// available; the overdue clock.
	DaysFixAvailable *int       `json:"days_fix_available,omitempty"`
	Vulnerabilities  []vulnData `json:"vulnerabilities"`
}

type counts struct {
	Total        int `json:"total"`
	Production   int `json:"production"`
	Staging      int `json:"staging"`
	Development  int `json:"development"`
	UnknownEnv   int `json:"unknown_environment"`
	FixAvailable int `json:"fix_available"`
	NoFix        int `json:"no_fix"`
	Overdue      int `json:"overdue_unmuted"`
	Muted        int `json:"muted"`
}

type vulnData struct {
	Title       string   `json:"title"`
	CVEs        []string `json:"cves,omitempty"`
	CVSS        *float64 `json:"cvss,omitempty"`
	Severity    string   `json:"severity,omitempty"`
	Vector      string   `json:"cvss_vector,omitempty"`
	CWEs        []string `json:"cwes,omitempty"`
	Description string   `json:"description,omitempty"`
	Unfixed     bool     `json:"unfixed,omitempty"`
	FixedIn     string   `json:"fixed_in,omitempty"`
	KEV         *kevData `json:"cisa_kev,omitempty"`
	EPSS        *float64 `json:"epss,omitempty"`
	EPSSPct     *float64 `json:"epss_percentile,omitempty"`
}

type kevData struct {
	DateAdded       string `json:"date_added"`
	KnownRansomware bool   `json:"known_ransomware_use,omitempty"`
}

// state is kept from Prompt for Finalize and FakeAnswer.
type state struct {
	Groups []groupData `json:"groups"`
}

func decodeState(p ai.Prompt) (*state, error) {
	var st state
	if err := json.Unmarshal(p.State, &st); err != nil || st.Groups == nil {
		return nil, fmt.Errorf("missing prompt state")
	}
	return &st, nil
}

// Prompt implements ai.Agent.
func (a *Agent) Prompt(ctx context.Context, raw json.RawMessage) (ai.Prompt, error) {
	var in input
	if err := json.Unmarshal(raw, &in); err != nil {
		return ai.Prompt{}, err
	}
	now := a.now()
	groups, err := a.groups(now)
	if err != nil {
		return ai.Prompt{}, fmt.Errorf("failed to load the exposures: %w", err)
	}
	envs, err := a.envs()
	if err != nil {
		a.warnf("site environments unavailable: %v", err)
		envs = map[string]string{}
	}

	want := map[string]bool{}
	for _, k := range in.Keys {
		want[k] = true
	}
	var data []groupData
	var cves []string
	vulnsByKey := map[string]map[string]models.Vulnerability{}
	for _, g := range groups {
		key := models.VulnExposureGroupKey(g.Kind, g.Slug, g.Version)
		if len(want) > 0 && !want[key] {
			continue
		}
		vulns := a.vulns(g.Kind, g.Slug, g.Version)
		vulnsByKey[key] = vulns
		for _, v := range g.Vulnerabilities {
			if fv, ok := vulns[v.UUID]; ok {
				cves = append(cves, vulnCVEs(fv)...)
			}
		}
		data = append(data, describeGroup(g, key, envs, now))
	}
	if len(data) == 0 {
		return ai.Prompt{Skip: json.RawMessage(`{"rankings":[]}`)}, nil
	}

	kev, err := a.kev(ctx)
	if err != nil {
		a.warnf("%v", err)
	}
	epss, err := a.epss(ctx, uniqueSorted(cves))
	if err != nil {
		a.warnf("%v", err)
	}

	byKey := map[string]models.VulnExposureGroup{}
	for _, g := range groups {
		byKey[models.VulnExposureGroupKey(g.Kind, g.Slug, g.Version)] = g
	}
	for i := range data {
		g := byKey[data[i].Key]
		data[i].Vulnerabilities = describeVulns(g, vulnsByKey[data[i].Key], kev, epss)
	}
	sort.Slice(data, func(i, j int) bool { return data[i].Key < data[j].Key })

	doc, err := json.MarshalIndent(map[string]any{
		"exploitation_data": map[string]bool{"cisa_kev": kev != nil, "epss": epss != nil},
		"groups":            data,
	}, "", " ")
	if err != nil {
		return ai.Prompt{}, err
	}
	st, err := json.Marshal(state{Groups: data})
	if err != nil {
		return ai.Prompt{}, err
	}
	text := fmt.Sprintf("Rank these %d exposure groups.\n\n<exposure_data>\n%s\n</exposure_data>", len(data), doc)
	return ai.Prompt{Text: text, State: st}, nil
}

func describeGroup(g models.VulnExposureGroup, key string, envs map[string]string, now time.Time) groupData {
	d := groupData{Key: key, Kind: g.Kind, Name: g.Name, ClosedOnWPOrg: g.Closed,
		ClosedReason: g.ClosedReason, ClosedDate: g.ClosedDate, MaxCVSS: g.MaxCvss, InstalledVersions: []string{}}
	if g.LastUpdated != nil {
		d.LastUpdated = g.LastUpdated.Format("2006-01-02")
	}
	versions, fixedIn := map[string]bool{}, map[string]bool{}
	var firstSeen, fixSince *time.Time
	for _, s := range g.Sites {
		d.Sites.Total++
		switch envs[s.SiteID] {
		case string(models.SiteEnvironmentProduction):
			d.Sites.Production++
		case string(models.SiteEnvironmentStaging):
			d.Sites.Staging++
		case string(models.SiteEnvironmentDevelopment):
			d.Sites.Development++
		default:
			d.Sites.UnknownEnv++
		}
		if s.FixState == models.VulnFixAvailable {
			d.Sites.FixAvailable++
		} else {
			d.Sites.NoFix++
		}
		if s.Mute != nil {
			d.Sites.Muted++
		} else if s.Overdue {
			d.Sites.Overdue++
		}
		if s.Version != "" {
			versions[s.Version] = true
		}
		if s.FixedIn != "" {
			fixedIn[s.FixedIn] = true
		}
		if s.PossibleFixVersion != "" {
			d.PossibleFixVersion = s.PossibleFixVersion
		}
		if firstSeen == nil || s.FirstSeenAt.Before(*firstSeen) {
			t := s.FirstSeenAt
			firstSeen = &t
		}
		if s.Mute == nil && s.FixAvailableSince != nil && (fixSince == nil || s.FixAvailableSince.Before(*fixSince)) {
			t := *s.FixAvailableSince
			fixSince = &t
		}
	}
	d.InstalledVersions = sortedKeys(versions)
	d.FixedIn = sortedKeys(fixedIn)
	if firstSeen != nil {
		d.DaysOpen = days(now.Sub(*firstSeen))
	}
	if fixSince != nil {
		n := days(now.Sub(*fixSince))
		d.DaysFixAvailable = &n
	}
	return d
}

func describeVulns(g models.VulnExposureGroup, feed map[string]models.Vulnerability,
	kev map[string]threatintel.KEVEntry, epss map[string]threatintel.EPSS) []vulnData {
	out := []vulnData{}
	for _, v := range g.Vulnerabilities {
		d := vulnData{Title: cleanText(v.Name), CVSS: v.Cvss, Unfixed: v.Unfixed, FixedIn: v.FixedIn}
		if fv, ok := feed[v.UUID]; ok {
			d.CVEs = vulnCVEs(fv)
			if fv.Impact != nil {
				if c := fv.Impact.Cvss; c != nil {
					d.Severity, d.Vector = c.Severity, c.Vector
				}
				for _, cwe := range fv.Impact.Cwe {
					d.CWEs = append(d.CWEs, strings.TrimSpace(cwe.Cwe+" "+cwe.Name))
				}
			}
			d.Description = description(fv)
		}
		for _, cve := range d.CVEs {
			if e, ok := kev[cve]; ok && d.KEV == nil {
				d.KEV = &kevData{DateAdded: e.DateAdded, KnownRansomware: e.KnownRansomware}
			}
			if s, ok := epss[cve]; ok && (d.EPSS == nil || s.Score > *d.EPSS) {
				score, pct := s.Score, s.Percentile
				d.EPSS, d.EPSSPct = &score, &pct
			}
		}
		out = append(out, d)
	}
	return out
}

// description is the vulnerability's description, or its first source's,
// cut to maxDescription characters with a marker.
func description(v models.Vulnerability) string {
	text := ""
	if v.Description != nil {
		text = *v.Description
	}
	for _, s := range v.Source {
		if text == "" && s.Description != nil {
			text = *s.Description
		}
	}
	text = cleanText(text)
	if r := []rune(text); len(r) > maxDescription {
		text = string(r[:maxDescription]) + " [description truncated]"
	}
	return text
}

var emailRe = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,}`)

// cleanText strips HTML and extra whitespace from feed text, and removes
// email addresses (researchers' contacts in advisories), so the prompt
// holds no personal contact data.
func cleanText(s string) string {
	s = strings.Join(strings.Fields(utils.CleanHTML(s)), " ")
	return emailRe.ReplaceAllString(s, "[email removed]")
}

func vulnCVEs(v models.Vulnerability) []string {
	var texts []string
	for _, s := range v.Source {
		texts = append(texts, s.Id, s.Name)
	}
	return threatintel.CVEIDs(texts...)
}

// Finalize implements ai.Agent: it keeps the rankings of the groups that
// were sent, once each, clamps the scores and orders them by score.
func (a *Agent) Finalize(ctx context.Context, p ai.Prompt, answer json.RawMessage) (json.RawMessage, error) {
	st, err := decodeState(p)
	if err != nil {
		return nil, err
	}
	var got models.AIVulnRanking
	if err := json.Unmarshal(answer, &got); err != nil {
		return nil, err
	}
	sent := map[string]bool{}
	for _, g := range st.Groups {
		sent[g.Key] = true
	}
	out := models.AIVulnRanking{Rankings: []models.AIVulnRank{}}
	seen := map[string]bool{}
	for _, r := range got.Rankings {
		if !sent[r.Key] || seen[r.Key] {
			continue
		}
		seen[r.Key] = true
		r.Score = max(0, min(100, r.Score))
		r.Rationale = strings.TrimSpace(r.Rationale)
		out.Rankings = append(out.Rankings, r)
	}
	if len(out.Rankings) == 0 {
		return nil, fmt.Errorf("the answer ranks none of the %d groups", len(st.Groups))
	}
	for _, g := range st.Groups {
		if !seen[g.Key] {
			out.Unranked = append(out.Unranked, g.Key)
		}
	}
	sort.SliceStable(out.Rankings, func(i, j int) bool {
		a, b := out.Rankings[i], out.Rankings[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		return a.Priority < b.Priority
	})
	for i := range out.Rankings {
		out.Rankings[i].Rank = i + 1
	}
	return json.Marshal(out)
}

// FakeAnswer implements ai.Faker with a simple rule-based ranking, for
// local development.
func (a *Agent) FakeAnswer(ctx context.Context, p ai.Prompt) (json.RawMessage, error) {
	st, err := decodeState(p)
	if err != nil {
		return nil, err
	}
	out := models.AIVulnRanking{Rankings: []models.AIVulnRank{}}
	for _, g := range st.Groups {
		out.Rankings = append(out.Rankings, fakeRank(g))
	}
	return json.Marshal(out)
}

func fakeRank(g groupData) models.AIVulnRank {
	maxCVSS, inKEV := 0.0, false
	if g.MaxCVSS != nil {
		maxCVSS = *g.MaxCVSS
	}
	for _, v := range g.Vulnerabilities {
		if v.CVSS != nil && *v.CVSS > maxCVSS {
			maxCVSS = *v.CVSS
		}
		inKEV = inKEV || v.KEV != nil
	}
	score := int(maxCVSS * 7)
	if inKEV {
		score += 30
	}
	if g.Sites.Production > 0 {
		score += 10
	}
	if g.Sites.Muted == g.Sites.Total {
		score /= 3
	}
	score = min(100, score)
	r := models.AIVulnRank{Key: g.Key, Score: score, Confidence: "low",
		Rationale: fmt.Sprintf("Fake ranking (aiFake): highest CVSS %.1f on %d production sites.", maxCVSS, g.Sites.Production)}
	switch {
	case score >= 80:
		r.Priority = models.AIPriorityP1
	case score >= 60:
		r.Priority = models.AIPriorityP2
	case score >= 30:
		r.Priority = models.AIPriorityP3
	default:
		r.Priority = models.AIPriorityP4
	}
	switch {
	case g.Sites.FixAvailable > 0 && score >= 60:
		r.Action = models.AIActionUpdateNow
	case g.Sites.FixAvailable > 0:
		r.Action = models.AIActionUpdateThisWeek
	case g.ClosedOnWPOrg:
		r.Action = models.AIActionRemovePlugin
	case score < 30:
		r.Action = models.AIActionMuteCandidate
	default:
		r.Action = models.AIActionInvestigate
	}
	return r
}

func days(d time.Duration) int {
	if d < 0 {
		return 0
	}
	return int(d / (24 * time.Hour))
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func uniqueSorted(in []string) []string {
	m := map[string]bool{}
	for _, s := range in {
		m[s] = true
	}
	return sortedKeys(m)
}
