package wpcli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
)

// adminUserTTL is how long a site's cached administrator user ID is trusted
// before it's re-fetched. Some plugins (e.g. FileBird Pro) only register their
// update checker when `current_user_can('manage_options')` is true, which
// fails under WP-CLI's default user 0 — running plugin list/update as a real
// administrator fixes that. Caching this for a day avoids an extra `wp user
// list` round trip on every plugin check.
const adminUserTTL = 24 * time.Hour

// GetAdminUserID returns the lowest-numbered administrator user ID on the
// target site, for deterministic reuse across calls.
func GetAdminUserID(site models.CliSite) (int, error) {
	res, err := RunWP(CliOptions{SiteID: site.ID, SSH: site.SSH, Path: site.Path}, "user", "list", "--role=administrator", "--fields=ID", "--format=json")
	if err != nil {
		return 0, fmt.Errorf("failed to list administrators: %w (stderr: %s)", err, res.Error)
	}

	return parseLowestAdminUserID(res.Output)
}

// parseLowestAdminUserID extracts the lowest numeric user ID from the JSON
// array produced by `wp user list --fields=ID --format=json`. Split out from
// GetAdminUserID so the parsing/selection logic can be unit tested without
// shelling out to wp-cli.
func parseLowestAdminUserID(output string) (int, error) {
	output = strings.TrimSpace(output)
	idx := strings.Index(output, "[")
	if idx == -1 {
		return 0, fmt.Errorf("no valid JSON array found in output")
	}

	var raw []struct {
		ID string `json:"ID"`
	}
	if err := json.Unmarshal([]byte(output[idx:]), &raw); err != nil {
		return 0, fmt.Errorf("failed to parse user list JSON: %w", err)
	}
	if len(raw) == 0 {
		return 0, fmt.Errorf("no administrator users found on site")
	}

	lowest := 0
	for _, u := range raw {
		id, err := strconv.Atoi(strings.TrimSpace(u.ID))
		if err != nil {
			continue
		}
		if lowest == 0 || id < lowest {
			lowest = id
		}
	}
	if lowest == 0 {
		return 0, fmt.Errorf("no valid administrator user IDs found on site")
	}

	return lowest, nil
}

// resolveAdminUser returns a cached administrator user ID for the site,
// refreshing it via wp-cli if missing or older than adminUserTTL. It never
// fails the caller: any error fetching or caching the admin user is logged
// and 0 is returned, meaning callers should omit --user (today's behavior).
func resolveAdminUser(site models.CliSite) int {
	userID, updatedAt, found, err := db.GetSiteAdminUser(site.ID)
	if err != nil {
		verb.Printf(verb.Verbose, "Failed to read cached admin user for site %s: %v\n", site.Name, err)
	} else if found {
		if lu, err := parseCacheTimestamp(updatedAt); err == nil && time.Now().UTC().Sub(lu) < adminUserTTL {
			return userID
		}
	}

	fresh, err := GetAdminUserID(site)
	if err != nil {
		verb.Printf(verb.Verbose, "Failed to fetch admin user for site %s: %v\n", site.Name, err)
		if found {
			return userID
		}
		return 0
	}

	if err := db.SaveSiteAdminUser(site.ID, fresh); err != nil {
		verb.Printf(verb.Verbose, "Failed to cache admin user for site %s: %v\n", site.Name, err)
	}

	return fresh
}

// parseCacheTimestamp parses a DATETIME value read back from SQLite. The
// modernc.org/sqlite driver returns DATETIME columns as RFC3339 strings (e.g.
// "2026-08-05T11:52:49Z"), not the "YYYY-MM-DD HH:MM:SS" layout the `sqlite3`
// CLI displays for the same underlying TEXT value.
func parseCacheTimestamp(value string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}
	return time.ParseInLocation("2006-01-02 15:04:05", value, time.UTC)
}
