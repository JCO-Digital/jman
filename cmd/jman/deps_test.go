package main

import (
	"os/exec"
	"strings"
	"testing"
)

// forbiddenDeps are packages the jman CLI must not link: it gets its data
// from jman-api, and the database code belongs to jman-api alone (see
// docs/cli-api-client.md).
var forbiddenDeps = []string{
	"github.com/JCO-Digital/jman/internal/db",
	"github.com/JCO-Digital/jman/internal/cache",
	"github.com/JCO-Digital/jman/internal/fetch/",
	"github.com/JCO-Digital/jman/internal/sitestate",
	"github.com/JCO-Digital/jman/internal/vuln",
	"github.com/JCO-Digital/jman/internal/slack",
	"github.com/JCO-Digital/jman/internal/api",
	"modernc.org/sqlite",
}

func TestCLIHasNoServerDependencies(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Skipf("go list unavailable: %v", err)
	}
	for _, dep := range strings.Fields(string(out)) {
		for _, bad := range forbiddenDeps {
			// internal/vuln must not match internal/vulnview.
			if dep == bad || (strings.HasSuffix(bad, "/") && strings.HasPrefix(dep, bad)) || strings.HasPrefix(dep, bad+"/") {
				t.Errorf("jman depends on %s; the CLI must get its data from jman-api", dep)
			}
		}
	}
}
