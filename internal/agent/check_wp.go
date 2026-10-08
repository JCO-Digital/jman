package agent

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/JCO-Digital/jman/internal/models"
)

// CheckWPData runs the WordPress data collection for this server's sites
// once and prints what happened, without reporting anything to jman-api,
// self-updating or touching any local state. It works against any
// jman-api version, since it ignores the manifest's collection interval.
// domain limits it to one site. It returns how many sites failed.
func CheckWPData(ctx context.Context, cfg Config, domain string, out io.Writer) (int, error) {
	manifest, err := NewClient(cfg).FetchManifest(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch manifest: %w", err)
	}

	var sites []models.AgentManifestSite
	for _, site := range manifest.Sites {
		if site.IsWordpress && (domain == "" || strings.EqualFold(site.Domain, domain)) {
			sites = append(sites, site)
		}
	}
	if len(sites) == 0 {
		if domain != "" {
			return 0, fmt.Errorf("no WordPress site %q in this server's manifest", domain)
		}
		return 0, fmt.Errorf("no WordPress sites in this server's manifest")
	}

	runner, err := newRunner()
	if err != nil {
		return len(sites), fmt.Errorf("WordPress data collection is unavailable: %w", err)
	}
	if r, ok := runner.(*wpRunner); ok {
		fmt.Fprintf(out, "wp-cli: %s\n", r.wpPath)
	}
	if manifest.WPDataIntervalMinutes == 0 {
		fmt.Fprintln(out, "Note: this jman-api doesn't enable agent collection yet; the service won't collect until it does.")
	}

	failed := 0
	for _, site := range sites {
		fmt.Fprintf(out, "\n%s\n", site.Domain)
		if !checkSite(ctx, runner, site, out) {
			failed++
		}
	}
	fmt.Fprintf(out, "\n%d of %d site(s) collected", len(sites)-failed, len(sites))
	if failed > 0 {
		fmt.Fprintf(out, ", %d failed", failed)
	}
	fmt.Fprintln(out)
	return failed, nil
}

func checkSite(ctx context.Context, runner wpDataRunner, site models.AgentManifestSite, out io.Writer) bool {
	sitePath, err := resolvePath(site.Domain, site.SiteUser)
	if err != nil {
		fmt.Fprintf(out, "  FAILED: %v\n", err)
		return false
	}
	fmt.Fprintf(out, "  path:   %s\n", sitePath)
	id, err := resolveIdentity(sitePath, site.SiteUser)
	if err != nil {
		fmt.Fprintf(out, "  FAILED: %v\n", err)
		return false
	}
	fmt.Fprintf(out, "  runs as: %s (uid %d, gid %d, home %s)\n", id.Username, id.UID, id.GID, id.Home)

	data := collectSiteWPData(ctx, runner, site)
	if data.Error != "" {
		fmt.Fprintf(out, "  FAILED: %s\n", data.Error)
		return false
	}
	if data.Plugins == nil {
		fmt.Fprintln(out, "  ok: unchanged, matches jman-api's data (only the hash would be sent)")
		return true
	}
	regular, updates := 0, 0
	for _, p := range data.Plugins {
		if p.Status != "must-use" && p.Status != "dropin" {
			regular++
		}
		if p.Update != "" {
			updates++
		}
	}
	core := data.Core.Version
	if data.Core.MinorUpdate != "" {
		core += ", minor update " + data.Core.MinorUpdate
	}
	if data.Core.MajorUpdate != "" {
		core += ", major update " + data.Core.MajorUpdate
	}
	fmt.Fprintf(out, "  ok: %d plugins (%d regular, %d with updates), WordPress %s\n", len(data.Plugins), regular, updates, core)
	fmt.Fprintln(out, "  differs from jman-api's data (would be sent in full)")
	return true
}
