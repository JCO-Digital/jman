package refresh

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/sitestate"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/JCO-Digital/jman/internal/wpcli"
)

// CollectAgentlessSites iterates over all managed sites in inventory.db that have
// WP-CLI enabled but do not have an agent installed, collecting wp-config flags
// and disk usage via remote WP-CLI/SSH. It also collects core version, plugins
// and users, but only for sites the slow refresh's SSH sweep doesn't already
// read (see cache.SSHSweptSiteIDs), since it runs right after that sweep.
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

	// nil (no site list) means none count as swept: collect everything.
	swept := cache.SSHSweptSiteIDs()

	var wg sync.WaitGroup
	sem := make(chan struct{}, 6) // bounded concurrency

	for _, s := range targetSites {
		wg.Add(1)
		go func(site models.ManagedSite) {
			defer wg.Done()
			defer wpcli.AcquireHost(site.ToCliSite().SSH)()
			sem <- struct{}{}
			defer func() { <-sem }()

			if err := collectSingleSite(site, !swept[site.ID]); err != nil {
				verb.LogPrintf(verb.Normal, "Agentless collection failed for %s: %v", site.Domain, err)
			}
		}(s)
	}

	wg.Wait()
	return nil
}

// wpFlagsEval reports the site's WordPress config flags and disk usage
// as JSON, in one WordPress start. The core auto-update setting mirrors
// what jman-agent reads from wp-config.php ("disabled", the
// WP_AUTO_UPDATE_CORE value, or "default"), except that with no
// WP_AUTO_UPDATE_CORE it also honours the wp-admin toggle for major
// updates (the auto_update_core_major option). Disk usage is POSIX
// `du -sk` of the WordPress root in bytes, 0 if it couldn't be measured,
// as when the server disables shell_exec.
const wpFlagsEval = `$auto = "default";
if (defined("AUTOMATIC_UPDATER_DISABLED") && AUTOMATIC_UPDATER_DISABLED) {
	$auto = "disabled";
} elseif (defined("WP_AUTO_UPDATE_CORE")) {
	// "beta", "rc", "development" and "branch-development" also allow
	// major updates.
	$auto = WP_AUTO_UPDATE_CORE === false || WP_AUTO_UPDATE_CORE === "false" ? "false" : (WP_AUTO_UPDATE_CORE === "minor" ? "minor" : "true");
} elseif (get_site_option("auto_update_core_major") === "enabled") {
	$auto = "true";
}
$disk = 0;
// A server can disable shell_exec; calling it then throws.
$du = function_exists("shell_exec") ? @shell_exec("du -sk . 2>/dev/null") : null;
if ($du && preg_match("/^(\\d+)/", $du, $m)) { $disk = ((int) $m[1]) * 1024; }
echo json_encode(["multisite" => is_multisite(), "disallow_file_mods" => defined("DISALLOW_FILE_MODS") && DISALLOW_FILE_MODS, "auto_update_core" => $auto, "disk_bytes" => $disk]);`

// siteFlags is wpFlagsEval's output.
type siteFlags struct {
	Multisite        bool   `json:"multisite"`
	DisallowFileMods bool   `json:"disallow_file_mods"`
	AutoUpdateCore   string `json:"auto_update_core"`
	DiskBytes        int64  `json:"disk_bytes"`
}

// parseSiteFlags parses wpFlagsEval output, skipping any PHP notices
// printed before the JSON.
func parseSiteFlags(output string) (siteFlags, error) {
	var flags siteFlags
	output = strings.TrimSpace(output)
	idx := strings.Index(output, "{")
	if idx == -1 {
		return flags, fmt.Errorf("no JSON object found in flags output")
	}
	err := json.Unmarshal([]byte(output[idx:]), &flags)
	return flags, err
}

// collectSingleSite reads a site's WordPress flags and disk usage, and with
// wpData also its core version, plugins and users.
func collectSingleSite(site models.ManagedSite, wpData bool) error {
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

	// 1. Collect WP flags (MULTISITE, DISALLOW_FILE_MODS, core
	// auto-updates) and disk usage.
	flagRes, flagErr := wpcli.RunWP(opts, "eval", wpFlagsEval)
	if flagErr == nil {
		if flags, err := parseSiteFlags(flagRes.Output); err == nil {
			_ = db.SetSiteWpFlags(site.ID, flags.Multisite, flags.DisallowFileMods, &flags.AutoUpdateCore)
			if flags.DiskBytes > 0 {
				_ = db.RecordSiteDiskUsage(site.ID, flags.DiskBytes, time.Now())
			}
		} else {
			verb.LogPrintf(verb.Normal, "Agentless collection: failed to parse flags of %s: %v", site.Domain, err)
		}
	}

	if !wpData {
		return nil
	}

	// 3. Collect WordPress Core Version
	coreVer, coreErr := wpcli.CoreVersion(cliSite)
	if coreErr == nil && coreVer != "" {
		minorUpdate, majorUpdate := checkCoreUpdates(cliSite)
		core := models.SiteCore{Version: coreVer, MinorUpdate: minorUpdate, MajorUpdate: majorUpdate}
		if _, err := sitestate.ApplyCore(site.ID, core, sitestate.SourceSSH, time.Now()); err != nil {
			verb.LogPrintf(verb.Normal, "Agentless collection: failed to save core version for %s: %v", site.Domain, err)
		}
	}

	// 4. Collect Installed Plugins
	plugins, plugErr := wpcli.GetPluginsWithUpdates(cliSite)
	if plugErr == nil {
		if _, err := sitestate.ApplyPlugins(site.ID, plugins, sitestate.SourceSSH, time.Now()); err != nil {
			verb.LogPrintf(verb.Normal, "Agentless collection: failed to save plugins for %s: %v", site.Domain, err)
		}
	}

	// 5. Collect Users
	users, userErr := wpcli.GetSiteUsers(cliSite)
	if userErr == nil {
		if _, err := sitestate.ApplyUsers(site.ID, users, sitestate.SourceSSH, time.Now()); err != nil {
			verb.LogPrintf(verb.Normal, "Agentless collection: failed to save users for %s: %v", site.Domain, err)
		}
	}

	return nil
}

func checkCoreUpdates(cliSite models.CliSite) (minorUpdate, majorUpdate string) {
	updates, err := wpcli.CheckCore(cliSite)
	if err != nil {
		return "", ""
	}
	return wpcli.SplitCoreUpdates(updates)
}
