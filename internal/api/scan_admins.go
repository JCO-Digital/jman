package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/sitestate"
	"github.com/JCO-Digital/jman/internal/verb"
)

// ruleNewAdmin is the finding jman-api raises for an administrator that
// appeared on a site without jman creating it: a hidden account is how
// attackers keep their way in after a cleanup.
const ruleNewAdmin = "new-admin"

// ruleNewAppPassword is the finding jman-api raises for an application
// password that appeared on a site: a REST API login that skips two-factor
// authentication and survives deleting rogue accounts when it's on a real
// one. jman never creates them.
const ruleNewAppPassword = "new-app-password"

func init() {
	sitestate.NewAdminsHook = reportNewAdmins
	sitestate.NewAppPasswordsHook = reportNewAppPasswords
}

// reportNewAppPasswords raises a new-app-password finding for each
// application password that appeared, high on an administrator and medium
// on anyone else, and puts them in the site's security incident.
func reportNewAppPasswords(siteID string, added []sitestate.NewAppPassword, src sitestate.Source, since time.Time) {
	now := time.Now()
	reported := 0
	for _, a := range added {
		if err := db.UpsertScanFinding(siteID, newAppPasswordFinding(a, src, since), "", now); err != nil {
			verb.LogPrintf(verb.Normal, "%v", err)
			continue
		}
		reported++
	}
	if reported > 0 {
		verb.LogPrintf(verb.Normal, "Site %s has %d new application password(s)", siteID, reported)
		triageScanFindings(siteID, true)
	}
}

// newAppPasswordFinding describes an application password that appeared on
// a site since since. A creation time before since was written by hand.
func newAppPasswordFinding(a sitestate.NewAppPassword, src sitestate.Source, since time.Time) models.ScanFinding {
	u, p := a.User, a.AppPassword
	sum := sha256.Sum256([]byte("app-password\x00" + p.UUID))
	created := time.Unix(p.Created, 0).UTC()
	lastUsed := "never"
	if p.LastUsed != nil && *p.LastUsed > 0 {
		lastUsed = time.Unix(*p.LastUsed, 0).UTC().Format(time.RFC3339)
		if p.LastIP != "" {
			lastUsed += " from " + p.LastIP
		}
	}
	detail := fmt.Sprintf("application password %q appeared for %s <%s> (%s), seen by %s", p.Name, u.Login, u.Email, u.Roles, src)
	if p.Created > 0 && created.Before(since.Add(-time.Hour)) {
		detail += fmt.Sprintf("; it claims to have been created %s, before jman last saw the site's users without it, so the date was faked", created.Format("2006-01-02"))
	}
	severity := "medium"
	if sitestate.IsAdmin(u) {
		severity = "high"
	}
	return models.ScanFinding{
		Engine:   "api",
		Rule:     ruleNewAppPassword,
		Severity: severity,
		Path:     "users/" + u.Login + "/app-passwords/" + p.UUID,
		SHA256:   hex.EncodeToString(sum[:]),
		Snippet: fmt.Sprintf("user: %s <%s> (ID %d, %s)\nname: %s\nuuid: %s\ncreated: %s\nlast used: %s",
			u.Login, u.Email, u.ID, u.Roles, p.Name, p.UUID, created.Format(time.RFC3339), lastUsed),
		Detail: detail,
	}
}

// reportNewAdmins raises a new-admin finding for each administrator jman
// didn't create, and puts them in the site's security incident.
func reportNewAdmins(siteID string, admins []models.SiteUser, src sitestate.Source) {
	created, err := adminsCreatedByJman(siteID)
	if err != nil {
		verb.LogPrintf(verb.Normal, "%v", err)
		return
	}
	now := time.Now()
	reported := 0
	for _, u := range admins {
		if created[strings.ToLower(u.Login)] {
			continue
		}
		f := newAdminFinding(u, src)
		if trustedAdminEmail(u.Email) {
			f.Severity = "medium"
		}
		if err := db.UpsertScanFinding(siteID, f, "", now); err != nil {
			verb.LogPrintf(verb.Normal, "%v", err)
			continue
		}
		reported++
	}
	if reported > 0 {
		verb.LogPrintf(verb.Normal, "Site %s has %d administrator(s) jman didn't create", siteID, reported)
		triageScanFindings(siteID, true)
	}
}

// trustedAdminEmail reports whether an email is in one of the
// trustedAdminDomains, or a subdomain of one.
func trustedAdminEmail(email string) bool {
	_, domain, ok := strings.Cut(strings.ToLower(strings.TrimSpace(email)), "@")
	if !ok || domain == "" {
		return false
	}
	for _, d := range config.Cfg.TrustedAdminDomains {
		d = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(d), "@"))
		if d != "" && (domain == d || strings.HasSuffix(domain, "."+d)) {
			return true
		}
	}
	return false
}

// newAdminFinding describes an administrator jman didn't create. The hash
// identifies the account, so a new account with the same login is a new
// finding.
func newAdminFinding(u models.SiteUser, src sitestate.Source) models.ScanFinding {
	sum := sha256.Sum256(fmt.Appendf(nil, "%d\x00%s\x00%s", u.ID, u.Login, u.Registered))
	return models.ScanFinding{
		Engine:   "api",
		Rule:     ruleNewAdmin,
		Severity: "high",
		Path:     "users/" + u.Login,
		SHA256:   hex.EncodeToString(sum[:]),
		Snippet:  fmt.Sprintf("login: %s\nemail: %s\nname: %s\nroles: %s\nregistered: %s\nuser ID: %d", u.Login, u.Email, u.DisplayName, u.Roles, u.Registered, u.ID),
		Detail:   fmt.Sprintf("administrator %s <%s> appeared on the site (seen by %s) without jman creating it", u.Login, u.Email, src),
	}
}

// adminsCreatedByJman returns the logins, lowercased, of the
// administrators jman created on a site: those in its update ledger, and
// those of user create jobs still running (a job refreshes the site's users
// before it writes the ledger).
func adminsCreatedByJman(siteID string) (map[string]bool, error) {
	created := map[string]bool{}
	entries, err := db.GetSiteUpdateLedger(siteID)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.UpdateType != "user" || e.Status != models.LedgerCreated {
			continue
		}
		var data struct {
			Login string `json:"login"`
			Role  string `json:"role"`
		}
		if json.Unmarshal([]byte(e.DataJSON), &data) == nil && data.Role == "administrator" {
			created[strings.ToLower(data.Login)] = true
		}
	}
	jobs, err := db.ListUpdateJobs(time.Now().Add(-time.Hour))
	if err != nil {
		return nil, err
	}
	for _, job := range jobs {
		if job.SiteID != siteID || job.Kind != models.UpdateJobKindUserCreate || job.Status == models.UpdateJobFailed {
			continue
		}
		if p, err := models.DecodePayload[models.UserCreatePayload](job); err == nil && p.Role == "administrator" {
			created[strings.ToLower(p.Login)] = true
		}
	}
	return created, nil
}
