package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestSetTopLevelLine(t *testing.T) {
	line := `apiUrl = "https://jman.example.com"`
	tests := []struct {
		name, in, want string
	}{
		{"empty file", "", line + "\n"},
		{
			"replaces an existing value",
			"slackChannel = \"#ops\"\napiUrl = \"https://old.example.com\"\n",
			"slackChannel = \"#ops\"\n" + line + "\n",
		},
		{
			"adds before the first table",
			"slackChannel = \"#ops\"\n\n[[wpcliHostTimeouts]]\nhost = \"*.wpengine.net\"\n",
			"slackChannel = \"#ops\"\n" + line + "\n\n[[wpcliHostTimeouts]]\nhost = \"*.wpengine.net\"\n",
		},
		{
			"ignores apiUrl inside a table",
			"[other]\napiUrl = \"x\"\n",
			line + "\n\n[other]\napiUrl = \"x\"\n",
		},
		{"keeps comments", "# my config\n", "# my config\n" + line + "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := setTopLevelLine(tt.in, apiURLLine, line)
			if got != tt.want {
				t.Errorf("got:\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}

func TestSaveAPIURL(t *testing.T) {
	old := RunData
	RunData.ConfigDir = t.TempDir()
	t.Cleanup(func() { RunData = old })

	path := filepath.Join(RunData.ConfigDir, "config.toml")
	if err := os.WriteFile(path, []byte("slackChannel = \"#ops\"\n\n[[wpcliHostTimeouts]]\nhost = \"h\"\nminutes = 5\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SaveAPIURL("https://jman.example.com"); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	var parsed struct {
		APIURL  string `toml:"apiUrl"`
		Slack   string `toml:"slackChannel"`
		Timeout []struct {
			Host string `toml:"host"`
		} `toml:"wpcliHostTimeouts"`
	}
	if err := toml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("result is not valid TOML: %v\n%s", err, data)
	}
	if parsed.APIURL != "https://jman.example.com" || parsed.Slack != "#ops" || len(parsed.Timeout) != 1 {
		t.Errorf("unexpected result: %+v\n%s", parsed, data)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0600 {
		t.Errorf("mode = %v", info.Mode().Perm())
	}
}
