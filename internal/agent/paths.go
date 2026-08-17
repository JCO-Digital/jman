package agent

import (
	"fmt"
	"os/user"
	"path/filepath"
)

// ResolveSitePath returns the absolute filesystem path to a site's public
// folder, by looking up siteUser's actual home directory via the OS's own
// user database rather than assuming any particular provisioning
// convention (SpinupWP sites may live at /sites/<domain>/files,
// <home_folder>/files, or elsewhere depending on how the server was
// provisioned — asking the OS directly is correct regardless of which).
func ResolveSitePath(siteUser, publicFolder string) (string, error) {
	u, err := user.Lookup(siteUser)
	if err != nil {
		return "", fmt.Errorf("failed to look up local user %q: %w", siteUser, err)
	}
	return filepath.Join(u.HomeDir, publicFolder), nil
}
