package updatejobs

import (
	"context"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/textdiff"
	"github.com/JCO-Digital/jman/internal/utils"
)

// The text diff compares a site's visible text before and after a job
// that updates it (see docs/update-text-diff.md). Each page is fetched
// twice before the job, which gives the baseline and its noise, and once
// after. It only reports: a failed fetch never fails the job.

// textDiffFetchTimeout bounds one page fetch.
const textDiffFetchTimeout = 30 * time.Second

var textDiffClient = utils.NewHTTPClient(textDiffFetchTimeout)

// Seams for tests.
var (
	fetchPage = func(pageURL string, headers map[string]string) textdiff.Page {
		ctx, cancel := context.WithTimeout(context.Background(), textDiffFetchTimeout)
		defer cancel()
		return textdiff.Fetch(ctx, textDiffClient, pageURL, headers)
	}
	textDiffPaths   = sitePaths
	textDiffHeaders = siteHeaders
	textDiffMutes   = func(siteID string) ([]models.TextDiffMute, error) {
		return db.ListActiveTextDiffMutes(siteID, time.Now())
	}
	textDiffQuery = func() string { return config.Cfg.TextDiffQuery }
)

// runsTextDiff reports whether a job changes the site's code, so its
// pages are compared: plugin updates, plugin installs (which include ZIP
// updates) and core updates.
func runsTextDiff(job models.UpdateJob) bool {
	switch job.Kind {
	case models.UpdateJobKindPlugins, models.UpdateJobKindInstall, models.UpdateJobKindCore:
		return true
	}
	return false
}

// textBaseline is the state of a site's pages before a job.
type textBaseline struct {
	domain  string
	headers map[string]string
	// query is textDiffQuery, added to every fetch.
	query string
	pages []basePage
}

type basePage struct {
	path  string
	page  textdiff.Page // the first fetch that worked
	noise map[string]bool
	err   string // set when neither fetch worked
}

// takeTextBaseline fetches each of the site's pages twice and records the
// first working fetch and the lines that differ between the two. It
// stores a running text diff on the job.
func takeTextBaseline(job models.UpdateJob, site models.CliSite) *textBaseline {
	paths, err := textDiffPaths(job.SiteID)
	if err != nil {
		log.Printf("Update job %d: failed to load text diff pages, comparing the frontpage only: %v", job.ID, err)
		paths = []string{"/"}
	}
	b := &textBaseline{domain: site.Name, headers: textDiffHeaders(job.SiteID, site.Name), query: extraQuery(job.ID)}
	diff := models.TextDiff{Status: models.TextDiffRunning}
	for _, path := range paths {
		bp := basePage{path: path}
		first := fetchPage(b.url(path), b.headers)
		second := fetchPage(b.url(path), b.headers)
		switch {
		case first.Error == "" && second.Error == "":
			bp.page = first
			bp.noise = textdiff.Noise(first.Lines, second.Lines)
		case first.Error == "":
			bp.page = first
		case second.Error == "":
			bp.page = second
		default:
			bp.err = first.Error
		}
		b.pages = append(b.pages, bp)
		diff.Pages = append(diff.Pages, models.TextDiffPage{Path: path})
	}
	if err := db.SetUpdateJobTextDiff(job.ID, diff); err != nil {
		log.Printf("Update job %d: %v", job.ID, err)
	}
	return b
}

func (b *textBaseline) url(path string) string {
	u := "https://" + b.domain + path
	if b.query == "" {
		return u
	}
	if strings.Contains(path, "?") {
		return u + "&" + b.query
	}
	return u + "?" + b.query
}

// extraQuery returns the textDiffQuery setting, without a leading "?" or
// "&", or "" if it isn't a valid query string.
func extraQuery(jobID int64) string {
	q := strings.TrimLeft(strings.TrimSpace(textDiffQuery()), "?&")
	if q == "" {
		return ""
	}
	if _, err := url.ParseQuery(q); err != nil || strings.ContainsAny(q, " #") {
		log.Printf("Update job %d: ignoring textDiffQuery %q: not a valid query string", jobID, q)
		return ""
	}
	return q
}

// compareText fetches each page again after the job and stores the
// finished text diff on the job.
func compareText(job models.UpdateJob, b *textBaseline) {
	mutes, err := textDiffMutes(job.SiteID)
	if err != nil {
		log.Printf("Update job %d: failed to load text diff mutes: %v", job.ID, err)
	}
	diff := models.TextDiff{Status: models.TextDiffDone}
	for _, bp := range b.pages {
		after := fetchPage(b.url(bp.path), b.headers)
		diff.Pages = append(diff.Pages, comparePage(bp, after, mutes))
	}
	if err := db.SetUpdateJobTextDiff(job.ID, diff); err != nil {
		log.Printf("Update job %d: %v", job.ID, err)
	}
}

func comparePage(bp basePage, after textdiff.Page, mutes []models.TextDiffMute) models.TextDiffPage {
	p := models.TextDiffPage{Path: bp.path}
	switch {
	case bp.err != "":
		p.Outcome = models.TextDiffNoBaseline
		p.Error = bp.err
		return p
	case after.Error != "":
		p.Outcome = models.TextDiffError
		p.Error = after.Error
		p.FinalURLBefore = bp.page.FinalURL
		return p
	}

	muted := map[[2]string]bool{}
	for _, m := range mutes {
		if m.Path == bp.path {
			muted[[2]string{m.Direction, m.Line}] = true
		}
	}
	hide := func(kind, norm string) bool {
		return bp.noise[norm] || muted[[2]string{kind, norm}]
	}
	p.Hunks, p.Hidden = textdiff.Diff(bp.page.Lines, after.Lines, hide)
	p.Outcome = models.TextDiffUnchanged
	if len(p.Hunks) > 0 {
		p.Outcome = models.TextDiffChanged
	}
	if bp.page.FinalURL != after.FinalURL {
		p.FinalURLBefore, p.FinalURLAfter = bp.page.FinalURL, after.FinalURL
		p.Outcome = models.TextDiffChanged
	}
	return p
}

// sitePaths returns the frontpage and the site's extra pages.
func sitePaths(siteID string) ([]string, error) {
	paths := []string{"/"}
	pages, err := db.ListSiteDiffPages(siteID)
	if err != nil {
		return paths, err
	}
	for _, p := range pages {
		if p.Path != "/" {
			paths = append(paths, p.Path)
		}
	}
	return paths, nil
}

// siteHeaders returns the secret headers of the site's default uptime
// check (basic auth on a staging site, for example), if its target is the
// site's own domain.
func siteHeaders(siteID, domain string) map[string]string {
	checks, err := db.ListMonitorChecks()
	if err != nil {
		log.Printf("Text diff: failed to load monitor checks: %v", err)
		return nil
	}
	for _, c := range checks {
		if c.SiteID != siteID || !c.IsDefault || (c.Host != "" && !strings.EqualFold(c.Host, domain)) {
			continue
		}
		secrets, err := db.GetMonitorCheckSecretValues()
		if err != nil {
			log.Printf("Text diff: failed to load monitor check secrets: %v", err)
			return nil
		}
		return secrets[c.ID]
	}
	return nil
}

// ValidTextDiffPath checks an extra page's path and returns it cleaned: it
// must start with "/", stay on the site (no scheme or host) and have no
// fragment.
func ValidTextDiffPath(path string) (string, bool) {
	path = strings.TrimSpace(path)
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.ContainsAny(path, " \t\r\n#\\") {
		return "", false
	}
	u, err := url.Parse(path)
	if err != nil || u.Scheme != "" || u.Host != "" {
		return "", false
	}
	return path, len(path) <= 500
}
