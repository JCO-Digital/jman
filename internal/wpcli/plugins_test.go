package wpcli

import "testing"

func TestParsePluginDetails(t *testing.T) {
	out := `Warning: something noisy
[{"name":"akismet","title":"Akismet Anti-spam","version":"5.3","status":"active","author":"Automattic","update_version":"5.4"},{"name":"hello","title":"Hello Dolly","version":"1.7.2","status":"inactive","author":"Matt Mullenweg","update_version":""}]`
	got, err := parsePluginDetails(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Title != "Akismet Anti-spam" || got[0].UpdateVersion != "5.4" || got[1].Author != "Matt Mullenweg" {
		t.Errorf("parsePluginDetails = %+v", got)
	}
}
