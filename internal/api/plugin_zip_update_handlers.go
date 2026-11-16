package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/updatejobs"
	"github.com/JCO-Digital/jman/internal/utils"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/JCO-Digital/jman/internal/wpcli"
)

// zipDownloadTimeout bounds fetching a plugin ZIP from a URL.
const zipDownloadTimeout = 2 * time.Minute

// maxZipUpdateSites bounds the sites one ZIP update request may queue.
const maxZipUpdateSites = 500

// maxZipRedirects bounds the redirects followed when downloading a ZIP.
const maxZipRedirects = 5

// zipHTTPClient downloads plugin ZIPs. It follows redirects from an allowed
// URL (GitHub serves release assets from another host), but only over
// HTTPS.
var zipHTTPClient = func() *http.Client {
	c := utils.NewHTTPClient(zipDownloadTimeout)
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxZipRedirects {
			return errors.New("too many redirects")
		}
		if req.URL.Scheme != "https" {
			return errors.New("redirect to a non-HTTPS URL")
		}
		return nil
	}
	return c
}()

// zipURLPrefixes returns the configured URL prefixes plugin ZIPs may be
// downloaded from. A seam for tests.
var zipURLPrefixes = func() []string { return config.Cfg.PluginZipURLPrefixes }

// PluginZipSourcesHandler returns the URL prefixes plugin ZIPs may be
// downloaded from, for the ZIP update form.
//
//	GET /api/plugin-zips/sources
func PluginZipSourcesHandler(w http.ResponseWriter, r *http.Request) {
	prefixes := []string{}
	for _, p := range zipURLPrefixes() {
		if _, ok := parseZipURLPrefix(p); ok {
			prefixes = append(prefixes, strings.TrimSpace(p))
		}
	}
	WriteJSON(w, http.StatusOK, map[string][]string{"url_prefixes": prefixes})
}

// parseZipURLPrefix parses a configured URL prefix; ok is false if it
// isn't an http(s) URL with a host and no query, fragment or user info.
func parseZipURLPrefix(raw string) (u *url.URL, ok bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" || hasDotSegment(u.Path) {
		return nil, false
	}
	return u, true
}

// zipURLAllowed reports whether u is under one of the URL prefixes. A
// prefix's path matches whole path segments: "/JCO-Digital" and
// "/JCO-Digital/" match "/JCO-Digital/x" but not "/JCO-Digital-x". URLs
// with user info, dot segments or escaped path characters (such as %2F),
// which could reach another path than they appear to, never match.
func zipURLAllowed(u *url.URL, prefixes []string) bool {
	if u.User != nil || u.RawPath != "" || hasDotSegment(u.Path) {
		return false
	}
	for _, raw := range prefixes {
		p, ok := parseZipURLPrefix(raw)
		if !ok || !strings.EqualFold(p.Scheme, u.Scheme) || !strings.EqualFold(p.Host, u.Host) {
			continue
		}
		dir := strings.TrimSuffix(p.Path, "/")
		if dir == "" || u.Path == dir || strings.HasPrefix(u.Path, dir+"/") {
			return true
		}
	}
	return false
}

func hasDotSegment(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		if seg == "." || seg == ".." {
			return true
		}
	}
	return false
}

