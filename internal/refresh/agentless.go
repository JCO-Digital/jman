package refresh

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/JCO-Digital/jman/internal/wpcli"
)

// CollectAgentlessSites iterates over all managed sites in inventory.db that have
// WP-CLI enabled but do not have an agent installed, collecting wp-config flags,
// disk usage, core version, and installed plugins via remote WP-CLI/SSH.
func CollectAgentlessSites() error {
	sites, err := db.ListManagedSites()
	if err != nil {
		return fmt.Errorf("failed to list managed sites for agentless collection: %w", err)
	}

	var targetSites []models.ManagedSite
	for _, s := range sites {
		if s.CanWPCLI && !s.HasAgent && s.SSHHost != "" && s.Status == "active" {
			targetSites = append(targetSites, s)
		}
	}

	if len(targetSites) == 0 {
		return nil
	}

	verb.Printf(verb.Normal, "Starting agentless collection for %d sites...\n", len(targetSites))

	var wg sync.WaitGroup
	sem := make(chan struct{}, 6) // bounded concurrency

	for _, s := range targetSites {
		wg.Add(1)
		go func(site models.ManagedSite) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if err := collectSingleSite(site); err != nil {
				verb.LogPrintf(verb.Normal, "Agentless collection failed for %s: %v", site.Domain, err)
			}
		}(s)
	}

	wg.Wait()
	return nil
}

func collectSingleSite(site models.ManagedSite) error {
	sshSpec := fmt.Sprintf("%s@%s", site.SSHUser, site.SSHHost)
	if site.SSHPort > 0 && site.SSHPort != 22 {
		sshSpec = fmt.Sprintf("%s@%s:%d", site.SSHUser, site.SSHHost, site.SSHPort)
	}

	opts := wpcli.CliOptions{
		SSH:     sshSpec,
		Path:    site.SitePath,
		Timeout: 45 * time.Second,
	}
	cliSite := site.ToCliSite()

	// 1. Collect WP Flags (MULTISITE, DISALLOW_FILE_MODS)
	flagCmd := `echo json_encode(["multisite" => is_multisite(), "disallow_file_mods" => defined("DISALLOW_FILE_MODS") && DISALLOW_FILE_MODS]);`
	flagRes, flagErr := wpcli.RunWP(opts, "eval", flagCmd)
	if flagErr == nil && flagRes.Output != "" {
		var flags struct {
			Multisite        bool `json:"multisite"`
			DisallowFileMods bool `json:"disallow_file_mods"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(flagRes.Output)), &flags); err == nil {
			_ = db.SetSiteWpFlags(site.ID, flags.Multisite, flags.DisallowFileMods)
		}
	}

	// 2. Collect Disk Usage (via POSIX du -sk in WP root)
	diskCmd := `$out = @shell_exec("du -sk . 2>/dev/null"); if ($out && preg_match("/^(\\d+)/", $out, $m)) { echo ((int)$m[1]) * 1024; } else { echo 0; }`
	diskRes, diskErr := wpcli.RunWP(opts, "eval", diskCmd)
	if diskErr == nil && diskRes.Output != "" {
		if bytesUsed, parseErr := strconv.ParseInt(strings.TrimSpace(diskRes.Output), 10, 64); parseErr == nil && bytesUsed > 0 {
			_ = db.RecordSiteDiskUsage(site.ID, bytesUsed, time.Now())
		}
	}

	// 3. Collect WordPress Core Version
	coreVer, coreErr := wpcli.CoreVersion(cliSite)
	if coreErr == nil && coreVer != "" {
		minorUpdate, majorUpdate := checkCoreUpdates(cliSite)
		_ = db.SaveSiteCore(site.ID, coreVer, minorUpdate, majorUpdate)
	}

	// 4. Collect Installed Plugins
	plugins, plugErr := wpcli.GetPlugins(cliSite, false)
	if plugErr == nil {
		_ = db.DeleteSitePlugins(site.ID)
		for _, p := range plugins {
			p.SiteID = site.ID
			_ = db.SaveSitePlugin(p)
		}
	}

	return nil
}

func checkCoreUpdates(cliSite models.CliSite) (minorUpdate, majorUpdate string) {
	updates, err := wpcli.CheckCore(cliSite)
	if err != nil {
		return "", ""
	}
	for _, u := range updates {
		switch u.UpdateType {
		case "minor":
			if minorUpdate == "" {
				minorUpdate = u.Version
			}
		case "major":
			if majorUpdate == "" {
				majorUpdate = u.Version
			}
		}
	}
	return minorUpdate, majorUpdate
}
