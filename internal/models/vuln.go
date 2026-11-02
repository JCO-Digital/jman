package models

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"github.com/JCO-Digital/jman/internal/utils"
)

type Cwe struct {
	Cwe         string `json:"cwe"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type Cvss struct {
	Version     string  `json:"version"`
	Vector      string  `json:"vector"`
	Av          string  `json:"av"`
	Ac          string  `json:"ac"`
	Pr          string  `json:"pr"`
	Ui          string  `json:"ui"`
	S           string  `json:"s"`
	C           string  `json:"c"`
	I           string  `json:"i"`
	A           string  `json:"a"`
	Score       string  `json:"score"`
	Severity    string  `json:"severity"`
	Exploitable string  `json:"exploitable"`
	Impact      *string `json:"impact"`
}

type Impact struct {
	Cvss *Cvss `json:"cvss,omitempty"`
	Cwe  []Cwe `json:"cwe,omitempty"`
}

// UnmarshalJSON custom unmarshaler for Impact to handle empty arrays.
func (i *Impact) UnmarshalJSON(data []byte) error {
	if string(data) == "[]" || string(data) == "null" {
		return nil
	}
	type Alias Impact
	aux := (*Alias)(i)
	return json.Unmarshal(data, aux)
}

type Source struct {
	Id          string  `json:"id"`
	Name        string  `json:"name"`
	Link        string  `json:"link"`
	Description *string `json:"description"`
	Date        *string `json:"date"`
}

type Operator struct {
	MinVersion  *string `json:"min_version"`
	MinOperator *string `json:"min_operator"`
	MaxVersion  *string `json:"max_version"`
	MaxOperator *string `json:"max_operator"`
	Unfixed     string  `json:"unfixed"`
	Closed      string  `json:"closed"`
}

type Vulnerability struct {
	Uuid string `json:"uuid"`
	// FeedUuid is the UUID the feed gave a WordPress core vulnerability,
	// which it shares with the version's other vulnerabilities; see
	// NormalizeCoreVulnerabilities. Empty for plugins.
	FeedUuid    string       `json:"feed_uuid,omitempty"`
	Name        string       `json:"name"`
	Description *string      `json:"description"`
	Operator    Operator     `json:"operator"`
	Source      []Source     `json:"source"`
	Impact      *Impact      `json:"impact,omitempty"`
	Sites       []PluginSite `json:"sites,omitempty"`
	// PatchedSites are sites inside the affected range that a backport rule
	// marks as not affected. Only kept while Sites is non-empty.
	PatchedSites []PluginSite `json:"patched_sites,omitempty"`
	Suppressed   bool         `json:"suppressed"`
}

type VulnData struct {
	Name   *string `json:"name"`
	Plugin string  `json:"plugin"`
	Link   *string `json:"link"`
	// Latest is the Unix timestamp of the plugin's last update, as a string;
	// it is not a version.
	Latest *string `json:"latest"`
	// Closed is set when the plugin was closed on wordpress.org, with the
	// reason (e.g. "security-issue") and date ("2006-01-02") it was closed.
	Closed        FeedFlag        `json:"closed"`
	ClosedReason  *string         `json:"closed_reason,omitempty"`
	ClosedDate    *string         `json:"closed_date,omitempty"`
	Vulnerability []Vulnerability `json:"vulnerability"`
	Suppressed    bool            `json:"suppressed"`
}

// FeedFlag is a boolean the wpvulnerability.net feed sends as a number
// (1), a string ("1") or not at all. Anything but 1/"1"/true is false.
type FeedFlag bool

// UnmarshalJSON accepts the number, string and boolean forms of the flag.
func (f *FeedFlag) UnmarshalJSON(data []byte) error {
	switch strings.Trim(string(data), `"`) {
	case "1", "true":
		*f = true
	default:
		*f = false
	}
	return nil
}

type VulnResponse struct {
	Error   int       `json:"error"`
	Message *string   `json:"message"`
	Data    *VulnData `json:"data,omitempty"`
	Updated any       `json:"updated"`
}

// CoreVulnData is the payload for a WordPress core vulnerability lookup,
// scoped to a single installed core version.
type CoreVulnData struct {
	Core          string          `json:"core"`
	Link          *string         `json:"link"`
	Vulnerability []Vulnerability `json:"vulnerability"`
}

// CoreVulnResponse mirrors VulnResponse for the wpvulnerability.net core endpoint.
type CoreVulnResponse struct {
	Error   int           `json:"error"`
	Message *string       `json:"message"`
	Data    *CoreVulnData `json:"data,omitempty"`
	Updated any           `json:"updated"`
}

// CoreVulnReport is the aggregated report for a single installed WordPress core
// version, listing the vulnerabilities affecting it and the sites running it.
type CoreVulnReport struct {
	Version         string          `json:"version"`
	Vulnerabilities []Vulnerability `json:"vulnerabilities"`
	Suppressed      bool            `json:"suppressed"`
}

