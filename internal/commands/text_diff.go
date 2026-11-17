package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/client"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
)

// reviewBaseURL returns the web UI's base URL for links to the update
// review, or "" if jman-api doesn't say.
func reviewBaseURL(ctx context.Context, c *client.Client) string {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	health, err := c.Health(ctx)
	if err != nil || health.WebURL == "" {
		return ""
	}
	return strings.TrimSuffix(health.WebURL, "/")
}

// printTextDiff prints one line on what a job's update text diff found
// (docs/update-text-diff.md), with a link to it in the web UI when there
// is something to review. webURL is only called then.
func printTextDiff(job models.UpdateJob, webURL func() string) {
	line, review := describeTextDiff(job)
	if line == "" {
		return
	}
	if review {
		if base := webURL(); base != "" {
			line += fmt.Sprintf(": %s/updates/%d", base, job.ID)
		}
	}
	if review {
		verb.Printf(verb.Normal, "  %s %s\n", verb.Yellow("Pages:"), line)
		return
	}
	verb.Printf(verb.Verbose, "  Pages: %s\n", line)
}

// describeTextDiff summarizes a job's text diff, and whether it found
// something to review.
func describeTextDiff(job models.UpdateJob) (string, bool) {
	s := job.TextDiffSummary
	if s == nil {
		return "", false
	}
	if s.Status == models.TextDiffInterrupted {
		return "the check was interrupted by a jman-api restart", false
	}
	var parts []string
	if s.Errors > 0 {
		parts = append(parts, verb.Red(fmt.Sprintf("%s broke", pages(s.Errors))))
	}
	if s.Changed > 0 {
		parts = append(parts, fmt.Sprintf("text changed on %s", pages(s.Changed)))
	}
	if s.NoBaseline > 0 {
		parts = append(parts, fmt.Sprintf("%s couldn't be checked", pages(s.NoBaseline)))
	}
	if len(parts) == 0 {
		return fmt.Sprintf("%s unchanged", pages(s.Pages)), false
	}
	return strings.Join(parts, ", "), true
}

func pages(n int) string {
	if n == 1 {
		return "1 page"
	}
	return fmt.Sprintf("%d pages", n)
}
