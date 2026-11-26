package wpcli

import (
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/JCO-Digital/jman/internal/models"
)

func TestParseUserList(t *testing.T) {
	out := `PHP Deprecated: something on line 3
[{"ID":"1","user_login":"admin","user_email":"admin@example.com","display_name":"Admin","roles":"administrator","user_registered":"2020-01-02 03:04:05"},{"ID":7,"user_login":"ed","user_email":"ed@example.com","display_name":"Ed","roles":"editor,author","user_registered":"2021-01-01 00:00:00"}]`
	users, err := ParseUserList(out, "site-1")
	if err != nil {
		t.Fatal(err)
	}
	want := []models.SiteUser{
		{SiteID: "site-1", ID: 1, Login: "admin", Email: "admin@example.com", DisplayName: "Admin", Roles: "administrator", Registered: "2020-01-02 03:04:05"},
		{SiteID: "site-1", ID: 7, Login: "ed", Email: "ed@example.com", DisplayName: "Ed", Roles: "editor,author", Registered: "2021-01-01 00:00:00"},
	}
	if !reflect.DeepEqual(users, want) {
		t.Errorf("users = %+v", users)
	}

	if users, err := ParseUserList("[]", ""); err != nil || len(users) != 0 {
		t.Errorf("empty list = %+v, %v", users, err)
	}
	if _, err := ParseUserList("Error: no database", ""); err == nil {
		t.Error("no error for output without JSON")
	}
}

func TestSetPostCounts(t *testing.T) {
	users := []models.SiteUser{{ID: 1}, {ID: 2}}
	SetPostCounts(users, map[int]int{1: 12})
	if users[0].PostCount == nil || *users[0].PostCount != 12 || users[1].PostCount == nil || *users[1].PostCount != 0 {
		t.Errorf("users = %+v", users)
	}
}

func TestParseUserCount(t *testing.T) {
	if n, err := ParseUserCount("Notice: x\n1234\n"); err != nil || n != 1234 {
		t.Errorf("count = %d, %v", n, err)
	}
	if _, err := ParseUserCount("Error: nope"); err == nil {
		t.Error("no error for a non-number")
	}
}

func TestNewSiteUsersCaps(t *testing.T) {
	list := make([]models.SiteUser, models.MaxSiteUsers+1)
	su := NewSiteUsers(list, map[string]int{"subscriber": 3})
	if len(su.Users) != models.MaxSiteUsers || !su.Truncated {
		t.Errorf("got %d users, truncated=%v", len(su.Users), su.Truncated)
	}
	if su := NewSiteUsers(nil, nil); su.Users == nil || su.Truncated {
		t.Errorf("empty = %+v", su)
	}
}

func TestUserListArgsExcludeRoles(t *testing.T) {
	args := strings.Join(UserListArgs(), " ")
	if !strings.Contains(args, "--role__not_in=subscriber,customer") || !strings.Contains(args, "--number=1001") {
		t.Errorf("args = %s", args)
	}
}

// TestUserEvalCodeParses checks the PHP sent to `wp eval` with php -l,
// where PHP is installed.
func TestUserEvalCodeParses(t *testing.T) {
	php, err := exec.LookPath("php")
	if err != nil {
		t.Skip("php not installed")
	}
	for name, code := range map[string]string{
		"user details": UserExtrasEval,
		"delete check": UserDeleteCheckCode(5, "o'brien@example.com"),
	} {
		cmd := exec.Command(php, "-l")
		cmd.Stdin = strings.NewReader("<?php " + code)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s: %v\n%s", name, err, out)
		}
	}
}

func TestParseUserDeleteCheck(t *testing.T) {
	check, err := ParseUserDeleteCheck(`Notice: x
{"exists":true,"login":"mainwp","email":"m@example.com","posts":3,"reassign_ids":[1]}`)
	if err != nil || !check.Exists || check.Login != "mainwp" || check.Posts != 3 || len(check.ReassignIDs) != 1 || check.ReassignIDs[0] != 1 {
		t.Errorf("check = %+v, %v", check, err)
	}
	if !strings.Contains(UserDeleteCheckCode(5, "x"), "base64_decode('eyJyZWFzc2lnbiI6IngiLCJ1c2VyX2lkIjo1fQ==')") {
		t.Errorf("input isn't passed as base64 JSON:\n%s", UserDeleteCheckCode(5, "x"))
	}
}

