package models

import "testing"

func TestIsMinorStrayFile(t *testing.T) {
	for p, want := range map[string]bool{
		"wp-content/uploads/2020/06/logo.png.bak": true,
		"wp-content/uploads/report.pdf.old":       true,
		"notes.txt.orig":                          true,
		".style.css.swp":                          true,
		"wp-config.php.bak":                       false,
		"wp-config.bak":                           false,
		"index.php.old":                           false,
		".index.php.swp":                          false,
		"shell.phtml.bak":                         false,
		".htaccess.orig":                          false,
		".user.ini.bak":                           false,
		".env.bak":                                false,
		"backup.sql.bak":                          false,
		"backup.sql":                              false,
		"site.zip":                                false,
		".env":                                    false,
	} {
		if got := IsMinorStrayFile(p); got != want {
			t.Errorf("IsMinorStrayFile(%q) = %v, want %v", p, got, want)
		}
	}
}
