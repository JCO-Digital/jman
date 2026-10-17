package models

import "testing"

func TestWPDataHashUsers(t *testing.T) {
	plugins := []WPPlugin{{Name: "akismet", Status: "active", Version: "5.0"}}
	core := SiteCore{Version: "6.6.1"}
	base := WPDataHash(plugins, core, nil)
	// Without users, the hash is the one agents computed before users
	// were collected.
	if want := "3dba4cbb289692b05d913f2cbde0397fcd4287bdda6adf251fa2ab72c4214b91"; base != want {
		t.Fatalf("hash = %q, want %q", base, want)
	}

	a := &SiteUsers{Users: []SiteUser{{ID: 1, Login: "a"}, {ID: 2, Login: "b"}}, ExcludedCounts: map[string]int{"subscriber": 3, "customer": 1}}
	b := &SiteUsers{Users: []SiteUser{{ID: 2, Login: "b", SiteID: "x"}, {ID: 1, Login: "a"}}, ExcludedCounts: map[string]int{"customer": 1, "subscriber": 3}}
	if WPDataHash(plugins, core, a) == base {
		t.Error("users don't change the hash")
	}
	if WPDataHash(plugins, core, a) != WPDataHash(plugins, core, b) {
		t.Error("the hash depends on user order or site ID")
	}
	b.ExcludedCounts["subscriber"] = 4
	if WPDataHash(plugins, core, a) == WPDataHash(plugins, core, b) {
		t.Error("counts don't change the hash")
	}
	if WPDataHash(plugins, core, &SiteUsers{}) == base {
		t.Error("an empty user list hashes like no users")
	}
}
