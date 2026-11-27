package wpcli

import (
	"os/exec"
	"strings"
	"testing"
)

// The outputs are from update-attempts.jsonl (docs/update-log.md).
func TestFailureCause(t *testing.T) {
	cases := []struct {
		name, output, plugin string
		// Other plugins failed in the same call.
		shared        bool
		cause, detail string
	}{
		{
			name:   "cURL timeout is the download's",
			output: "Warning: Download failed. cURL error 28: Operation timed out after 300001 milliseconds with 0 bytes received; Error: No plugins updated (1 failed).",
			plugin: "x",
			cause:  CauseDownloadFailed, detail: "cURL error 28: Operation timed out after 300001 milliseconds with 0 bytes received",
		},
		{
			name:   "cURL connection refused is the download's",
			output: "Warning: Download failed. cURL error 7: Failed to connect to updates.example.com port 443: Connection refused",
			plugin: "x",
			cause:  CauseDownloadFailed, detail: "cURL error 7: Failed to connect to updates.example.com port 443: Connection refused",
		},
		{
			name:   "two kinds of failure in one call",
			output: "Warning: Update package not available.; Warning: Download failed. Unauthorized; Error: No plugins updated (2 failed).",
			plugin: "gravityforms", shared: true,
		},
		{
			name:   "one kind of failure in a shared call",
			output: "Warning: wordpress-seo: This update requires WordPress version 7.0, but the version installed is 6.9.12.; Warning: Update package not available.; Error: No plugins updated (2 failed).",
			plugin: "polylang-pro", shared: true,
			cause: CauseNoPackage,
		},
		{
			name:   "slug that ends another plugin's",
			output: "Warning: wordpress-seo: This update requires WordPress version 7.0, but the version installed is 6.9.12.; Error: No plugins updated (2 failed).",
			plugin: "seo", shared: true,
		},
		{
			name:   "ZIP needing a newer PHP",
			output: "Warning: The PHP version on your server is 7.4.33, however the uploaded plugin requires 8.1.",
			plugin: "x",
			cause:  CauseRequirements, detail: "PHP 8.1",
		},
		{
			name:   "needs WordPress",
			output: "failed to update plugin: Warning: wordpress-seo: This update requires WordPress version 7.0, but the version installed is 6.9.12.; Error: No plugins updated.",
			plugin: "wordpress-seo",
			cause:  CauseRequirements, detail: "WordPress 7.0",
		},
		{
			name:   "needs PHP",
			output: "failed to update plugin: Warning: duracelltomi-google-tag-manager: This update requires PHP version 8.0, but the version installed is 7.4.33.18.; Error: No plugins updated.",
			plugin: "duracelltomi-google-tag-manager",
			cause:  CauseRequirements, detail: "PHP 8.0",
		},
		{
			name:   "another plugin's requirement",
			output: "Warning: wordpress-seo: This update requires WordPress version 7.0, but the version installed is 6.9.12.; Warning: Update package not available.; Error: No plugins updated (2 failed).",
			plugin: "polylang-pro",
			cause:  CauseNoPackage,
		},
		{
			name:   "blank requirement (WPML)",
			output: "failed to update plugin: Warning: sitepress-multilingual-cms: This update requires WordPress version , but the version installed is 7.1.5.; Error: No plugins updated.",
			plugin: "sitepress-multilingual-cms",
			cause:  CauseRequirements, detail: "WordPress",
		},
		{
			name:   "WordPress's own requirement message",
			output: "Warning: Your WordPress version is 6.8, however the new plugin version requires 6.9.",
			plugin: "x",
			cause:  CauseRequirements, detail: "WordPress 6.9",
		},
		{
			name:   "no license",
			output: "Warning: Update package not available.; Error: No plugins updated (1 failed).",
			plugin: "polylang-pro",
			cause:  CauseNoPackage,
		},
		{
			name:   "no license, in Finnish",
			output: "Warning: Päivityspaketti ei ole käytettävissä.; Error: No plugins updated (1 failed).",
			plugin: "polylang-pro",
			cause:  CauseNoPackage,
		},
		{
			name:   "download refused",
			output: "Warning: Download failed. Unauthorized; Error: No plugins updated (1 failed).",
			plugin: "gravityforms",
			cause:  CauseDownloadFailed, detail: "Unauthorized",
		},
		{
			name: "fatal error in another plugin",
			output: "Fatal error: Uncaught Error: Class \"PLL_REST_Translated_Object\" not found in /sites/x/files/wp-content/plugins/polylang-wc/include/rest-product.php:11\n" +
				"PHP Fatal error:  Uncaught Error: Class \"PLL_REST_Translated_Object\" not found in /sites/x/files/wp-content/plugins/polylang-wc/include/rest-product.php:11",
			plugin: "stream",
			cause:  CauseSiteError, detail: "plugins/polylang-wc",
		},
		{
			name:   "critical error page only",
			output: "failed to update plugin: Error: There has been a critical error on this website.",
			plugin: "wordpress-seo",
			cause:  CauseSiteError,
		},
		{
			name:   "file permissions",
			output: "Warning: The update cannot be installed because some files could not be copied. This is usually due to inconsistent file permissions.",
			plugin: "x",
			cause:  CauseFiles,
		},
		{
			name:   "broken ZIP",
			output: "Warning: The package could not be installed. PCLZIP_ERR_BAD_FORMAT (-10) : Unable to find End of Central Dir Record signature",
			plugin: "x",
			cause:  CauseBadPackage,
		},
		{
			name:   "timeout",
			output: "failed to update plugin: wp-cli timed out after 16m0s",
			plugin: "x",
			cause:  CauseTimeout,
		},
		{
			name:   "ssh",
			output: "ssh: connect to host example.com port 22: Connection refused",
			plugin: "x",
			cause:  CauseConnection,
		},
		{
			name:   "unknown",
			output: "Error: Something else entirely.",
			plugin: "x",
		},
		{
			// PHP warnings from the site's code are noise, not a fatal.
			name:   "PHP warning only",
			output: "PHP Warning:  foreach() argument must be of type array|object, null given in /x/wp-content/plugins/ld-organization/ld-organization.php on line 159\nWarning: Update package not available.",
			plugin: "polylang-pro",
			cause:  CauseNoPackage,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cause, detail := FailureCause(c.output, c.plugin, c.shared)
			if cause != c.cause || detail != c.detail {
				t.Errorf("FailureCause = %q, %q; want %q, %q", cause, detail, c.cause, c.detail)
			}
		})
	}
}

