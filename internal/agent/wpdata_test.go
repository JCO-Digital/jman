package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

func stubUsers(t *testing.T, users ...*user.User) {
	t.Helper()
	oldLookup, oldLookupID := lookupUser, lookupUserID
	lookupUser = func(name string) (*user.User, error) {
		for _, u := range users {
			if u.Username == name {
				return u, nil
			}
		}
		return nil, user.UnknownUserError(name)
	}
	lookupUserID = func(uid string) (*user.User, error) {
		for _, u := range users {
			if u.Uid == uid {
				return u, nil
			}
		}
		return nil, user.UnknownUserIdError(0)
	}
	t.Cleanup(func() { lookupUser, lookupUserID = oldLookup, oldLookupID })
}

func TestIdentityForUID(t *testing.T) {
	site := &user.User{Uid: "1001", Gid: "1001", Username: "example", HomeDir: "/sites/example.com"}
	other := &user.User{Uid: "1002", Gid: "1002", Username: "other", HomeDir: "/home/other"}
	rootGroup := &user.User{Uid: "1003", Gid: "0", Username: "wheelie", HomeDir: "/home/wheelie"}
	stubUsers(t, site, other, rootGroup)

	cases := []struct {
		name         string
		uid          uint32
		manifestUser string
		wantErr      string
	}{
		{"owner", 1001, "example", ""},
		{"no manifest user", 1001, "", ""},
		// SpinupWP's site_user is often an SFTP-only account that doesn't
		// exist locally; that's no reason to refuse.
		{"manifest user unknown locally", 1001, "sftp-only", ""},
		{"root-owned", 0, "example", "owned by root"},
		{"owner without account", 4242, "", "has no account"},
		{"root primary group", 1003, "", "root as its primary group"},
		{"owner differs from site user", 1001, "other", "but the site's user is other"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			id, err := identityForUID(c.uid, "/sites/example.com/files", c.manifestUser)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if id.UID != 1001 || id.GID != 1001 || id.Username != "example" || id.Home != "/sites/example.com" {
					t.Errorf("identity = %+v", id)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("err = %v, want one containing %q", err, c.wantErr)
			}
		})
	}
}

func TestResolveSiteIdentityRefusesSymlinksAndFiles(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "files")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveSiteIdentity(link, ""); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Errorf("symlink: err = %v", err)
	}
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveSiteIdentity(file, ""); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("file: err = %v", err)
	}

	// A directory owned by an ordinary account resolves to it.
	me, err := user.Current()
	if err != nil {
		t.Skip(err)
	}
	id, err := resolveSiteIdentity(target, "")
	if me.Uid == "0" {
		if err == nil {
			t.Error("a root-owned directory resolved")
		}
		return
	}
	if err != nil || strconv.FormatUint(uint64(id.UID), 10) != me.Uid {
		t.Errorf("identity = %+v, %v; want uid %s", id, err, me.Uid)
	}
}

func TestResolveSitePathRejectsBadDomains(t *testing.T) {
	for _, d := range []string{"../../root", "example.com/../x", "", "localhost", "a/b.com", ".example.com"} {
		if _, err := ResolveSitePath(d, ""); err == nil || !strings.Contains(err.Error(), "invalid site domain") {
			t.Errorf("ResolveSitePath(%q) err = %v, want invalid domain", d, err)
		}
	}
}

func TestCappedBuffer(t *testing.T) {
	b := &cappedBuffer{max: 5}
	b.Write([]byte("abc"))
	if b.truncated {
		t.Fatal("truncated too early")
	}
	b.Write([]byte("defg"))
	if !b.truncated || b.buf.String() != "abcde" {
		t.Errorf("buffer = %q truncated=%v", b.buf.String(), b.truncated)
	}
}

// fakeRunner answers wp-cli calls from a map of joined args to output.
type fakeRunner struct {
	out   map[string]string
	fail  map[string]error
	calls []string
	ids   []siteIdentity
}

func (f *fakeRunner) run(_ context.Context, id siteIdentity, _ string, args ...string) (string, error) {
	key := strings.Join(args, " ")
	f.calls = append(f.calls, key)
	f.ids = append(f.ids, id)
	if err := f.fail[key]; err != nil {
		return "", err
	}
	return f.out[key], nil
}

