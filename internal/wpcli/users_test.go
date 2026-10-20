package wpcli

import (
	"os/exec"
	"slices"
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
	if !slices.Equal(users, want) {
		t.Errorf("users = %+v", users)
	}

	if users, err := ParseUserList("[]", ""); err != nil || len(users) != 0 {
		t.Errorf("empty list = %+v, %v", users, err)
	}
	if _, err := ParseUserList("Error: no database", ""); err == nil {
		t.Error("no error for output without JSON")
	}
}

func TestParseUserPostCounts(t *testing.T) {
	counts, err := ParseUserPostCounts("PHP Notice: x\n{\"1\":12,\"7\":1}")
	if err != nil || len(counts) != 2 || counts[1] != 12 || counts[7] != 1 {
		t.Errorf("counts = %v, %v", counts, err)
	}
	if counts, err := ParseUserPostCounts("{}"); err != nil || len(counts) != 0 {
		t.Errorf("no posts = %v, %v", counts, err)
	}
	if _, err := ParseUserPostCounts("Error: no database"); err == nil {
		t.Error("no error for output without JSON")
	}

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
		"post counts":  UserPostCountsEval,
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
