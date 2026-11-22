package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/fetch/wporg"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/sitestate"
	"github.com/JCO-Digital/jman/internal/utils"
	"github.com/JCO-Digital/jman/internal/verb"
)

// ruleUnknownPlugin is the finding jman-api raises for a plugin that
// doesn't come from wordpress.org or J&Co Digital. Most are premium or
// custom plugins, allowlisted once for the whole fleet; a backdoor dressed
// as a plugin is one too, and no file rule catches it: it has no checksums
// to differ from, and its code can be plain WordPress API calls.
const ruleUnknownPlugin = "unknown-plugin"

const (
	// pluginOriginOnWPOrgTTL is how long a slug found on wordpress.org is
	// trusted before asking again: plugins are rarely closed.
	pluginOriginOnWPOrgTTL = 30 * 24 * time.Hour
	// pluginOriginMissingTTL is how long jman-api remembers that
	// wordpress.org doesn't have a slug.
	pluginOriginMissingTTL = 24 * time.Hour
	// unknownPluginSweepInterval is how often every site's plugins are
	// checked, so plugins that were there before the check existed, or
	// before jman-api last started, are found.
	unknownPluginSweepInterval = 24 * time.Hour
)

// impersonatedPluginNames are plugin names and authors that claim to be
// WordPress itself, lowercased. wordpress.org's own plugins use them, so
// one that isn't on wordpress.org is lying.
var impersonatedPluginNames = map[string]bool{
	"wordpress": true, "wordpress.org": true, "wordpress core": true,
	"wordpress contributors": true, "wordpressdotorg": true, "the wordpress team": true,
}

var (
	// wporgPluginInfo looks a slug up on wordpress.org; tests replace it.
	wporgPluginInfo = wporg.GetPluginInfo
	// runPluginCheck runs a site's plugin check off the caller's request,
	// since a slug jman-api hasn't seen means asking wordpress.org; tests
	// run it in place.
	runPluginCheck = func(f func()) { go f() }
	// pluginCheckMu runs one site's plugin check at a time, so a sweep and
	// an agent report don't look up the same slugs at once.
	pluginCheckMu sync.Mutex
)

func init() {
	sitestate.PluginsHook = func(siteID string, plugins []models.WPPlugin, installed []string) {
		runPluginCheck(func() { checkSitePlugins(siteID, plugins, installed) })
	}
}

