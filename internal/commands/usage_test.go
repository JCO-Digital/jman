package commands

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestUsageArgs(t *testing.T) {
	tests := []struct {
		use     string
		args    []string
		wantErr string
	}{
		{"info <target> [plugin-name]", nil, "missing <target>"},
		{"info <target> [plugin-name]", []string{"site"}, ""},
		{"info <target> [plugin-name]", []string{"site", "akismet"}, ""},
		{"info <target> [plugin-name]", []string{"site", "akismet", "x"}, `too many arguments: "x"`},
		{"admin <target> <username> <email>", []string{"site"}, "missing <username> and <email>"},
		{"wp <target> <wp-cli-args...>", []string{"site"}, "missing <wp-cli-args...>"},
		{"wp <target> <wp-cli-args...>", []string{"site", "plugin", "list", "--status=active"}, ""},
		{"refresh", []string{"x"}, `too many arguments: "x"`},
	}
	for _, tt := range tests {
		cmd := &cobra.Command{Use: tt.use}
		err := usageArgs(cmd, tt.args)
		switch {
		case tt.wantErr == "" && err != nil:
			t.Errorf("%q %v: unexpected error %v", tt.use, tt.args, err)
		case tt.wantErr != "" && (err == nil || !strings.HasPrefix(err.Error(), tt.wantErr)):
			t.Errorf("%q %v: error %v, want %q", tt.use, tt.args, err, tt.wantErr)
		case err != nil && !strings.Contains(err.Error(), "Usage: "+tt.use):
			t.Errorf("%q: error lacks the usage line: %v", tt.use, err)
		}
	}
}
