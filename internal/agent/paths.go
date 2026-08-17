package agent

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
)

// ResolveSitePath returns the absolute filesystem path to a site's public
// folder. SpinupWP's standard layout places every site on a server under
// /sites/<domain>/<public_folder> (a single shared account, not a dedicated
// Unix user per site — site_user in the API is often an SFTP-only account
// rather than a real local user). Some servers may instead give each site
// its own Unix user whose home directory holds the site, so that's tried as
// a fallback if the standard path doesn't exist locally.
func ResolveSitePath(domain, siteUser, publicFolder string) (string, error) {
	standardPath := filepath.Join("/sites", domain, publicFolder)
	if info, err := os.Stat(standardPath); err == nil && info.IsDir() {
		return standardPath, nil
	}

	if siteUser != "" {
		if u, err := user.Lookup(siteUser); err == nil {
			homePath := filepath.Join(u.HomeDir, publicFolder)
			if info, err := os.Stat(homePath); err == nil && info.IsDir() {
				return homePath, nil
			}
		}
	}

	return "", fmt.Errorf("no valid site path found (tried %s, and a local user lookup for %q)", standardPath, siteUser)
}