// StagePluginZipHandler stores a plugin ZIP for a ZIP update and reports
// the plugin it holds (slug, name and version). The ZIP is an uploaded
// file (multipart/form-data, field "file") or a URL that jman-api
// downloads itself, so sites don't need access to it and every site gets
// the same, checked file. URLs must be under one of the configured
// pluginZipUrlPrefixes. A staged ZIP expires after an hour.
//
//	POST /api/plugin-zips
//	{"url": "https://example.com/download?key=..."}
func StagePluginZipHandler(w http.ResponseWriter, r *http.Request) {
	var filename, zipPath string
	var size int64

	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		name, p, _, status, err := savePluginUpload(w, r)
		if err != nil {
			if status == http.StatusInternalServerError {
				verb.LogPrintf(verb.Normal, "StagePluginZipHandler: %v", err)
				WriteError(w, status, "Failed to store the uploaded file")
			} else {
				WriteError(w, status, err.Error())
			}
			return
		}
		filename, zipPath = name, p
		if fi, err := os.Stat(p); err == nil {
			size = fi.Size()
		}
	} else {
		var body struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid request body")
			return
		}
		name, p, n, status, err := downloadPluginZip(r, body.URL)
		if err != nil {
			if status == http.StatusInternalServerError {
				verb.LogPrintf(verb.Normal, "StagePluginZipHandler: %v", err)
				WriteError(w, status, "Failed to store the downloaded file")
			} else {
				WriteError(w, status, err.Error())
			}
			return
		}
		filename, zipPath, size = name, p, n
	}

	info := wpcli.InspectPluginZip(zipPath, filename)
	if info.Name == "" {
		_ = os.Remove(zipPath)
		WriteError(w, http.StatusBadRequest, "The ZIP doesn't hold a WordPress plugin: no PHP file with a Plugin Name header was found where WordPress looks for one")
		return
	}

	z, err := updatejobs.StageZip(updatejobs.StagedZip{
		Filename:  filename,
		Size:      size,
		Plugin:    info,
		CreatedBy: getUsername(r),
		Path:      zipPath,
	})
	if err != nil {
		_ = os.Remove(zipPath)
		verb.LogPrintf(verb.Normal, "StagePluginZipHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to stage the ZIP")
		return
	}
	WriteJSON(w, http.StatusCreated, z)
}

// downloadPluginZip fetches a ZIP from an http(s) URL into the upload
// directory and returns a filename for it (a .zip name from the response
// or the URL), its path and size. On error, status is the HTTP status to
// answer with.
func downloadPluginZip(r *http.Request, rawURL string) (name, zipPath string, size int64, status int, err error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", "", 0, http.StatusBadRequest, fmt.Errorf("The URL must be an http(s) URL")
	}
	prefixes := zipURLPrefixes()
	if len(prefixes) == 0 {
		return "", "", 0, http.StatusForbidden, fmt.Errorf("Downloading ZIPs from URLs is off: no URLs are allowed (pluginZipUrlPrefixes in jman-api's config). Upload the ZIP instead")
	}
	if !zipURLAllowed(u, prefixes) {
		return "", "", 0, http.StatusForbidden, fmt.Errorf("ZIPs can only be downloaded from URLs under %s", strings.Join(prefixes, ", "))
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, u.String(), nil)
	if err != nil {
		return "", "", 0, http.StatusBadRequest, fmt.Errorf("Invalid URL")
	}
	res, err := zipHTTPClient.Do(req)
	if err != nil {
		return "", "", 0, http.StatusBadGateway, fmt.Errorf("Failed to download the ZIP from %s", u.Host)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", "", 0, http.StatusBadGateway, fmt.Errorf("Downloading the ZIP failed: %s answered %s", u.Host, res.Status)
	}
	if res.ContentLength > maxPluginUploadBytes {
		return "", "", 0, http.StatusRequestEntityTooLarge, fmt.Errorf("The file is larger than %d MB", maxPluginUploadBytes>>20)
	}

	zipPath, size, err = storePluginZip(io.LimitReader(res.Body, maxPluginUploadBytes+1))
	switch {
	case errors.Is(err, errNotZip):
		return "", "", 0, http.StatusBadRequest, fmt.Errorf("The URL didn't return a ZIP file")
	case err != nil:
		return "", "", 0, http.StatusInternalServerError, err
	case size > maxPluginUploadBytes:
		_ = os.Remove(zipPath)
		return "", "", 0, http.StatusRequestEntityTooLarge, fmt.Errorf("The file is larger than %d MB", maxPluginUploadBytes>>20)
	}
	return downloadFilename(res, u), zipPath, size, http.StatusOK, nil
}

// downloadFilename names a downloaded ZIP after the response's
// Content-Disposition filename or the URL path, if either is a .zip name.
func downloadFilename(res *http.Response, u *url.URL) string {
	if _, params, err := mime.ParseMediaType(res.Header.Get("Content-Disposition")); err == nil {
		if name := path.Base(params["filename"]); strings.HasSuffix(strings.ToLower(name), ".zip") {
			return name
		}
	}
	if name := path.Base(u.Path); strings.HasSuffix(strings.ToLower(name), ".zip") {
		return name
	}
	return "plugin.zip"
}

