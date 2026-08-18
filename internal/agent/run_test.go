package agent

import (
	"reflect"
	"testing"

	"github.com/JCO-Digital/jman/internal/models"
)

func siteIDs(sites []models.AgentManifestSite) []int {
	ids := make([]int, len(sites))
	for i, s := range sites {
		ids[i] = s.SiteID
	}
	return ids
}

func TestRotateSites(t *testing.T) {
	sites := []models.AgentManifestSite{
		{SiteID: 1}, {SiteID: 2}, {SiteID: 3}, {SiteID: 4},
	}

	cases := []struct {
		counter int
		want    []int
	}{
		{counter: 0, want: []int{1, 2, 3, 4}},
		{counter: 1, want: []int{2, 3, 4, 1}},
		{counter: 2, want: []int{3, 4, 1, 2}},
		{counter: 4, want: []int{1, 2, 3, 4}}, // wraps around
		{counter: 5, want: []int{2, 3, 4, 1}},
	}

	for _, c := range cases {
		got := siteIDs(rotateSites(sites, c.counter))
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("rotateSites(counter=%d) = %v, want %v", c.counter, got, c.want)
		}
	}
}

func TestRotateSites_NoStarvation(t *testing.T) {
	// Every site must eventually reach the front of the queue, so a
	// perpetually-backlogged site can never permanently starve another.
	sites := []models.AgentManifestSite{
		{SiteID: 10}, {SiteID: 20}, {SiteID: 30},
	}

	seenFirst := map[int]bool{}
	for counter := 0; counter < len(sites); counter++ {
		rotated := rotateSites(sites, counter)
		seenFirst[rotated[0].SiteID] = true
	}

	for _, s := range sites {
		if !seenFirst[s.SiteID] {
			t.Errorf("site %d never reached the front of the queue across a full rotation", s.SiteID)
		}
	}
}

func TestRotateSites_Empty(t *testing.T) {
	if got := rotateSites(nil, 5); len(got) != 0 {
		t.Errorf("expected empty result for empty input, got %v", got)
	}
}
