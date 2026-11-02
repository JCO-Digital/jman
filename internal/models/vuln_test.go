package models

import (
	"encoding/json"
	"testing"
)

func TestVulnDataClosed(t *testing.T) {
	for _, tt := range []struct {
		json string
		want bool
	}{
		{`{"closed": 1, "closed_reason": "security-issue", "closed_date": "2022-12-27"}`, true},
		{`{"closed": "1"}`, true},
		{`{"closed": 0}`, false},
		{`{"closed": "0"}`, false},
		{`{"closed": null}`, false},
		{`{}`, false},
	} {
		var d VulnData
		if err := json.Unmarshal([]byte(tt.json), &d); err != nil {
			t.Fatalf("%s: %v", tt.json, err)
		}
		if bool(d.Closed) != tt.want {
			t.Errorf("%s: Closed = %v, want %v", tt.json, d.Closed, tt.want)
		}
	}

	var d VulnData
	_ = json.Unmarshal([]byte(`{"closed": 1, "closed_reason": "security-issue", "closed_date": "2022-12-27"}`), &d)
	if d.ClosedReason == nil || *d.ClosedReason != "security-issue" || d.ClosedDate == nil || *d.ClosedDate != "2022-12-27" {
		t.Errorf("closed reason/date not parsed: %+v", d)
	}
}

func TestNormalizeCoreVulnerabilities(t *testing.T) {
	feed := func() []Vulnerability {
		return []Vulnerability{
			{Uuid: "shared", Name: "6.9.10", Source: []Source{{Id: "wf-1", Name: "WordPress Core 6.6.0 - 7.1.3 - Authenticated (Contributor+) Stored XSS"}}},
			{Uuid: "shared", Name: "6.9.10", Source: []Source{{Id: "wf-2", Name: "WordPress Core 4.9.0 - 7.1.3 - Unauthenticated Information Exposure"}}},
			{Uuid: "shared", Name: "6.9.10", Source: []Source{{Id: "CVE-2026-1", Name: "CVE-2026-1"}}},
			// A repeat of the first entry.
			{Uuid: "shared", Name: "6.9.10", Source: []Source{{Id: "wf-1", Name: "WordPress Core 6.6.0 - 7.1.3 - Authenticated (Contributor+) Stored XSS"}}},
		}
	}

	got := NormalizeCoreVulnerabilities(feed())
	if len(got) != 3 {
		t.Fatalf("got %d vulnerabilities, want 3 (the repeat merged)", len(got))
	}
	ids := map[string]bool{}
	for _, v := range got {
		if v.FeedUuid != "shared" {
			t.Errorf("FeedUuid = %q, want the feed's UUID", v.FeedUuid)
		}
		if len(v.Uuid) != 64 || v.Uuid == "shared" {
			t.Errorf("Uuid = %q, want a derived 64-character ID", v.Uuid)
		}
		ids[v.Uuid] = true
	}
	if len(ids) != 3 {
		t.Errorf("UUIDs are not distinct: %v", ids)
	}
	if got[0].Name != "WordPress Core 6.6.0 - 7.1.3 - Authenticated (Contributor+) Stored XSS" || got[2].Name != "CVE-2026-1" {
		t.Errorf("names = %q, %q; want the source names", got[0].Name, got[2].Name)
	}

	// The same vulnerability listed for another core version, under that
	// version's UUID, keeps its ID.
	other := NormalizeCoreVulnerabilities([]Vulnerability{
		{Uuid: "other-version", Name: "7.0.7", Source: []Source{{Id: "wf-1", Name: "x"}}},
	})
	if other[0].Uuid != got[0].Uuid {
		t.Errorf("ID differs across versions: %q vs %q", other[0].Uuid, got[0].Uuid)
	}

	// Normalizing again changes nothing.
	again := NormalizeCoreVulnerabilities(append([]Vulnerability(nil), got...))
	for i := range got {
		if again[i].Uuid != got[i].Uuid || again[i].FeedUuid != got[i].FeedUuid || again[i].Name != got[i].Name {
			t.Errorf("entry %d changed on a second pass: %+v vs %+v", i, again[i], got[i])
		}
	}

	// An entry without sources is left as the feed has it.
	bare := NormalizeCoreVulnerabilities([]Vulnerability{{Uuid: "u", Name: "6.9.10"}})
	if bare[0].Uuid != "u" || bare[0].FeedUuid != "" {
		t.Errorf("source-less entry changed: %+v", bare[0])
	}
}
