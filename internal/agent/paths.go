package agent

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
)

// domainRe matches a hostname: dot-separated labels of letters, digits,
// hyphens and underscores. Site paths are built from the domain jman-api
// sends, so anything else (a "..", a slash) is refused rather than joined
// into a path the root-running agent then reads or runs wp-cli in.
var domainRe = regexp.MustCompile(`^[A-Za-z0-9_]([A-Za-z0-9_-]*[A-Za-z0-9_])?(\.[A-Za-z0-9_]([A-Za-z0-9_-]*[A-Za-z0-9_])?)+$`)

// siteContentDir is the subdirectory under a site's home that holds its
// actual WordPress install (wp-config.php, wp-content, etc.). This is a
// fixed jman convention, not derived from SpinupWP's "public_folder" API
// field — public_folder describes the web-server-exposed docroot, which can
// be nested inside siteContentDir for security (e.g. Bedrock-style layouts
// serving from a "web" subfolder), and is a different thing from where
// wp-config.php lives. The same "files" convention is already relied on
// elsewhere in jman for real SSH/wp-cli execution (see cache.GetSiteList).
const siteContentDir = "files"

// ResolveSitePath returns the absolute filesystem path to a site's content
// directory. SpinupWP's standard layout places every site on a server under
// /sites/<domain>/files (a single shared account, not a dedicated Unix user
// per site — site_user in the API is often an SFTP-only account rather than
// a real local user). Some servers may instead give each site its own Unix
// user whose home directory holds the site, so that's tried as a fallback
// if the standard path doesn't exist locally.
func ResolveSitePath(domain, siteUser string) (string, error) {
	if !domainRe.MatchString(domain) {
		return "", fmt.Errorf("invalid site domain %q", domain)
	}
	standardPath := filepath.Join("/sites", domain, siteContentDir)
	if info, err := os.Stat(standardPath); err == nil && info.IsDir() {
		return standardPath, nil
	}

	if siteUser != "" {
		if u, err := user.Lookup(siteUser); err == nil {
			homePath := filepath.Join(u.HomeDir, siteContentDir)
			if info, err := os.Stat(homePath); err == nil && info.IsDir() {
				return homePath, nil
			}
		}
	}

	return "", fmt.Errorf("no valid site path found (tried %s, and a local user lookup for %q)", standardPath, siteUser)
}