// The snippet runs on every update, so a syntax error would break them
// all; lint it where PHP is installed, and check it keeps only the listed
// strings in WordPress's text domain untranslated.
func TestEnglishMessagesPHP(t *testing.T) {
	php, err := exec.LookPath("php")
	if err != nil {
		t.Skip("php not installed")
	}
	cmd := exec.Command(php, "-l")
	cmd.Stdin = strings.NewReader("<?php " + englishMessagesPHP)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("php -l: %v\n%s", err, out)
	}

	// Run it against a stand-in WP_CLI that applies the filter.
	script := `<?php
class WP_CLI { public static $f; public static function add_wp_hook($tag, $fn, $prio, $args) { self::$f = $fn; } }
` + englishMessagesPHP + `
$f = WP_CLI::$f;
echo $f('Päivityspaketti ei ole käytettävissä.', 'Update package not available.', 'default'), "\n";
echo $f('Lataus epäonnistui.', 'Download failed.', 'default'), "\n";
echo $f('Päivitetään lisäosaa…', 'Updating the plugin&#8230;', 'default'), "\n";
echo $f('Ei saatavilla', 'Update package not available.', 'some-plugin'), "\n";
`
	cmd = exec.Command(php)
	cmd.Stdin = strings.NewReader(script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("php: %v\n%s", err, out)
	}
	want := "Update package not available.\nDownload failed.\nPäivitetään lisäosaa…\nEi saatavilla\n"
	if string(out) != want {
		t.Errorf("filter output:\n%s\nwant:\n%s", out, want)
	}
}
