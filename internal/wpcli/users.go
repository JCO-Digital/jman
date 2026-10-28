package wpcli

import (
	"encoding/base64"
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

// UserAppPasswordsEval is PHP for `wp eval` that prints every user's
// application passwords, as a JSON object of user ID to a list of them,
// in one query. It leaves out the password hashes.
const UserAppPasswordsEval = `global $wpdb;
$rows = $wpdb->get_results($wpdb->prepare("SELECT user_id, meta_value FROM {$wpdb->usermeta} WHERE meta_key = %s", '_application_passwords'), ARRAY_N);
if ($wpdb->last_error) { WP_CLI::error($wpdb->last_error); }
$out = array();
foreach ((array) $rows as $r) {
	$list = maybe_unserialize($r[1]);
	if (!is_array($list)) { continue; }
	foreach ($list as $p) {
		if (!is_array($p)) { continue; }
		$out[(string) $r[0]][] = array(
			'uuid' => isset($p['uuid']) ? (string) $p['uuid'] : '',
			'name' => isset($p['name']) ? (string) $p['name'] : '',
			'created' => isset($p['created']) ? (int) $p['created'] : 0,
			'last_used' => isset($p['last_used']) ? (int) $p['last_used'] : null,
			'last_ip' => isset($p['last_ip']) ? (string) $p['last_ip'] : '',
		);
	}
}
echo json_encode((object) $out);`

// maxAppPasswordName caps an application password's stored name.
const maxAppPasswordName = 200

// SetAppPasswords parses UserAppPasswordsEval output, skipping any PHP
// notices printed before the JSON, and sets the listed users'
// application passwords. Users of excluded roles aren't listed, so theirs
// are left out.
func SetAppPasswords(su *models.SiteUsers, output string) error {
	output = strings.TrimSpace(output)
	idx := strings.Index(output, "{")
	if idx == -1 {
		return fmt.Errorf("no JSON object found in application password output")
	}
	var raw map[string][]models.SiteAppPassword
	if err := json.Unmarshal([]byte(output[idx:]), &raw); err != nil {
		return fmt.Errorf("failed to parse application passwords: %w", err)
	}
	for i := range su.Users {
		list := raw[strconv.Itoa(su.Users[i].ID)]
		if len(list) > models.MaxAppPasswordsPerUser {
			list = list[:models.MaxAppPasswordsPerUser]
		}
		for j := range list {
			if len(list[j].Name) > maxAppPasswordName {
				list[j].Name = strings.ToValidUTF8(list[j].Name[:maxAppPasswordName], "")
			}
		}
		su.Users[i].AppPasswords = list
	}
	su.AppPasswordsCollected = true
	return nil
}

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
	// Application passwords only inform the security checks, so a failure
	// leaves them uncollected.
	res, err = RunWP(opts, "eval", UserAppPasswordsEval)
	if err == nil {
		err = SetAppPasswords(&su, res.Output)
	}
	if err != nil {
		verb.LogPrintf(verb.Normal, "Failed to read application passwords on %s: %v", site.Name, err)
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

// CreateUser creates a user on the site and returns its ID. The password
// is passed as an argument, so it's never part of the returned error, which
// is wp-cli's first error line (e.g. that the login already exists).
func CreateUser(site models.CliSite, login, email, role, password string) (int, error) {
	opts := CliOptions{SiteID: site.ID, SSH: site.SSH, Path: site.Path, Timeout: WriteTimeout}
	res, err := RunWP(opts, "user", "create", login, email, "--role="+role, "--user_pass="+password, "--porcelain")
	if err != nil {
		return 0, err
	}
	id, err := ParseUserCount(res.Output)
	if err != nil {
		return 0, fmt.Errorf("user created, but its ID wasn't reported: %w", err)
	}
	return id, nil
}

// UserDeleteCheck is what a site says about a user about to be deleted.
type UserDeleteCheck struct {
	// Exists is false if there's no user with the ID; Login and Email are
	// then empty.
	Exists bool   `json:"exists"`
	Login  string `json:"login"`
	Email  string `json:"email"`
	// Posts is how many posts of any type and status the user authored.
	Posts int `json:"posts"`
	// ReassignIDs are the users whose login or email is the reassign
	// target; empty without one.
	ReassignIDs []int `json:"reassign_ids"`
}

// userDeleteCheckEval is PHP for `wp eval` that prints a UserDeleteCheck.
// %s is the base64 of a JSON object with user_id and reassign, so no input
// needs escaping. Posts are counted as in UserPostCountsEval.
const userDeleteCheckEval = `$in = json_decode(base64_decode('%s'), true);
global $wpdb;
$out = array('exists' => false, 'login' => '', 'email' => '', 'posts' => 0, 'reassign_ids' => array());
$u = get_userdata((int) $in['user_id']);
if ($u) {
	$out['exists'] = true;
	$out['login'] = $u->user_login;
	$out['email'] = $u->user_email;
}
$out['posts'] = (int) $wpdb->get_var($wpdb->prepare("SELECT COUNT(*) FROM {$wpdb->posts} WHERE post_author = %%d", (int) $in['user_id']));
if ($in['reassign'] !== '') {
	$ids = $wpdb->get_col($wpdb->prepare("SELECT ID FROM {$wpdb->users} WHERE user_login = %%s OR user_email = %%s", $in['reassign'], $in['reassign']));
	$out['reassign_ids'] = array_map('intval', $ids);
}
if ($wpdb->last_error) { WP_CLI::error($wpdb->last_error); }
echo json_encode($out);`

// UserDeleteCheckCode returns the `wp eval` code checking a user before
// deleting it, with reassign the login or email of the user to get its
// content ("" for none).
func UserDeleteCheckCode(userID int, reassign string) string {
	in, _ := json.Marshal(map[string]any{"user_id": userID, "reassign": reassign})
	return fmt.Sprintf(userDeleteCheckEval, base64.StdEncoding.EncodeToString(in))
}

// CheckUserDelete reports on a user about to be deleted: whether it still
// exists and as whom, its post count and who the reassign target matches.
func CheckUserDelete(site models.CliSite, userID int, reassign string) (UserDeleteCheck, error) {
	opts := CliOptions{SiteID: site.ID, SSH: site.SSH, Path: site.Path}
	res, err := RunWP(opts, "eval", UserDeleteCheckCode(userID, reassign))
	if err != nil {
		return UserDeleteCheck{}, fmt.Errorf("failed to check the user: %w", err)
	}
	return ParseUserDeleteCheck(res.Output)
}

// ParseUserDeleteCheck parses the output of UserDeleteCheckCode, skipping
// any PHP notices printed before the JSON.
func ParseUserDeleteCheck(output string) (UserDeleteCheck, error) {
	output = strings.TrimSpace(output)
	idx := strings.Index(output, "{")
	if idx == -1 {
		return UserDeleteCheck{}, fmt.Errorf("no JSON object found in user check output")
	}
	var check UserDeleteCheck
	if err := json.Unmarshal([]byte(output[idx:]), &check); err != nil {
		return UserDeleteCheck{}, fmt.Errorf("failed to parse user check: %w", err)
	}
	return check, nil
}

// DeleteUser deletes a user from the site, giving its content to the user
// with ID reassignTo, or deleting the content if reassignTo is 0.
func DeleteUser(site models.CliSite, userID, reassignTo int) error {
	opts := CliOptions{SiteID: site.ID, SSH: site.SSH, Path: site.Path, Timeout: WriteTimeout}
	args := []string{"user", "delete", strconv.Itoa(userID), "--yes"}
	if reassignTo != 0 {
		args = append(args, "--reassign="+strconv.Itoa(reassignTo))
	}
	_, err := RunWP(opts, args...)
	return err
}