// StartUnknownPluginSweep checks every site's plugins now and daily.
func StartUnknownPluginSweep(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(unknownPluginSweepInterval)
		defer ticker.Stop()
		for {
			sweepUnknownPlugins()
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// sweepUnknownPlugins checks the stored plugins of every site.
func sweepUnknownPlugins() {
	all, err := db.GetAllSitePlugins()
	if err != nil {
		verb.LogPrintf(verb.Normal, "%v", err)
		return
	}
	bySite := map[string][]models.WPPlugin{}
	var order []string
	for _, p := range all {
		if _, seen := bySite[p.SiteID]; !seen {
			order = append(order, p.SiteID)
		}
		bySite[p.SiteID] = append(bySite[p.SiteID], p)
	}
	for _, siteID := range order {
		checkSitePlugins(siteID, bySite[siteID], nil)
	}
}

// checkSitePlugins raises an unknown-plugin finding for each of a site's
// regular plugins that isn't on wordpress.org or one of J&Co Digital's
// jcore-* plugins: high when it poses as WordPress itself, medium when it
// is in installed (it appeared without jman installing it), and low
// otherwise, as for a premium plugin that was already there. A finding
// keeps the highest severity it was reported with.
func checkSitePlugins(siteID string, plugins []models.WPPlugin, installed []string) {
	pluginCheckMu.Lock()
	defer pluginCheckMu.Unlock()

	reported, err := db.ListScanFindings(db.ScanFindingFilter{SiteID: siteID})
	if err != nil {
		verb.LogPrintf(verb.Normal, "%v", err)
		return
	}
	previous := map[string]string{}
	for _, f := range reported {
		if f.Rule == ruleUnknownPlugin {
			previous[f.Path] = f.Severity
		}
	}
	isNew := map[string]bool{}
	for _, slug := range installed {
		isNew[slug] = true
	}

	now := time.Now()
	count := 0
	for _, p := range plugins {
		if p.Status == "must-use" || p.Status == "dropin" || strings.HasPrefix(p.Name, "jcore-") {
			continue
		}
		onWPOrg, known := pluginOrigin(p.Name, now)
		if !known || onWPOrg {
			continue
		}
		info, _, err := db.GetPluginInfo(p.Name)
		if err != nil {
			verb.LogPrintf(verb.Normal, "%v", err)
		}
		f := unknownPluginFinding(p, info, isNew[p.Name])
		if prev := previous[f.Path]; config.ScanSeverityRank(prev) > config.ScanSeverityRank(f.Severity) {
			f.Severity = prev
		}
		if err := db.UpsertScanFinding(siteID, f, "", now); err != nil {
			verb.LogPrintf(verb.Normal, "%v", err)
			continue
		}
		count++
	}
	if count > 0 {
		triageScanFindings(siteID, true)
	}
}

// pluginOrigin returns whether wordpress.org has a slug, from the cache or
// by asking. known is false when it couldn't be found out.
func pluginOrigin(slug string, now time.Time) (onWPOrg, known bool) {
	cached, checkedAt, found, err := db.GetPluginOrigin(slug)
	if err != nil {
		verb.LogPrintf(verb.Normal, "%v", err)
		return false, false
	}
	ttl := pluginOriginMissingTTL
	if cached {
		ttl = pluginOriginOnWPOrgTTL
	}
	if found && now.Sub(checkedAt) < ttl {
		return cached, true
	}
	info, err := wporgPluginInfo(slug)
	if err != nil {
		verb.LogPrintf(verb.Verbose, "Couldn't check plugin %s on wordpress.org: %v", slug, err)
		// An old answer is better than none.
		return cached, found
	}
	onWPOrg = info != nil
	if err := db.SavePluginOrigin(slug, onWPOrg, now); err != nil {
		verb.LogPrintf(verb.Normal, "%v", err)
	}
	return onWPOrg, true
}

// impersonatesWordPress reports whether a plugin's name or author claims
// to be WordPress itself.
func impersonatesWordPress(info *models.PluginInfo) bool {
	if info == nil {
		return false
	}
	for _, s := range []string{info.Name, info.Author} {
		if impersonatedPluginNames[strings.ToLower(strings.TrimSpace(utils.CleanHTML(s)))] {
			return true
		}
	}
	return false
}

// unknownPluginFinding describes a plugin that isn't on wordpress.org.
// The hash identifies the slug, not the files, so one allowlist entry
// covers a premium plugin on every site and in every version.
func unknownPluginFinding(p models.WPPlugin, info *models.PluginInfo, appeared bool) models.ScanFinding {
	sum := sha256.Sum256([]byte(ruleUnknownPlugin + "\x00" + p.Name))
	name, author := p.Name, ""
	if info != nil {
		if s := utils.CleanHTML(info.Name); s != "" {
			name = s
		}
		author = utils.CleanHTML(info.Author)
	}
	f := models.ScanFinding{
		Engine:   "api",
		Rule:     ruleUnknownPlugin,
		Severity: "low",
		Path:     "wp-content/plugins/" + p.Name,
		SHA256:   hex.EncodeToString(sum[:]),
		Snippet:  fmt.Sprintf("plugin: %s\nname: %s\nauthor: %s\nversion: %s\nstatus: %s", p.Name, name, author, p.Version, p.Status),
		Detail:   fmt.Sprintf("plugin %s (%q) isn't on wordpress.org or from J&Co Digital; if it's a known premium or custom plugin, allowlist it once for every site", p.Name, name),
	}
	switch {
	case impersonatesWordPress(info):
		f.Severity = "high"
		f.Detail = fmt.Sprintf("plugin %s calls itself %q by %q, but WordPress's own plugins are on wordpress.org and this one isn't: backdoors pose as WordPress", p.Name, name, author)
	case appeared:
		f.Severity = "medium"
		f.Detail = fmt.Sprintf("plugin %s (%q) appeared on the site without jman installing it, and isn't on wordpress.org or from J&Co Digital", p.Name, name)
	}
	return f
}
