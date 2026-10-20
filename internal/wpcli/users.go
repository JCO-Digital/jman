package wpcli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
)

// ExcludedUserRoles are the roles left out of a site's user list and only
// counted: sites can have thousands of them, and they're not who anyone
// searches for.
var ExcludedUserRoles = []string{"subscriber", "customer"}

// UserListArgs returns the wp-cli arguments listing a site's users without
// the excluded roles, one more than models.MaxSiteUsers so that a longer
// list can be told apart.
func UserListArgs() []string {
	return []string{
		"user", "list",
		"--role__not_in=" + strings.Join(ExcludedUserRoles, ","),
		"--fields=ID,user_login,user_email,display_name,roles,user_registered",
		"--orderby=ID", "--order=ASC",
		fmt.Sprintf("--number=%d", models.MaxSiteUsers+1),
		"--format=json",
	}
}

// UserCountArgs returns the wp-cli arguments counting a site's users with
// the given role.
func UserCountArgs(role string) []string {
	return []string{"user", "list", "--role=" + role, "--format=count"}
}

// UserPostCountsEval is PHP for `wp eval` that prints how many posts each
// author has, as a JSON object of user ID to count. It counts every post
// type and status straight from the posts table, as deleting a user
// deletes all of them; post_author is indexed, so this stays cheap on large
// sites.
const UserPostCountsEval = `global $wpdb;
$rows = $wpdb->get_results("SELECT post_author, COUNT(*) FROM {$wpdb->posts} WHERE post_author > 0 GROUP BY post_author", ARRAY_N);
if ($wpdb->last_error) { WP_CLI::error($wpdb->last_error); }
$out = array();
foreach ((array) $rows as $r) { $out[(string) $r[0]] = (int) $r[1]; }
echo json_encode((object) $out);`

// GetSiteUsers returns the target site's users, without the excluded
// roles, which are counted. If the posts can't be counted, the users'
// post counts are left unknown.
func GetSiteUsers(site models.CliSite) (models.SiteUsers, error) {
	opts := CliOptions{SiteID: site.ID, SSH: site.SSH, Path: site.Path}
	res, err := RunWP(opts, UserListArgs()...)
	if err != nil {
		return models.SiteUsers{}, fmt.Errorf("failed to list users: %w (stderr: %s)", err, res.Error)
	}
	users, err := ParseUserList(res.Output, site.ID)
	if err != nil {
		return models.SiteUsers{}, err
	}
	counts := make(map[string]int, len(ExcludedUserRoles))
	for _, role := range ExcludedUserRoles {
		res, err := RunWP(opts, UserCountArgs(role)...)
		if err != nil {
			return models.SiteUsers{}, fmt.Errorf("failed to count %s users: %w (stderr: %s)", role, err, res.Error)
		}
		n, err := ParseUserCount(res.Output)
		if err != nil {
			return models.SiteUsers{}, err
		}
		counts[role] = n
	}
	su := NewSiteUsers(users, counts)
	res, err = RunWP(opts, "eval", UserPostCountsEval)
	if err == nil {
		var posts map[int]int
		if posts, err = ParseUserPostCounts(res.Output); err == nil {
			SetPostCounts(su.Users, posts)
		}
	}
	if err != nil {
		verb.LogPrintf(verb.Normal, "Failed to count posts per user on %s: %v", site.Name, err)
	}
	return su, nil
}

// ParseUserPostCounts parses UserPostCountsEval output, skipping any PHP
// notices printed before the JSON.
func ParseUserPostCounts(output string) (map[int]int, error) {
	output = strings.TrimSpace(output)
	idx := strings.Index(output, "{")
	if idx == -1 {
		return nil, fmt.Errorf("no JSON object found in post count output")
	}
	var raw map[string]int
	if err := json.Unmarshal([]byte(output[idx:]), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse post counts: %w", err)
	}
	counts := make(map[int]int, len(raw))
	for k, n := range raw {
		id, err := strconv.Atoi(k)
		if err != nil {
			return nil, fmt.Errorf("invalid user ID %q in post counts", k)
		}
		counts[id] = n
	}
	return counts, nil
}

// SetPostCounts sets each user's post count from counts by user ID; users
// without posts aren't in counts and get 0.
func SetPostCounts(users []models.SiteUser, counts map[int]int) {
	for i := range users {
		n := counts[users[i].ID]
		users[i].PostCount = &n
	}
}

// NewSiteUsers builds a site's users from a parsed list (as fetched with
// UserListArgs) and the excluded roles' counts, capping the list at
// models.MaxSiteUsers.
func NewSiteUsers(users []models.SiteUser, counts map[string]int) models.SiteUsers {
	su := models.SiteUsers{Users: users, ExcludedCounts: counts}
	if su.Users == nil {
		su.Users = []models.SiteUser{}
	}
	if len(su.Users) > models.MaxSiteUsers {
		su.Users = su.Users[:models.MaxSiteUsers]
		su.Truncated = true
	}
	return su
}

// ParseUserList parses `wp user list --format=json` output (with the
// fields of UserListArgs) into users of the given site, skipping any PHP
// notices printed before the JSON.
func ParseUserList(output, siteID string) ([]models.SiteUser, error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil, nil
	}
	idx := strings.Index(output, "[")
	if idx == -1 {
		return nil, fmt.Errorf("no valid JSON array found in output")
	}

	var raw []struct {
		ID          json.Number `json:"ID"`
		Login       string      `json:"user_login"`
		Email       string      `json:"user_email"`
		DisplayName string      `json:"display_name"`
		Roles       string      `json:"roles"`
		Registered  string      `json:"user_registered"`
	}
	if err := json.Unmarshal([]byte(output[idx:]), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse user list JSON: %w", err)
	}

	users := make([]models.SiteUser, 0, len(raw))
	for _, u := range raw {
		id, err := strconv.Atoi(strings.TrimSpace(u.ID.String()))
		if err != nil {
			continue
		}
		users = append(users, models.SiteUser{
			SiteID:      siteID,
			ID:          id,
			Login:       u.Login,
			Email:       u.Email,
			DisplayName: u.DisplayName,
			Roles:       u.Roles,
			Registered:  u.Registered,
		})
	}
	return users, nil
}

// ParseUserCount parses `wp user list --format=count` output, which is the
// last line, after any PHP notices.
func ParseUserCount(output string) (int, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	last := strings.TrimSpace(lines[len(lines)-1])
	n, err := strconv.Atoi(last)
	if err != nil {
		return 0, fmt.Errorf("invalid user count %q", last)
	}
	return n, nil
}