func TestSiteUsersFromExtras(t *testing.T) {
	list := func() []models.SiteUser { return []models.SiteUser{{ID: 1, Login: "admin"}, {ID: 7, Login: "ed"}} }

	out := "PHP Notice: something\n" + `{"counts":{"subscriber":1234,"customer":0},"posts":{"1":12,"9":4},` +
		`"app_passwords":{"1":[{"uuid":"u1","name":"wordpress_1778988921","created":1711152199,"last_used":null,"last_ip":""}],"9":[{"uuid":"u9","name":"subscriber's","created":1}]},"errors":{}}`
	su, warnings, err := SiteUsersFromExtras(list(), out)
	if err != nil || len(warnings) != 0 {
		t.Fatalf("err = %v, warnings = %v", err, warnings)
	}
	if su.ExcludedCounts["subscriber"] != 1234 || su.ExcludedCounts["customer"] != 0 {
		t.Errorf("counts = %v", su.ExcludedCounts)
	}
	if pc := su.Users[0].PostCount; pc == nil || *pc != 12 {
		t.Errorf("admin posts = %v, want 12", pc)
	}
	if pc := su.Users[1].PostCount; pc == nil || *pc != 0 {
		t.Errorf("ed posts = %v, want 0", pc)
	}
	if !su.AppPasswordsCollected || len(su.Users[0].AppPasswords) != 1 || su.Users[0].AppPasswords[0].UUID != "u1" || su.Users[0].AppPasswords[0].LastUsed != nil || len(su.Users[1].AppPasswords) != 0 {
		t.Errorf("app passwords = %+v", su.Users)
	}

	// Posts and application passwords that couldn't be read are left
	// unknown with a warning.
	su, warnings, err = SiteUsersFromExtras(list(), `{"counts":{"subscriber":1,"customer":2},"errors":{"posts":"Table 'wp_posts' doesn't exist"}}`)
	if err != nil || len(warnings) != 2 || !strings.Contains(warnings[0], "wp_posts") {
		t.Fatalf("err = %v, warnings = %v", err, warnings)
	}
	if su.Users[0].PostCount != nil || su.AppPasswordsCollected {
		t.Errorf("users = %+v", su)
	}

	// Users that couldn't be counted fail the whole list.
	for _, bad := range []string{
		`{"counts":{"subscriber":1},"errors":{"counts":"Lost connection"}}`,
		`{"counts":{"subscriber":1},"errors":{}}`,
		"Error: no database",
	} {
		if _, _, err := SiteUsersFromExtras(list(), bad); err == nil {
			t.Errorf("no error for %s", bad)
		}
	}
}

func TestCollectSiteUsersFallsBackWithoutEval(t *testing.T) {
	list := `[{"ID":"1","user_login":"admin","user_email":"a@example.com","display_name":"Admin","roles":"administrator","user_registered":"2020-01-01 00:00:00"}]`
	extras := `{"counts":{"subscriber":3,"customer":1},"posts":{"1":2},"app_passwords":{},"errors":{}}`

	var calls []string
	run := func(evalFails bool) func(args ...string) (string, error) {
		return func(args ...string) (string, error) {
			cmd := strings.Join(args, " ")
			calls = append(calls, cmd)
			switch {
			case args[0] == "eval":
				if evalFails {
					return "", errors.New("PHP Fatal error: eval() has been disabled for security reasons")
				}
				return extras, nil
			case strings.Contains(cmd, "--role=subscriber"):
				return "5\n", nil
			case strings.Contains(cmd, "--role=customer"):
				return "0\n", nil
			default:
				return list, nil
			}
		}
	}

	su, warnings, err := CollectSiteUsers(run(false), "s1")
	if err != nil || len(warnings) != 0 || len(calls) != 2 || su.ExcludedCounts["subscriber"] != 3 || su.Users[0].SiteID != "s1" {
		t.Fatalf("with eval: su = %+v, warnings = %v, err = %v, calls = %v", su, warnings, err, calls)
	}

	calls = nil
	su, warnings, err = CollectSiteUsers(run(true), "s1")
	if err != nil || len(warnings) != 1 || len(calls) != 4 {
		t.Fatalf("without eval: warnings = %v, err = %v, calls = %v", warnings, err, calls)
	}
	if su.ExcludedCounts["subscriber"] != 5 || su.Users[0].PostCount != nil || su.AppPasswordsCollected {
		t.Errorf("without eval: su = %+v", su)
	}
}
