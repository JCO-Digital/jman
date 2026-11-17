package commands

import (
	"strings"
	"testing"

	"github.com/JCO-Digital/jman/internal/models"
)

func TestDescribeTextDiff(t *testing.T) {
	cases := []struct {
		summary    *models.TextDiffSummary
		want       string
		wantReview bool
	}{
		{nil, "", false},
		{&models.TextDiffSummary{Status: models.TextDiffDone, Pages: 2}, "2 pages unchanged", false},
		{&models.TextDiffSummary{Status: models.TextDiffDone, Pages: 3, Changed: 1, NoBaseline: 1}, "text changed on 1 page, 1 page couldn't be checked", true},
		{&models.TextDiffSummary{Status: models.TextDiffDone, Pages: 2, Errors: 2}, "2 pages broke", true},
		{&models.TextDiffSummary{Status: models.TextDiffInterrupted, Pages: 1}, "interrupted", false},
	}
	for _, c := range cases {
		got, review := describeTextDiff(models.UpdateJob{TextDiffSummary: c.summary})
		if !strings.Contains(got, c.want) || (c.want == "" && got != "") || review != c.wantReview {
			t.Errorf("describeTextDiff(%+v) = %q, %v; want %q, %v", c.summary, got, review, c.want, c.wantReview)
		}
	}
}
