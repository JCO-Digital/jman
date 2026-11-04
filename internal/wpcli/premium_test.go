package wpcli

import (
	"os/exec"
	"strings"
	"testing"
)

// The snippet runs on every site's plugin list and update, so a syntax
// error would break them all; lint it where PHP is installed.
func TestPremiumUpdatesPHPLints(t *testing.T) {
	php, err := exec.LookPath("php")
	if err != nil {
		t.Skip("php not installed")
	}
	cmd := exec.Command(php, "-l")
	cmd.Stdin = strings.NewReader("<?php " + premiumUpdatesPHP)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("php -l: %v\n%s", err, out)
	}
	if !strings.HasPrefix(PremiumUpdatesArg(), "--exec=") {
		t.Errorf("PremiumUpdatesArg() = %q", PremiumUpdatesArg())
	}
}
