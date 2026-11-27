package wpcli

import (
	"regexp"
	"strings"
)

// Failure causes of a plugin or core update, in models.UpdateResult.Cause:
// what a failed update's wp-cli output says went wrong, for a short label.
const (
	// CauseRequirements: the update needs a newer WordPress or PHP. The
	// detail is what it needs, e.g. "WordPress 7.0" or "PHP 8.0".
	CauseRequirements = "requirements"
	// CauseNoPackage: WordPress knows of the update but has no download
	// for it, which for a premium plugin nearly always means its license
	// isn't active on the site (missing, expired or for another domain).
	CauseNoPackage = "no_package"
	// CauseDownloadFailed: the download failed. The detail is the HTTP
	// reason or error after "Download failed.", e.g. "Unauthorized".
	CauseDownloadFailed = "download_failed"
	// CauseSiteError: WordPress itself failed with a PHP fatal error, so
	// nothing could be updated. The detail is the plugin, theme or
	// mu-plugin the error happened in ("plugins/polylang-wc"), if known.
	CauseSiteError = "site_error"
	// CauseFiles: files couldn't be written, copied or removed (file
	// permissions, a full disk, a leftover directory).
	CauseFiles = "files"
	// CauseBadPackage: the downloaded package isn't an installable plugin
	// (a broken ZIP, or a page instead of a ZIP).
	CauseBadPackage = "bad_package"
	// CauseUpdateLocked: WordPress was already running another update.
	CauseUpdateLocked = "update_locked"
	// CauseTimeout: wp-cli didn't finish in time.
	CauseTimeout = "timeout"
	// CauseConnection: the server couldn't be reached over SSH.
	CauseConnection = "connection"
)

// englishMessages are WordPress's messages FailureCause recognizes, as
// they are in its source. EnglishMessagesArg keeps them untranslated, so
// a Finnish or Swedish site's output can be read too.
var englishMessages = []string{
	"Update package not available.",
	"Installation package not available.",
	"Package not available.",
	"Download failed.",
	"Another update is currently in progress.",
	"The package could not be installed.",
	"The package contains no files.",
	"The plugin contains no files.",
	"No valid plugins were found.",
	"Could not copy file.",
	"Could not copy files.",
	"Could not copy files. You may have run out of disk space.",
	"Could not create directory.",
	"Could not remove the old plugin.",
	"Could not remove the current plugin.",
	"Could not access filesystem.",
	"Destination folder already exists.",
	"The update cannot be installed because some files could not be copied. This is usually due to inconsistent file permissions.",
	"Your WordPress version is %1$s, however the new plugin version requires %2$s.",
	"The PHP version on your server is %1$s, however the new plugin version requires %2$s.",
	"There has been a critical error on this website.",
}

// englishMessagesPHP runs through wp-cli's --exec and keeps the
// englishMessages untranslated: only those, in WordPress's own text
// domain, and only for display. Everything else, language packs included,
// stays as it was. It must not fail on old WP-CLI versions without
// add_wp_hook.
var englishMessagesPHP = func() string {
	quoted := make([]string, len(englishMessages))
	for i, m := range englishMessages {
		quoted[i] = "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(m) + "'"
	}
	return `if (method_exists('WP_CLI', 'add_wp_hook')) { WP_CLI::add_wp_hook('gettext', function ($translation, $text, $domain) { ` +
		`static $keep = null; if (null === $keep) { $keep = array_flip([` + strings.Join(quoted, ", ") + `]); } ` +
		`return ('default' === $domain && isset($keep[$text])) ? $text : $translation; }, 10, 3); }`
}()

// EnglishMessagesArg is the wp-cli global argument that keeps the
// messages FailureCause reads in English (see englishMessagesPHP).
func EnglishMessagesArg() string {
	return "--exec=" + englishMessagesPHP
}

var (
	// "wordpress-seo: This update requires WordPress version 7.0, but …"
	// (wp-cli), "Your WordPress version is 6.9, however the new plugin
	// version requires 7.0." (WordPress).
	requiresWP        = regexp.MustCompile(`requires WordPress version ([\d.]+)|new plugin version requires ([\d.]+)\.`)
	requiresPHP       = regexp.MustCompile(`requires PHP version ([\d.]+)`)
	phpVersionMessage = regexp.MustCompile(`The PHP version on your server is [\d.]+, however the new plugin version requires ([\d.]+)`)
	// The wp-cli warning that names the plugin it is about.
	requirementsOf = regexp.MustCompile(`(?m)Warning: ([a-z0-9_.-]+): This update requires`)
	downloadFailed = regexp.MustCompile(`Download failed\.\s*([^;\n]*)`)
	// A PHP fatal error and the wp-content directory it happened in.
	fatalError   = regexp.MustCompile(`(?:PHP )?Fatal error: `)
	fatalIn      = regexp.MustCompile(`Fatal error: .*?/wp-content/(plugins|themes|mu-plugins)/([^/\s:]+)`)
	criticalSite = regexp.MustCompile(`There has been a critical error on this website`)
	sshFailure   = regexp.MustCompile(`ssh: connect to host|ssh: Could not resolve hostname|Connection refused|Connection timed out|Permission denied \(publickey`)
)