// SiteCore represents the WordPress core version installed on a specific site,
// and any newer minor/major version available for it.
type SiteCore struct {
	SiteID      string `json:"site_id"` // site UUID
	Version     string `json:"version"`
	MinorUpdate string `json:"minor_update,omitempty"`
	MajorUpdate string `json:"major_update,omitempty"`
}

// CoreVersionData groups sites by installed WordPress core version.
type CoreVersionData struct {
	Version string       `json:"version"`
	Sites   []PluginSite `json:"sites"`
}

type VulnReport struct {
	Plugin          string          `json:"plugin"`
	Slug            string          `json:"slug"`
	PluginName      string          `json:"plugin_name"`
	Vulnerabilities []Vulnerability `json:"vulnerabilities"`
	Suppressed      bool            `json:"suppressed"`
}

type VulnPlugin struct {
	PluginName    string          `json:"plugin_name"`
	Version       string          `json:"version"`
	Cvss          *float64        `json:"cvss"`
	Vulnerability []Vulnerability `json:"vulnerability"`
}

// IgnoredVuln represents a vulnerability UUID that has been added to the ignore list.
type IgnoredVuln struct {
	UUID      string `json:"uuid"`
	Reason    string `json:"reason"`
	CreatedAt string `json:"created_at"`
}

// SanitizeVulnerabilities normalizes the free-text fields of vulnerabilities
// coming from the wpvulnerability.net feed, which serves names and descriptions
// containing HTML tags and entities. Mirrors SanitizePluginInfo, so consumers
// (CLI, Slack, API) all receive display-ready text.
func SanitizeVulnerabilities(vulns []Vulnerability) {
	for i := range vulns {
		v := &vulns[i]
		v.Name = utils.CleanHTML(v.Name)
		sanitizeStringPtr(&v.Description)
		for j := range v.Source {
			v.Source[j].Name = utils.CleanHTML(v.Source[j].Name)
			sanitizeStringPtr(&v.Source[j].Description)
		}
	}
}

// NormalizeCoreVulnerabilities gives each WordPress core vulnerability its
// own UUID and name. The wpvulnerability.net core feed lists all of a
// version's vulnerabilities under one UUID, named after the version; only
// their sources tell them apart.
//
// The UUID is derived from the source IDs, so a vulnerability keeps it
// across core versions and feed refreshes. The feed's UUID moves to
// FeedUuid, which ignore entries and backport rules made before this may
// still use. The name is the first source name that isn't a bare CVE ID,
// or else the first source name. Entries with the same sources are merged.
// Entries are normalized in place; normalizing twice changes nothing.
func NormalizeCoreVulnerabilities(vulns []Vulnerability) []Vulnerability {
	out := vulns[:0]
	seen := make(map[string]bool, len(vulns))
	for _, v := range vulns {
		if v.FeedUuid == "" {
			if len(v.Source) == 0 {
				// Nothing to tell it apart by; keep it as the feed has it.
				if !seen[v.Uuid] {
					seen[v.Uuid] = true
					out = append(out, v)
				}
				continue
			}
			v.FeedUuid = v.Uuid
			v.Uuid = coreVulnUUID(v.Source)
			v.Name = coreVulnName(v.Source, v.Name)
		}
		if seen[v.Uuid] {
			continue
		}
		seen[v.Uuid] = true
		out = append(out, v)
	}
	return out
}

// coreVulnUUID hashes a core vulnerability's source IDs (or, lacking IDs,
// links and names) into a stable 64-character hex ID like the feed's.
func coreVulnUUID(sources []Source) string {
	keys := make([]string, 0, len(sources))
	for _, s := range sources {
		switch {
		case s.Id != "":
			keys = append(keys, "id:"+s.Id)
		case s.Link != "":
			keys = append(keys, "link:"+s.Link)
		default:
			keys = append(keys, "name:"+s.Name)
		}
	}
	sort.Strings(keys)
	sum := sha256.Sum256([]byte("wordpress-core\n" + strings.Join(keys, "\n")))
	return hex.EncodeToString(sum[:])
}

func coreVulnName(sources []Source, fallback string) string {
	for _, s := range sources {
		if s.Name != "" && !strings.HasPrefix(s.Name, "CVE-") {
			return s.Name
		}
	}
	for _, s := range sources {
		if s.Name != "" {
			return s.Name
		}
	}
	return fallback
}

// SanitizeVulnData normalizes a plugin vulnerability payload, including the
// plugin display name the feed reports alongside the vulnerabilities.
func SanitizeVulnData(data *VulnData) {
	if data == nil {
		return
	}

	sanitizeStringPtr(&data.Name)
	SanitizeVulnerabilities(data.Vulnerability)
}

// sanitizeStringPtr applies CleanHTML in place to an optional string field.
func sanitizeStringPtr(s **string) {
	if *s == nil {
		return
	}
	cleaned := utils.CleanHTML(**s)
	*s = &cleaned
}
