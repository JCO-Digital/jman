package models

import "testing"

func TestVulnBackportMatches(t *testing.T) {
	rule := VulnBackport{Branch: "10.9", MinVersion: "10.9.5"}
	tests := []struct {
		ver  string
		want bool
	}{
		{"10.9.5", true},
		{"10.9.6", true},
		{"10.9.10", true},
		{"10.9.5.1", true},
		{"10.9.4", false},
		{"10.9", false},    // padded to 10.9.0, below the minimum
		{"10.90.5", false}, // numeric segments, not a string prefix
		{"10.10.0", false},
		{"11.0.0", false},
		{"10.9.5-beta1", false}, // pre-release sorts below the release
		{"not-a-version", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := rule.Matches(tt.ver); got != tt.want {
			t.Errorf("Matches(%q) = %v, want %v", tt.ver, got, tt.want)
		}
	}

	four := VulnBackport{Branch: "1.2.3", MinVersion: "1.2.3.4"}
	if !four.Matches("1.2.3.5") || four.Matches("1.2.3.3") || four.Matches("1.2.4.0") {
		t.Error("four-segment branch matched incorrectly")
	}
}

func TestVulnBackportValidate(t *testing.T) {
	valid := VulnBackport{Type: VulnBackportTypePlugin, Slug: "woocommerce", VulnUUID: "u1", Branch: "11.1", MinVersion: "11.1.2", Reason: "Backported"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid rule rejected: %v", err)
	}
	core := VulnBackport{Type: VulnBackportTypeCore, VulnUUID: "u1", Branch: "6.4", MinVersion: "6.4.5", Reason: "Backported"}
	if err := core.Validate(); err != nil {
		t.Fatalf("valid core rule rejected: %v", err)
	}

	tests := []struct {
		name string
		edit func(*VulnBackport)
	}{
		{"unknown type", func(b *VulnBackport) { b.Type = "theme" }},
		{"plugin without slug", func(b *VulnBackport) { b.Slug = "" }},
		{"slug with path", func(b *VulnBackport) { b.Slug = "../woocommerce" }},
		{"core with slug", func(b *VulnBackport) { b.Type = VulnBackportTypeCore }},
		{"missing uuid", func(b *VulnBackport) { b.VulnUUID = "" }},
		{"blank reason", func(b *VulnBackport) { b.Reason = "  " }},
		{"missing branch", func(b *VulnBackport) { b.Branch = "" }},
		{"non-numeric branch", func(b *VulnBackport) { b.Branch = "11.x" }},
		{"missing minimum", func(b *VulnBackport) { b.MinVersion = "" }},
		{"unparseable minimum", func(b *VulnBackport) { b.MinVersion = "../11.1.2" }},
		{"pre-release minimum", func(b *VulnBackport) { b.MinVersion = "11.1.2-rc1" }},
		{"prefixed minimum", func(b *VulnBackport) { b.MinVersion = "v11.1.2" }},
		{"minimum not longer than branch", func(b *VulnBackport) { b.MinVersion = "11.1" }},
		{"minimum outside branch", func(b *VulnBackport) { b.MinVersion = "11.10.2" }},
	}
	for _, tt := range tests {
		b := valid
		tt.edit(&b)
		if err := b.Validate(); err == nil {
			t.Errorf("%s: expected an error", tt.name)
		}
	}
}
