package search

import (
	"os"
	"testing"

	"github.com/JCO-Digital/jman/internal/models"
)

// withStdin runs fn with input as os.Stdin.
func withStdin(t *testing.T, input string, fn func()) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.WriteString(input)
	w.Close()
	old := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = old }()
	fn()
}

var testSites = []models.CliSite{
	{ID: "1", Name: "kehitys.jcore.fi", ServerName: "dev"},
	{ID: "2", Name: "shop.example.com", ServerName: "prod"},
	{ID: "3", Name: "blog.example.com", ServerName: "prod"},
}

func TestPromptSearchSingleMatch(t *testing.T) {
	// Read-only: no confirmation, so nothing is read from stdin (here, an
	// empty stdin would make reading fail).
	withStdin(t, "", func() {
		sites, err := PromptSearch(testSites, "kehitys", false)
		if err != nil || len(sites) != 1 || sites[0].ID != "1" {
			t.Errorf("read-only: got %v, %v", sites, err)
		}
	})

	// Changing: the user is asked, and can decline.
	withStdin(t, "n\n", func() {
		sites, err := PromptSearch(testSites, "kehitys", true)
		if err != nil || len(sites) != 0 {
			t.Errorf("declined: got %v, %v", sites, err)
		}
	})
	withStdin(t, "\n", func() {
		sites, err := PromptSearch(testSites, "kehitys", true)
		if err != nil || len(sites) != 1 {
			t.Errorf("confirmed: got %v, %v", sites, err)
		}
	})
}

func TestPromptSearchSeveralMatchesStillAsks(t *testing.T) {
	withStdin(t, "2\n", func() {
		sites, err := PromptSearch(testSites, "example.com", false)
		if err != nil || len(sites) != 1 || sites[0].ID != "3" {
			t.Errorf("got %v, %v", sites, err)
		}
	})
}