// FailureCause reads what went wrong from a failed update's output (the
// result's error and the calls' stderr): one of the Cause* codes and a
// detail, or "" if it isn't recognized. plugin is the plugin the result is
// for, or "" for core: one call updates several plugins and every failed
// one gets its messages, so a requirement warning that names another
// plugin isn't this plugin's cause.
func FailureCause(output, plugin string) (cause, detail string) {
	switch {
	case strings.Contains(output, "timed out after"):
		return CauseTimeout, ""
	case sshFailure.MatchString(output):
		return CauseConnection, ""
	case fatalError.MatchString(output) || criticalSite.MatchString(output):
		if m := fatalIn.FindStringSubmatch(output); m != nil {
			return CauseSiteError, m[1] + "/" + m[2]
		}
		return CauseSiteError, ""
	}
	if need := requirement(output, plugin); need != "" {
		return CauseRequirements, need
	}
	switch {
	case strings.Contains(output, "Update package not available.") ||
		strings.Contains(output, "Installation package not available.") ||
		strings.Contains(output, "Package not available.") ||
		// Finnish and Swedish, from runs without EnglishMessagesArg.
		strings.Contains(output, "Päivityspaketti ei ole käytettävissä.") ||
		strings.Contains(output, "Uppdateringspaketet är inte tillgängligt."):
		return CauseNoPackage, ""
	case strings.Contains(output, "Download failed."):
		m := downloadFailed.FindStringSubmatch(output)
		if m == nil {
			return CauseDownloadFailed, ""
		}
		return CauseDownloadFailed, strings.TrimSpace(m[1])
	case strings.Contains(output, "The package could not be installed.") ||
		strings.Contains(output, "contains no files.") ||
		strings.Contains(output, "No valid plugins were found.") ||
		strings.Contains(output, "Incompatible Archive") ||
		strings.Contains(output, "PCLZIP_ERR"):
		return CauseBadPackage, ""
	case strings.Contains(output, "Another update is currently in progress."):
		return CauseUpdateLocked, ""
	case strings.Contains(output, "Could not copy file") ||
		strings.Contains(output, "some files could not be copied") ||
		strings.Contains(output, "Could not create directory") ||
		strings.Contains(output, "Could not remove the") ||
		strings.Contains(output, "Could not access filesystem") ||
		strings.Contains(output, "Destination folder already exists") ||
		strings.Contains(output, "No space left on device"):
		return CauseFiles, ""
	}
	return "", ""
}

// requirement returns what an update needs ("WordPress 7.0", "PHP 8.0",
// or just "WordPress" when the version is blank) if the output says the
// plugin's update needs a newer version, else "".
func requirement(output, plugin string) string {
	// wp-cli names the plugin; with several failed plugins in one call,
	// only the named ones are blocked by requirements.
	if requirementsOf.MatchString(output) {
		if plugin == "" {
			return ""
		}
		own := regexp.MustCompile(regexp.QuoteMeta(plugin) + `: This update requires (WordPress|PHP) version ([\d.]*)`)
		if m := own.FindStringSubmatch(output); m != nil {
			return strings.TrimSpace(m[1] + " " + m[2])
		}
		return ""
	}
	if m := requiresWP.FindStringSubmatch(output); m != nil {
		return "WordPress " + m[1] + m[2]
	}
	if m := requiresPHP.FindStringSubmatch(output); m != nil {
		return "PHP " + m[1]
	}
	if m := phpVersionMessage.FindStringSubmatch(output); m != nil {
		return "PHP " + m[1]
	}
	return ""
}

// CauseLabel is a short description of a failure cause for people, e.g.
// "Needs WordPress 7.0" or "Site fails to load (fatal error in
// plugins/polylang-wc)"; "" for no cause. The web UI has the same labels
// (failureCauseLabel in web/src/utils/format.ts).
func CauseLabel(cause, detail string) string {
	with := func(label string) string {
		if detail == "" {
			return label
		}
		return label + " (" + detail + ")"
	}
	switch cause {
	case CauseRequirements:
		if detail == "" {
			return "Needs a newer WordPress or PHP"
		}
		return "Needs " + detail
	case CauseNoPackage:
		return "No update package: is the license active on this site?"
	case CauseDownloadFailed:
		return with("Download failed")
	case CauseSiteError:
		if detail == "" {
			return "Site fails to load (PHP fatal error)"
		}
		return "Site fails to load (fatal error in " + detail + ")"
	case CauseFiles:
		return "Couldn't write the files (permissions or disk space)"
	case CauseBadPackage:
		return "The package isn't an installable plugin"
	case CauseUpdateLocked:
		return "Another update was running on the site"
	case CauseTimeout:
		return "wp-cli timed out"
	case CauseConnection:
		return "Couldn't connect to the server"
	}
	return with(cause)
}
