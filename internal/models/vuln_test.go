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
