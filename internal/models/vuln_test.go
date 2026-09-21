package models

import "testing"

func TestSanitizeVulnerabilities(t *testing.T) {
	desc := "The <strong>Gravity Forms</strong> plugin is vulnerable.  "
	srcDesc := "Affects versions &lt; 3.1.1"

	vulns := []Vulnerability{
		{
			Name:        "Slider by 10Web &#8211; Responsive Image Slider",
			Description: &desc,
			Source: []Source{
				{Name: "Gravity Forms &lt;= 3.1.0.4 &#8211; Upload", Description: &srcDesc},
			},
		},
	}

	SanitizeVulnerabilities(vulns)

	if got, want := vulns[0].Name, "Slider by 10Web - Responsive Image Slider"; got != want {
		t.Errorf("Name = %q, want %q", got, want)
	}
	if got, want := *vulns[0].Description, "The Gravity Forms plugin is vulnerable."; got != want {
		t.Errorf("Description = %q, want %q", got, want)
	}
	if got, want := vulns[0].Source[0].Name, "Gravity Forms <= 3.1.0.4 - Upload"; got != want {
		t.Errorf("Source.Name = %q, want %q", got, want)
	}
	if got, want := *vulns[0].Source[0].Description, "Affects versions < 3.1.1"; got != want {
		t.Errorf("Source.Description = %q, want %q", got, want)
	}
}

// A nil Description is the common case for feed entries that only carry
// per-source text, so it must not panic.
func TestSanitizeVulnerabilitiesNilDescription(t *testing.T) {
	vulns := []Vulnerability{
		{Name: "No description", Source: []Source{{Name: "CVE-2026-84434"}}},
	}

	SanitizeVulnerabilities(vulns)

	if vulns[0].Description != nil {
		t.Errorf("Description = %v, want nil", *vulns[0].Description)
	}
	if got, want := vulns[0].Source[0].Name, "CVE-2026-84434"; got != want {
		t.Errorf("Source.Name = %q, want %q", got, want)
	}
}

func TestSanitizeVulnData(t *testing.T) {
	name := "Slider by 10Web &#8211; Responsive Image Slider"
	data := &VulnData{
		Name:          &name,
		Vulnerability: []Vulnerability{{Name: "XSS &amp; CSRF"}},
	}

	SanitizeVulnData(data)

	if got, want := *data.Name, "Slider by 10Web - Responsive Image Slider"; got != want {
		t.Errorf("Name = %q, want %q", got, want)
	}
	if got, want := data.Vulnerability[0].Name, "XSS & CSRF"; got != want {
		t.Errorf("Vulnerability.Name = %q, want %q", got, want)
	}

	// A nil payload is what the feed returns for unknown plugins.
	SanitizeVulnData(nil)
}
