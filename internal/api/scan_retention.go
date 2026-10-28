package api

import (
	"context"
	"net/http"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/verb"
)

// scanRetentionInterval is how often expired snippets are removed.
const scanRetentionInterval = 24 * time.Hour

// StartScanRetention removes the snippet and detail of findings triaged
// longer ago than scanSnippetRetentionDays, now and once a day until ctx
// ends. They quote files, which can hold personal data (a security
// plugin's log of failed logins, say); the rule, path and hash stay for
// the history and for reopening an incident.
func StartScanRetention(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(scanRetentionInterval)
		defer ticker.Stop()
		for {
			expireScanSnippets(time.Now())
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// expireScanSnippets removes the expired snippets as of now.
func expireScanSnippets(now time.Time) {
	retention := config.ScanSnippetRetention()
	if retention == 0 {
		return
	}
	scanTriageMu.Lock()
	defer scanTriageMu.Unlock()
	n, err := db.RedactExpiredScanFindings(now.Add(-retention), scanTriageBy)
	if err != nil {
		verb.LogPrintf(verb.Normal, "%v", err)
		return
	}
	if n > 0 {
		verb.LogPrintf(verb.Normal, "Removed the snippets of %d scan finding(s) triaged over %d days ago", n, int(retention.Hours()/24))
	}
}

// RedactScanFindingsHandler handles POST /api/scan/findings/redact: it
// removes the snippet and detail of findings at once, whatever their
// status, for content that shouldn't be kept, such as personal data.
func RedactScanFindingsHandler(w http.ResponseWriter, r *http.Request) {
	req, findings := decodeScanTriage(w, r, false)
	if req == nil {
		return
	}
	by := requestUsername(r)
	scanTriageMu.Lock()
	defer scanTriageMu.Unlock()
	n, err := db.RedactScanFindings(findingIDs(findings), by)
	if err != nil {
		verb.LogPrintf(verb.Normal, "RedactScanFindingsHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	verb.LogPrintf(verb.Normal, "%s removed the snippets of %d scan finding(s)", by, n)
	WriteJSON(w, http.StatusOK, map[string]int{"redacted": n})
}
