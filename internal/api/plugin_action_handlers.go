package api

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/updatejobs"
	"github.com/JCO-Digital/jman/internal/verb"
)

// maxPluginUploadBytes bounds an uploaded plugin ZIP.
const maxPluginUploadBytes = 64 << 20

// zipMagic is the local file header signature every non-empty ZIP starts with.
var zipMagic = []byte("PK\x03\x04")

// SitePluginsHandler returns a site's cached plugins, so clients can
// reload one site's plugin list after a change.
//
//	GET /api/sites/{id}/plugins
func SitePluginsHandler(w http.ResponseWriter, r *http.Request) {
	siteID, err := resolveSiteUUID(r.PathValue("id"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	plugins, err := db.GetSitePlugins(siteID)
	if err != nil {
		verb.LogPrintf(verb.Normal, "SitePluginsHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if plugins == nil {
		plugins = []models.WPPlugin{}
	}
	sort.Slice(plugins, func(i, j int) bool { return plugins[i].Name < plugins[j].Name })
	WriteJSON(w, http.StatusOK, plugins)
}

// SitePluginActionHandler queues a background job that activates,
// deactivates, deletes (files only) or uninstalls (runs the uninstall
// routine, usually removing the plugin's data) plugins on a site.
//
//	POST /api/sites/{id}/plugin-actions
//	{"action": "deactivate", "plugins": ["akismet"]}
func SitePluginActionHandler(w http.ResponseWriter, r *http.Request) {
	siteID, err := resolveSiteUUID(r.PathValue("id"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	var body struct {
		Action  string   `json:"action"`
		Plugins []string `json:"plugins"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if !models.IsPluginActionKind(body.Action) {
		WriteError(w, http.StatusBadRequest, `Action must be "activate", "deactivate", "delete" or "uninstall"`)
		return
	}
	names, err := uniquePluginSlugs(body.Plugins)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	cached, err := db.GetSitePlugins(siteID)
	if err != nil {
		verb.LogPrintf(verb.Normal, "SitePluginActionHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if err := checkPluginAction(body.Action, names, cached); err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if _, err := getCliSite(siteID); err != nil {
		WriteError(w, http.StatusNotFound, err.Error())
		return
	}

	job := models.UpdateJob{
		Kind:      body.Action,
		SiteID:    siteID,
		Plugins:   withInstalledVersions(siteID, names),
		CreatedBy: getUsername(r),
	}
	if err := updatejobs.Enqueue(&job); err != nil {
		verb.LogPrintf(verb.Normal, "SitePluginActionHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to queue plugin change")
		return
	}
	WriteJSON(w, http.StatusAccepted, job)
}

// checkPluginAction rejects actions that don't fit the plugins' cached
// state: must-use plugins and drop-ins can't be managed, network-activated
// plugins can't be deactivated per site, and active plugins must be
// deactivated before they're deleted or uninstalled.
func checkPluginAction(action string, names []string, cached []models.WPPlugin) error {
	status := make(map[string]string, len(cached))
	for _, p := range cached {
		status[p.Name] = p.Status
	}
	for _, name := range names {
		st, ok := status[name]
		switch {
		case !ok:
			return fmt.Errorf("Plugin %s is not installed on this site according to the cached plugin list; check for updates to refresh it", name)
		case st == "must-use" || st == "dropin":
			return fmt.Errorf("Plugin %s is a %s plugin and can't be managed here", name, st)
		case st == "active-network" && action == models.UpdateJobKindDeactivate:
			return fmt.Errorf("Plugin %s is network-activated; deactivate it in the network admin", name)
		case (action == models.UpdateJobKindDelete || action == models.UpdateJobKindUninstall) && st != "inactive":
			return fmt.Errorf("Plugin %s is %s; deactivate it before deleting it", name, st)
		}
	}
	return nil
}

// SitePluginInstallHandler queues a background install of one plugin. The
// source is a WordPress.org slug or a ZIP URL sent as JSON, or a ZIP file
// uploaded as multipart/form-data (fields "file" and "activate").
//
//	POST /api/sites/{id}/plugin-install
//	{"source": "akismet", "activate": true}
func SitePluginInstallHandler(w http.ResponseWriter, r *http.Request) {
	siteID, err := resolveSiteUUID(r.PathValue("id"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	job := models.UpdateJob{
		Kind:      models.UpdateJobKindInstall,
		SiteID:    siteID,
		CreatedBy: getUsername(r),
	}

	multipartUpload := strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data")
	if !multipartUpload {
		var body struct {
			Source   string `json:"source"`
			Activate bool   `json:"activate"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid request body")
			return
		}
		source, err := validateInstallSource(body.Source)
		if err != nil {
			WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		job.Source = source
		job.Activate = body.Activate

		// WP-CLI reports success for a slug that's already installed, so
		// catch that here. (ZIPs replace an existing plugin instead.)
		if pluginSlugRegex.MatchString(source) {
			installed, err := pluginInstalled(siteID, source)
			if err != nil {
				verb.LogPrintf(verb.Normal, "SitePluginInstallHandler: %v", err)
				WriteError(w, http.StatusInternalServerError, "Internal server error")
				return
			}
			if installed {
				WriteError(w, http.StatusConflict, fmt.Sprintf("Plugin %s is already installed on this site", source))
				return
			}
		}
	}

	// Check the site before accepting an upload onto disk.
	if _, err := getCliSite(siteID); err != nil {
		WriteError(w, http.StatusNotFound, err.Error())
		return
	}

	if multipartUpload {
		name, path, activate, status, err := savePluginUpload(w, r)
		if err != nil {
			if status == http.StatusInternalServerError {
				verb.LogPrintf(verb.Normal, "SitePluginInstallHandler: %v", err)
				WriteError(w, status, "Failed to store the uploaded file")
			} else {
				WriteError(w, status, err.Error())
			}
			return
		}
		job.Source = name
		job.UploadPath = path
		job.Activate = activate
	}

	if err := updatejobs.Enqueue(&job); err != nil {
		if job.UploadPath != "" {
			_ = os.Remove(job.UploadPath)
		}
		verb.LogPrintf(verb.Normal, "SitePluginInstallHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to queue plugin install")
		return
	}
	WriteJSON(w, http.StatusAccepted, job)
}

// pluginInstalled reports whether the site's cached plugin list has slug.
func pluginInstalled(siteID, slug string) (bool, error) {
	cached, err := db.GetSitePlugins(siteID)
	if err != nil {
		return false, err
	}
	for _, p := range cached {
		if p.Name == slug {
			return true, nil
		}
	}
	return false, nil
}

// validateInstallSource accepts a WordPress.org plugin slug or an http(s)
// URL of a ZIP file.
func validateInstallSource(source string) (string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "", fmt.Errorf("A plugin slug or ZIP URL is required")
	}
	if pluginSlugRegex.MatchString(source) && !strings.Contains(source, "/") {
		return source, nil
	}
	u, err := url.Parse(source)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("Source must be a plugin slug (e.g. akismet) or an http(s) URL of a ZIP file")
	}
	if !strings.HasSuffix(strings.ToLower(u.Path), ".zip") {
		return "", fmt.Errorf("The URL must point to a .zip file")
	}
	return u.String(), nil
}

// savePluginUpload stores the uploaded ZIP in the upload directory and
// returns its original filename, local path and the activate field. On
// error, status is the HTTP status to answer with.
func savePluginUpload(w http.ResponseWriter, r *http.Request) (name, path string, activate bool, status int, err error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxPluginUploadBytes+1<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return "", "", false, http.StatusRequestEntityTooLarge, fmt.Errorf("The file is larger than %d MB", maxPluginUploadBytes>>20)
		}
		return "", "", false, http.StatusBadRequest, fmt.Errorf("Invalid upload")
	}
	defer r.MultipartForm.RemoveAll()

	file, header, err := r.FormFile("file")
	if err != nil {
		return "", "", false, http.StatusBadRequest, fmt.Errorf("A ZIP file is required")
	}
	defer file.Close()

	name = filepath.Base(header.Filename)
	if !strings.HasSuffix(strings.ToLower(name), ".zip") {
		return "", "", false, http.StatusBadRequest, fmt.Errorf("The file must be a .zip file")
	}
	if header.Size > maxPluginUploadBytes {
		return "", "", false, http.StatusRequestEntityTooLarge, fmt.Errorf("The file is larger than %d MB", maxPluginUploadBytes>>20)
	}
	magic := make([]byte, len(zipMagic))
	if _, err := io.ReadFull(file, magic); err != nil || !bytes.Equal(magic, zipMagic) {
		return "", "", false, http.StatusBadRequest, fmt.Errorf("The file is not a valid ZIP archive")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", "", false, http.StatusInternalServerError, err
	}

	if err := os.MkdirAll(updatejobs.UploadDir(), 0o700); err != nil {
		return "", "", false, http.StatusInternalServerError, err
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return "", "", false, http.StatusInternalServerError, err
	}
	path = filepath.Join(updatejobs.UploadDir(), hex.EncodeToString(id)+".zip")
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", "", false, http.StatusInternalServerError, err
	}
	if _, err := io.Copy(out, file); err != nil {
		out.Close()
		_ = os.Remove(path)
		return "", "", false, http.StatusInternalServerError, err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(path)
		return "", "", false, http.StatusInternalServerError, err
	}

	activate = r.FormValue("activate") == "true" || r.FormValue("activate") == "1"
	return name, path, activate, http.StatusOK, nil
}
