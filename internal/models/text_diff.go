package models

import "time"

// TextDiff statuses: the check around an update job (see
// docs/update-text-diff.md).
const (
	// TextDiffRunning means the baseline was taken and the job is running
	// or the after fetch is in progress.
	TextDiffRunning = "running"
	// TextDiffDone means every page was compared (or couldn't be).
	TextDiffDone = "done"
	// TextDiffInterrupted means jman-api stopped before the check finished.
	TextDiffInterrupted = "interrupted"
)

// Page outcomes in TextDiffPage.Outcome.
const (
	TextDiffUnchanged = "unchanged"
	TextDiffChanged   = "changed"
	// TextDiffError means the page worked before the update and doesn't
	// after it.
	TextDiffError = "error"
	// TextDiffNoBaseline means the page couldn't be fetched before the
	// update, so there is nothing to compare.
	TextDiffNoBaseline = "no_baseline"
)

// TextDiff is the visible-text comparison of a site's pages before and
// after an update job.
type TextDiff struct {
	Status string         `json:"status"`
	Pages  []TextDiffPage `json:"pages"`
}

// Summary counts a diff's pages by outcome.
func (d TextDiff) Summary() TextDiffSummary {
	s := TextDiffSummary{Status: d.Status, Pages: len(d.Pages)}
	for _, p := range d.Pages {
		switch p.Outcome {
		case TextDiffChanged:
			s.Changed++
		case TextDiffError:
			s.Errors++
		case TextDiffNoBaseline:
			s.NoBaseline++
		}
	}
	return s
}

// TextDiffSummary is what job lists show of a TextDiff.
type TextDiffSummary struct {
	Status     string `json:"status"`
	Pages      int    `json:"pages"`
	Changed    int    `json:"changed"`
	Errors     int    `json:"errors"`
	NoBaseline int    `json:"no_baseline"`
}

// TextDiffPage is one page's comparison.
type TextDiffPage struct {
	// Path is the page's path on the site, "/" for the frontpage.
	Path    string `json:"path"`
	Outcome string `json:"outcome"`
	// Error says why the page couldn't be fetched: after the update for
	// TextDiffError, before it for TextDiffNoBaseline.
	Error string `json:"error,omitempty"`
	// FinalURLBefore and FinalURLAfter are the URLs after redirects. A
	// change between them makes the page TextDiffChanged.
	FinalURLBefore string         `json:"final_url_before,omitempty"`
	FinalURLAfter  string         `json:"final_url_after,omitempty"`
	Hunks          []TextDiffHunk `json:"hunks,omitempty"`
	// Hidden counts changed lines left out as noise or by a mute.
	Hidden int `json:"hidden,omitempty"`
}

// Text diff line kinds.
const (
	TextDiffContext = "context"
	TextDiffRemoved = "removed"
	TextDiffAdded   = "added"
)

// TextDiffLine is one line of a hunk, as it appeared on the page.
type TextDiffLine struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// TextDiffHunk is a run of changed lines with context around them.
type TextDiffHunk struct {
	Lines []TextDiffLine `json:"lines"`
}

// Changes counts a hunk's removed and added lines.
func (h TextDiffHunk) Changes() int {
	n := 0
	for _, l := range h.Lines {
		if l.Kind != TextDiffContext {
			n++
		}
	}
	return n
}

// SiteDiffPage is an extra page of a site that update jobs compare,
// besides the frontpage.
type SiteDiffPage struct {
	ID        int64     `json:"id"`
	SiteID    string    `json:"site_id"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"created_at"`
	CreatedBy string    `json:"created_by"`
}

// TextDiffMute hides a changed line from later text diffs of a site's
// page until it expires. Line is normalized (textdiff.Normalize), and the
// mute only hides changes in its Direction (TextDiffRemoved or
// TextDiffAdded). Expired and removed mutes are kept for the record.
type TextDiffMute struct {
	ID        int64     `json:"id"`
	SiteID    string    `json:"site_id"`
	Path      string    `json:"path"`
	Direction string    `json:"direction"`
	Line      string    `json:"line"`
	Reason    string    `json:"reason,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
	AuditFields
}

// Active reports whether the mute still hides lines at now.
func (m TextDiffMute) Active(now time.Time) bool { return now.Before(m.ExpiresAt) }