// PluginZipUpdateHandler queues updating a plugin from a staged ZIP (see
// StagePluginZipHandler) on a set of sites: one install job per site,
// sharing the ZIP. The ZIP must hold the plugin, and every site must have
// it installed. WP-CLI replaces the plugin's files and keeps it active or
// inactive as it was. Update locks don't apply: the ZIP's version is what
// the user picked.
//
//	POST /api/plugins/{slug}/zip-update
//	{"zip_id": "<id>", "site_ids": ["<uuid>", ...]}
func PluginZipUpdateHandler(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if !pluginSlugRegex.MatchString(slug) || strings.Contains(slug, "/") {
		WriteError(w, http.StatusBadRequest, "Invalid plugin slug")
		return
	}
	var body struct {
		ZipID   string   `json:"zip_id"`
		SiteIDs []FlexID `json:"site_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if len(body.SiteIDs) == 0 {
		WriteError(w, http.StatusBadRequest, "Select at least one site")
		return
	}
	if len(body.SiteIDs) > maxZipUpdateSites {
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("At most %d sites per request", maxZipUpdateSites))
		return
	}

	sites, err := cache.GetFastSiteList()
	if err != nil {
		verb.LogPrintf(verb.Normal, "PluginZipUpdateHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to load site list")
		return
	}
	reachable := make(map[string]bool, len(sites))
	for _, s := range sites {
		reachable[s.ID] = true
	}

	// Validate every site before taking the ZIP.
	type target struct{ siteID, version string }
	targets := make([]target, 0, len(body.SiteIDs))
	seen := map[string]bool{}
	for _, raw := range body.SiteIDs {
		siteID, err := resolveSiteUUID(string(raw))
		if err != nil {
			WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if seen[siteID] {
			continue
		}
		seen[siteID] = true
		if !reachable[siteID] {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("Site %s not found or not reachable via WP-CLI", siteID))
			return
		}
		version, err := zipUpdateTarget(siteID, slug)
		if err != nil {
			WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		targets = append(targets, target{siteID, version})
	}

	z, ok := updatejobs.TakeStagedZip(body.ZipID, getUsername(r))
	if !ok {
		WriteError(w, http.StatusNotFound, "The ZIP has expired or was already used; upload it again")
		return
	}
	if z.Plugin.Slug != slug {
		updatejobs.RestageZip(z)
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("The ZIP holds the plugin %q, not %q", z.Plugin.Slug, slug))
		return
	}

	jobs := make([]models.UpdateJob, len(targets))
	for i, t := range targets {
		jobs[i] = models.NewUpdateJob(models.UpdateJobKindInstall, t.siteID, models.InstallPayload{
			Source:     z.Filename,
			Plugin:     slug,
			OldVersion: t.version,
		})
		jobs[i].UploadPath = z.Path
		jobs[i].CreatedBy = getUsername(r)
	}
	n, err := updatejobs.EnqueueAll(jobs)
	if err != nil {
		verb.LogPrintf(verb.Normal, "PluginZipUpdateHandler: queued %d of %d jobs: %v", n, len(jobs), err)
		if n == 0 {
			updatejobs.RestageZip(z)
			WriteError(w, http.StatusInternalServerError, "Failed to queue the updates")
			return
		}
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Queued the update on only %d of %d sites", n, len(jobs)))
		return
	}
	WriteJSON(w, http.StatusCreated, jobs)
}

// zipUpdateTarget returns the cached version of the plugin on a site, or an
// error if the site doesn't have it installed in a way a ZIP can update.
func zipUpdateTarget(siteID, slug string) (string, error) {
	plugins, err := db.GetSitePlugins(siteID)
	if err != nil {
		return "", fmt.Errorf("Failed to read the plugins of site %s", siteID)
	}
	for _, p := range plugins {
		if p.Name != slug {
			continue
		}
		if p.Status == "must-use" || p.Status == "dropin" {
			return "", fmt.Errorf("Plugin %s is a %s plugin on site %s and can't be updated from a ZIP", slug, p.Status, siteID)
		}
		return p.Version, nil
	}
	return "", fmt.Errorf("Plugin %s is not installed on site %s according to the cached plugin list", slug, siteID)
}
