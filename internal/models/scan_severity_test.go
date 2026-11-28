package models

import "testing"

func TestIsMinorStrayFile(t *testing.T) {
	for p, want := range map[string]bool{
		"wp-content/uploads/2020/06/logo.png.bak": true,
		"wp-content/uploads/report.pdf.old":       true,
		".style.css.swp":                          true,
		"photo.JPG.orig.bak":                      true,
		"notes.txt.orig":                          false,
		"wp-config.php.bak":                       false,
		"wp-config.bak":                           false,
		"index.php.old":                           false,
		".index.php.swp":                          false,
		"shell.phtml.bak":                         false,
		".htaccess.orig":                          false,
		".htpasswd.bak":                           false,
		".user.ini.bak":                           false,
		"php.ini.bak":                             false,
		"config.inc.old":                          false,
		"wp-cli.yml.bak":                          false,
		"debug.log.old":                           false,
		"settings.json.bak":                       false,
		"server.key.bak":                          false,
		".env.bak":                                false,
		"backup.sql.bak":                          false,
		"site.zip.bak":                            false,
		"wordpress.tar.gz.old":                    false,
		"files.tgz.orig":                          false,
		"x.bak":                                   false,
		"backup.sql":                              false,
		"site.zip":                                false,
		".env":                                    false,
	} {
		if got := IsMinorStrayFile(p); got != want {
			t.Errorf("IsMinorStrayFile(%q) = %v, want %v", p, got, want)
		}
	}
}