func stubSite(t *testing.T, runner *fakeRunner) {
	t.Helper()
	oldPath, oldID, oldRunner := resolvePath, resolveIdentity, newRunner
	resolvePath = func(domain, _ string) (string, error) { return "/sites/" + domain + "/files", nil }
	resolveIdentity = func(string, string) (siteIdentity, error) {
		return siteIdentity{UID: 1001, GID: 1001, Username: "example", Home: "/sites/example.com"}, nil
	}
	newRunner = func() (wpDataRunner, error) { return runner, nil }
	lastWPCollectionMu.Lock()
	lastWPCollection = map[string]time.Time{}
	lastWPCollectionMu.Unlock()
	t.Cleanup(func() { resolvePath, resolveIdentity, newRunner = oldPath, oldID, oldRunner })
}

const pluginListJSON = `Deprecated: something on line 3
[{"name":"akismet","status":"active","update":"none","version":"5.0","update_version":"","auto_update":"off"},{"name":"woocommerce","status":"active","update":"available","version":"9.3.0","update_version":"9.4.1","auto_update":"on"}]`

func okRunner() *fakeRunner {
	return &fakeRunner{out: map[string]string{
		"plugin list --format=json":       pluginListJSON,
		"core version":                    "Deprecated: notice\n6.6.1\n",
		"core check-update --format=json": `[{"version":"6.6.2","update_type":"minor","package_url":"x"},{"version":"6.7.1","update_type":"major","package_url":"y"}]`,
	}}
}

func TestCollectSiteWPData(t *testing.T) {
	runner := okRunner()
	stubSite(t, runner)
	site := models.AgentManifestSite{SiteID: "s1", Domain: "example.com", IsWordpress: true}

	data := collectSiteWPData(context.Background(), runner, site)
	if data.Error != "" {
		t.Fatalf("error: %s", data.Error)
	}
	if len(data.Plugins) != 2 || data.Plugins[1].Update != "9.4.1" || !data.Plugins[1].AutoUpdate {
		t.Errorf("plugins = %+v", data.Plugins)
	}
	if data.Core == nil || *data.Core != (models.SiteCore{Version: "6.6.1", MinorUpdate: "6.6.2", MajorUpdate: "6.7.1"}) {
		t.Errorf("core = %+v", data.Core)
	}
	if data.Hash != models.WPDataHash(data.Plugins, *data.Core) {
		t.Error("hash doesn't match the data")
	}
	for _, id := range runner.ids {
		if id.UID == 0 {
			t.Error("wp-cli ran as root")
		}
	}

	// With jman-api already holding this state, only the hash is sent.
	site.WPDataHash = data.Hash
	again := collectSiteWPData(context.Background(), okRunner(), site)
	if again.Hash != data.Hash || again.Plugins != nil || again.Core != nil {
		t.Errorf("unchanged data was sent in full: %+v", again)
	}
}

func TestCollectSiteWPDataReportsErrors(t *testing.T) {
	runner := okRunner()
	runner.fail = map[string]error{"plugin list --format=json": errors.New("wp-cli [plugin list] failed: Error: Error establishing a database connection.")}
	stubSite(t, runner)
	data := collectSiteWPData(context.Background(), runner, models.AgentManifestSite{SiteID: "s1", Domain: "example.com"})
	if !strings.Contains(data.Error, "database connection") || data.Hash != "" || data.Plugins != nil {
		t.Errorf("data = %+v", data)
	}

	// Refusing to drop privileges is reported too.
	resolveIdentity = func(string, string) (siteIdentity, error) {
		return siteIdentity{}, fmt.Errorf("refusing to run wp-cli: /sites/example.com/files is owned by root")
	}
	data = collectSiteWPData(context.Background(), okRunner(), models.AgentManifestSite{SiteID: "s1", Domain: "example.com"})
	if !strings.Contains(data.Error, "owned by root") {
		t.Errorf("data = %+v", data)
	}

	// A failed update check still reports the installed state.
	runner = okRunner()
	runner.fail = map[string]error{"core check-update --format=json": errors.New("offline")}
	resolveIdentity = func(string, string) (siteIdentity, error) { return siteIdentity{UID: 1001, GID: 1001}, nil }
	data = collectSiteWPData(context.Background(), runner, models.AgentManifestSite{SiteID: "s1", Domain: "example.com"})
	if data.Error != "" || data.Core == nil || data.Core.Version != "6.6.1" || data.Core.MajorUpdate != "" {
		t.Errorf("data = %+v", data)
	}
}

