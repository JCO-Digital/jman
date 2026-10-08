package wpcli

import "testing"

func TestIsWordPressOrgPackage(t *testing.T) {
	cases := map[string]bool{
		"https://downloads.wordpress.org/plugin/smtp2go.1.17.1.zip":                              true,
		"https://DOWNLOADS.wordpress.org/plugin/x.zip":                                           true,
		"http://downloads.wordpress.org/plugin/x.zip":                                            false,
		"https://github.com/JCO-Digital/jcore-pakkaus/releases/download/v2.2.0/x.zip":            false,
		"https://downloads.wordpress.org.example.com/plugin/x.zip":                               false,
		"https://update.wpallimport.com/serve_package?package_id=1&host=downloads.wordpress.org": false,
		"": false,
	}
	for pkg, want := range cases {
		if got := IsWordPressOrgPackage(pkg); got != want {
			t.Errorf("IsWordPressOrgPackage(%q) = %v, want %v", pkg, got, want)
		}
	}
}
