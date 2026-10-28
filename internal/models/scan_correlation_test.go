package models

import (
	"strings"
	"testing"
)

func finding(rule, severity, path string) StoredScanFinding {
	f := StoredScanFinding{Status: ScanFindingOpen}
	f.Rule, f.Severity, f.Path = rule, severity, path
	return f
}

func TestCorrelateScanFindings(t *testing.T) {
	for _, c := range []struct {
		name     string
		findings []StoredScanFinding
		want     string // a part of the reason, or "" for none
	}{
		{"shell where none belongs", []StoredScanFinding{
			finding("core-unknown-file", "high", "wp-includes/class-wp-locale-helper.php"),
			finding("suspicious-code", "high", "wp-includes/class-wp-locale-helper.php"),
		}, "malicious code in a file that shouldn't be there: wp-includes/class-wp-locale-helper.php"},
		{"shell and a hidden admin", []StoredScanFinding{
			finding("core-unknown-file", "high", "x.php"),
			finding("new-admin", "high", "users/wpsvc_1d4e"),
		}, "compromise of code and accounts"},
		{"admin and a server backdoor", []StoredScanFinding{
			finding("new-app-password", "high", "users/a/app-passwords/u"),
			finding("disguised-process", "high", "process/[kaluad]"),
		}, "compromise of accounts and server"},
		{"two shells, one kind", []StoredScanFinding{
			finding("uploads-php", "high", "wp-content/uploads/a.php"),
			finding("suspicious-code", "high", "wp-content/themes/x/b.php"),
		}, ""},
		{"a staff admin (medium) and a change", []StoredScanFinding{
			finding("new-admin", "medium", "users/anna"),
			finding("uploads-php", "high", "wp-content/uploads/a.php"),
		}, ""},
		{"an editor's server (low) and a shell", []StoredScanFinding{
			finding("home-binary", "low", "~/.cursor-server/bin/node"),
			finding("uploads-php", "high", "wp-content/uploads/a.php"),
		}, ""},
	} {
		got := CorrelateScanFindings(c.findings)
		if (c.want == "") != (got == "") || !strings.Contains(got, c.want) {
			t.Errorf("%s: reason %q, want %q", c.name, got, c.want)
		}
	}

	// Only open findings count.
	closed := finding("suspicious-code", "high", "x.php")
	closed.Status = ScanFindingClosed
	if got := CorrelateScanFindings([]StoredScanFinding{finding("uploads-php", "high", "x.php"), closed}); got != "" {
		t.Errorf("closed finding correlated: %q", got)
	}
}