func TestCollectWPDataForSitesScheduling(t *testing.T) {
	runner := okRunner()
	stubSite(t, runner)
	manifest := &models.AgentManifest{
		WPDataIntervalMinutes: 60,
		Sites: []models.AgentManifestSite{
			{SiteID: "wp", Domain: "example.com", IsWordpress: true},
			{SiteID: "static", Domain: "static.example.com", IsWordpress: false},
		},
	}
	now := time.Now()

	data, done := collectWPDataForSites(context.Background(), manifest, now)
	if len(data) != 1 || data["wp"] == nil || len(done) != 1 {
		t.Fatalf("data = %v, done = %v", data, done)
	}
	// Not marked until the report is sent: still due.
	if data, _ := collectWPDataForSites(context.Background(), manifest, now); data["wp"] == nil {
		t.Error("a site whose report wasn't sent is no longer due")
	}
	markWPCollected(done, now)
	if data, _ := collectWPDataForSites(context.Background(), manifest, now.Add(30*time.Minute)); len(data) != 0 {
		t.Errorf("collected again after 30 minutes: %v", data)
	}
	if data, _ := collectWPDataForSites(context.Background(), manifest, now.Add(59*time.Minute+30*time.Second)); data["wp"] == nil {
		t.Error("not collected again after the interval")
	}

	// An older jman-api (no interval) turns collection off.
	manifest.WPDataIntervalMinutes = 0
	if data, _ := collectWPDataForSites(context.Background(), manifest, now.Add(24*time.Hour)); data != nil {
		t.Errorf("collected without an interval: %v", data)
	}
}

func TestCollectWPDataForSitesWithoutRunner(t *testing.T) {
	stubSite(t, okRunner())
	newRunner = func() (wpDataRunner, error) { return nil, errors.New("wp-cli not found") }
	manifest := &models.AgentManifest{WPDataIntervalMinutes: 60, Sites: []models.AgentManifestSite{{SiteID: "wp", Domain: "example.com", IsWordpress: true}}}
	data, done := collectWPDataForSites(context.Background(), manifest, time.Now())
	if data["wp"] == nil || data["wp"].Error != "wp-cli not found" || len(done) != 1 {
		t.Errorf("data = %v, done = %v", data, done)
	}
}

func TestCollectWPDataForSitesCapsPayloads(t *testing.T) {
	stubSite(t, okRunner())
	newRunner = func() (wpDataRunner, error) { return okRunner(), nil }
	manifest := &models.AgentManifest{WPDataIntervalMinutes: 60}
	for i := range maxWPDataPayloads + 5 {
		manifest.Sites = append(manifest.Sites, models.AgentManifestSite{SiteID: fmt.Sprintf("s%d", i), Domain: "example.com", IsWordpress: true})
	}
	data, done := collectWPDataForSites(context.Background(), manifest, time.Now())
	if len(data) != maxWPDataPayloads || len(done) != maxWPDataPayloads {
		t.Errorf("sent %d payloads, marked %d done; want %d", len(data), len(done), maxWPDataPayloads)
	}
}

func TestCheckRootOwned(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("files created by root are root-owned")
	}
	f := filepath.Join(t.TempDir(), "wp")
	if err := os.WriteFile(f, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := checkRootOwned(f); err == nil || !strings.Contains(err.Error(), "not root") {
		t.Errorf("err = %v, want a refusal of a non-root-owned binary", err)
	}
}

func TestFirstErrorLine(t *testing.T) {
	stderr := "PHP Deprecated: x\nError: Error establishing a database connection.\nmore"
	if got := firstErrorLine(stderr); got != "Error: Error establishing a database connection." {
		t.Errorf("got %q", got)
	}
	if got := firstErrorLine("warning one\nlast words\n\n"); got != "last words" {
		t.Errorf("got %q", got)
	}
}
