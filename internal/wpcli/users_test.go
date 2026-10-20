package wpcli

import (
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
