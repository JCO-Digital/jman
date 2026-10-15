package wpcli

import (
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

func TestParseLowestAdminUserID(t *testing.T) {
	cases := []struct {
		name    string
		output  string
		want    int
		wantErr bool
	}{
		{
			name:   "single admin",
			output: `[{"ID":"5"}]`,
			want:   5,
		},
		{
			name:   "picks lowest ID regardless of order",
			output: `[{"ID":"9"},{"ID":"2"},{"ID":"14"}]`,
			want:   2,
		},
		{
			name:   "leading wp-cli notice before the JSON array",
			output: "Some warning notice\n" + `[{"ID":"3"}]`,
			want:   3,
		},
		{
			name:    "empty array",
			output:  `[]`,
			wantErr: true,
		},
		{
			name:    "no JSON array",
			output:  ``,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseLowestAdminUserID(tc.output)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got id %d", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected %d, got %d", tc.want, got)
			}
		})
	}
}

type fakeAdminUsers struct {
	id        int
	updatedAt string
	saved     int
}

func (f *fakeAdminUsers) GetSiteAdminUser(string) (int, string, bool, error) {
	return f.id, f.updatedAt, f.id != 0, nil
}

func (f *fakeAdminUsers) SaveSiteAdminUser(_ string, id int) error {
	f.saved = id
	return nil
}

func TestResolveAdminUserUsesCache(t *testing.T) {
	old := AdminUsers
	t.Cleanup(func() { AdminUsers = old })

	cache := &fakeAdminUsers{id: 7, updatedAt: time.Now().UTC().Format(time.RFC3339)}
	AdminUsers = cache
	// A fresh cached ID is used without running WP-CLI (the site has no
	// SSH target, so a lookup would fail and return 0).
	if got := resolveAdminUser(models.CliSite{ID: "s", Name: "s", SSH: "invalid host"}); got != 7 {
		t.Errorf("resolveAdminUser = %d, want the cached 7", got)
	}
}
