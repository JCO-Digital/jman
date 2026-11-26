package api

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/update"
	"github.com/JCO-Digital/jman/internal/updatejobs"
	"github.com/JCO-Digital/jman/internal/utils"
	"github.com/JCO-Digital/jman/internal/verb"
)

const (
	// maxMuPluginBytes bounds a mu-plugin file, downloaded or uploaded.
	maxMuPluginBytes = 1 << 20
	// maxBulkJobSites bounds the sites of one request that queues a job
	// per site (mu-plugin installs, DISALLOW_FILE_MODS changes).
	maxBulkJobSites = 1000

	muPluginReleasesURL = "https://api.github.com/repos/JCO-Digital/bojaco-mu-plugin/releases/latest"
	// muPluginDownloadPrefix is the only place a bojaco mu-plugin URL may
	// point to: a release asset.
	muPluginDownloadPrefix = "https://github.com/JCO-Digital/bojaco-mu-plugin/releases/download/"
	compatRawURL           = "https://raw.githubusercontent.com/JCO-Digital/bojaco-compat/%s/%s"
)

// errMuSourceNotFound is a source that answered 404.
var errMuSourceNotFound = errors.New("not found")

// Seams for tests.
var (
	// fetchMuSource downloads a mu-plugin file, following redirects only
	// over HTTPS, as for plugin ZIPs.
	fetchMuSource = func(url string) ([]byte, error) {
		resp, err := zipHTTPClient.Get(url)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		switch resp.StatusCode {
		case http.StatusOK:
		case http.StatusNotFound:
			return nil, errMuSourceNotFound
		default:
			return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, maxMuPluginBytes+1))
		if err != nil {
			return nil, err
		}
		if len(data) > maxMuPluginBytes {
			return nil, fmt.Errorf("the file is larger than %d bytes", maxMuPluginBytes)
		}
		return data, nil
	}
	// latestMuPluginURL returns the PHP asset of the latest bojaco
	// mu-plugin release.
	latestMuPluginURL = func() (string, error) {
		release, err := update.GetLatestRelease(muPluginReleasesURL)
		if err != nil {
			return "", err
		}
		for _, asset := range release.Assets {
			if strings.HasSuffix(asset.Name, ".php") {
				return asset.BrowserDownloadURL, nil
			}
		}
		return "", fmt.Errorf("the latest release %s has no .php asset", release.TagName)
	}
)

// muPluginRequest is what InstallMuPluginHandler is asked to install.
type muPluginRequest struct {
	Sites []string `json:"sites"`
	// Plugin is "bojaco" or "compat".
	Plugin string `json:"plugin"`
	// Slug is the plugin a compat plugin is for.
	Slug string `json:"slug,omitempty"`
	// Source is a bojaco mu-plugin release asset URL; the latest release
	// is used without it.
	Source string `json:"source,omitempty"`
}

// muPluginFile is a mu-plugin file ready to be installed.
type muPluginFile struct {
	name, source string
	data         []byte
}

// InstallMuPluginHandler queues writing the bojaco mu-plugin or a
// bojaco-compat plugin into the mu-plugins of a set of sites. jman-api
// gets the file itself (from GitHub, or uploaded for the bojaco
// mu-plugin) and adds its hash to the scan allowlist before queueing, so
// the file scan doesn't raise a security incident for jman's own change.
// Open findings with the hash, e.g. from an earlier install, are marked
// allowlisted.
//
//	POST /api/mu-plugins
//	{"sites": ["<uuid>", …], "plugin": "bojaco"}
//	{"sites": ["<uuid>", …], "plugin": "compat", "slug": "woocommerce"}
//
// or multipart/form-data with "sites" (comma-separated), "plugin"
// ("bojaco") and the PHP file in "file".
func InstallMuPluginHandler(w http.ResponseWriter, r *http.Request) {
	var req muPluginRequest
	var upload *muPluginFile
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		var status int
		var err error
		req, upload, status, err = readMuPluginUpload(w, r)
		if err != nil {
			WriteError(w, status, err.Error())
			return
		}
	} else if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	sites, err := bulkJobSites(req.Sites)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	file := upload
	if file == nil {
		var status int
		file, status, err = fetchMuPlugin(req)
		if err != nil {
			if status == http.StatusBadGateway {
				verb.LogPrintf(verb.Normal, "InstallMuPluginHandler: %v", err)
			}
			WriteError(w, status, err.Error())
			return
		}
	} else if req.Source != "" || req.Slug != "" {
		WriteError(w, http.StatusBadRequest, "An uploaded file can't also have a source or slug")
		return
	}
	if !bytes.HasPrefix(file.data, []byte("<?php")) {
		WriteError(w, http.StatusUnprocessableEntity, "The file doesn't start with <?php")
		return
	}
	sum := sha256.Sum256(file.data)
	hash := hex.EncodeToString(sum[:])

	uploadPath, err := storeMuPlugin(file.data)
	if err != nil {
		verb.LogPrintf(verb.Normal, "InstallMuPluginHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to store the file")
		return
	}

	by := getUsername(r)
	if err := allowlistMuPlugin(file, hash, sites[0], by); err != nil {
		_ = os.Remove(uploadPath)
		verb.LogPrintf(verb.Normal, "InstallMuPluginHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to add the file to the scan allowlist")
		return
	}

	payload := models.MuInstallPayload{File: file.name, Source: file.source, SHA256: hash}
	jobs := make([]models.UpdateJob, len(sites))
	for i, siteID := range sites {
		jobs[i] = models.NewUpdateJob(models.UpdateJobKindMuInstall, siteID, payload)
		jobs[i].UploadPath = uploadPath
		jobs[i].CreatedBy = by
		jobs[i].Priority = defaultJobPriority(len(sites))
	}
	n, err := updatejobs.EnqueueAll(jobs)
	if err != nil {
		verb.LogPrintf(verb.Normal, "InstallMuPluginHandler: %v", err)
		if n == 0 {
			_ = os.Remove(uploadPath)
			WriteError(w, http.StatusInternalServerError, "Failed to queue the installs")
			return
		}
	}
	verb.LogPrintf(verb.Normal, "%s queued %s (%s, sha256 %s) on %d site(s)", by, file.name, file.source, hash, n)
	WriteJSON(w, http.StatusAccepted, map[string]any{
		"file":   file.name,
		"source": file.source,
		"sha256": hash,
		"jobs":   jobs[:n],
	})
}

// readMuPluginUpload reads a multipart mu-plugin install request. On
// error, status is the HTTP status to answer with.
func readMuPluginUpload(w http.ResponseWriter, r *http.Request) (muPluginRequest, *muPluginFile, int, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxMuPluginBytes+1<<20)
	if err := r.ParseMultipartForm(maxMuPluginBytes + 1<<20); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return muPluginRequest{}, nil, http.StatusRequestEntityTooLarge, fmt.Errorf("The file is larger than %d bytes", maxMuPluginBytes)
		}
		return muPluginRequest{}, nil, http.StatusBadRequest, fmt.Errorf("Invalid multipart form")
	}
	defer r.MultipartForm.RemoveAll()

	req := muPluginRequest{
		Plugin: r.FormValue("plugin"),
		Slug:   r.FormValue("slug"),
		Source: r.FormValue("source"),
	}
	for _, s := range strings.Split(r.FormValue("sites"), ",") {
		if s = strings.TrimSpace(s); s != "" {
			req.Sites = append(req.Sites, s)
		}
	}
	if req.Plugin != "bojaco" {
		return req, nil, http.StatusBadRequest, fmt.Errorf("Only the bojaco mu-plugin can be uploaded")
	}
	f, header, err := r.FormFile("file")
	if err != nil {
		return req, nil, http.StatusBadRequest, fmt.Errorf("A file is required")
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxMuPluginBytes+1))
	if err != nil {
		return req, nil, http.StatusBadRequest, fmt.Errorf("Failed to read the file")
	}
	if len(data) > maxMuPluginBytes {
		return req, nil, http.StatusRequestEntityTooLarge, fmt.Errorf("The file is larger than %d bytes", maxMuPluginBytes)
	}
	name := filepath.Base(header.Filename)
	return req, &muPluginFile{name: models.MuPluginBojaco, source: "upload " + name, data: data}, 0, nil
}

// bulkJobSites resolves and checks the sites of a request, in order and
// without duplicates.
func bulkJobSites(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("At least one site is required")
	}
	if len(raw) > maxBulkJobSites {
		return nil, fmt.Errorf("At most %d sites can be changed at once", maxBulkJobSites)
	}
	seen := map[string]bool{}
	var sites []string
	for _, s := range raw {
		id, err := resolveSiteUUID(s)
		if err != nil {
			return nil, err
		}
		if seen[id] {
			continue
		}
		if _, err := getCliSite(id); err != nil {
			return nil, err
		}
		seen[id] = true
		sites = append(sites, id)
	}
	return sites, nil
}

// fetchMuPlugin downloads the file a JSON request asks for. On error,
// status is the HTTP status to answer with.
func fetchMuPlugin(req muPluginRequest) (*muPluginFile, int, error) {
	switch req.Plugin {
	case "bojaco":
		if req.Slug != "" {
			return nil, http.StatusBadRequest, fmt.Errorf("The bojaco mu-plugin takes no slug")
		}
		source := strings.TrimSpace(req.Source)
		if source == "" {
			url, err := latestMuPluginURL()
			if err != nil {
				return nil, http.StatusBadGateway, fmt.Errorf("Failed to find the latest bojaco mu-plugin release: %v", err)
			}
			source = url
		} else if !strings.HasPrefix(source, muPluginDownloadPrefix) || strings.Contains(source, "..") {
			return nil, http.StatusBadRequest, fmt.Errorf("The source must be a bojaco-mu-plugin release asset (%s…)", muPluginDownloadPrefix)
		}
		data, err := fetchMuSource(source)
		if err != nil {
			return nil, http.StatusBadGateway, fmt.Errorf("Failed to download %s: %v", source, err)
		}
		return &muPluginFile{name: models.MuPluginBojaco, source: source, data: data}, 0, nil
	case "compat":
		if req.Source != "" {
			return nil, http.StatusBadRequest, fmt.Errorf("Compat plugins are always downloaded from bojaco-compat")
		}
		if !utils.IsValidSlug(req.Slug) {
			return nil, http.StatusBadRequest, fmt.Errorf("Invalid plugin slug %q", req.Slug)
		}
		name := models.CompatFile(req.Slug)
		if !models.ValidMuPluginFile(name) {
			return nil, http.StatusBadRequest, fmt.Errorf("Invalid plugin slug %q", req.Slug)
		}
		for _, branch := range []string{"main", "master"} {
			source := fmt.Sprintf(compatRawURL, branch, name)
			data, err := fetchMuSource(source)
			if errors.Is(err, errMuSourceNotFound) {
				continue
			}
			if err != nil {
				return nil, http.StatusBadGateway, fmt.Errorf("Failed to download %s: %v", source, err)
			}
			return &muPluginFile{name: name, source: source, data: data}, 0, nil
		}
		return nil, http.StatusNotFound, fmt.Errorf("%s does not exist in the bojaco-compat repository", name)
	}
	return nil, http.StatusBadRequest, fmt.Errorf(`Plugin must be "bojaco" or "compat"`)
}

// storeMuPlugin writes a mu-plugin file to the upload directory, where
// the jobs that install it read it from.
func storeMuPlugin(data []byte) (string, error) {
	dir := updatejobs.UploadDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("failed to create upload directory: %w", err)
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "mu-"+hex.EncodeToString(id)+".php")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("failed to write %s: %w", path, err)
	}
	return path, nil
}

// allowlistMuPlugin trusts a mu-plugin file's hash in the file scan and
// marks the open findings with it allowlisted, closing incidents left
// without open findings.
func allowlistMuPlugin(file *muPluginFile, hash, siteID, by string) error {
	scanTriageMu.Lock()
	defer scanTriageMu.Unlock()
	if _, err := db.AddScanAllowlistEntry(models.ScanAllowlistEntry{
		SHA256:      hash,
		Reason:      fmt.Sprintf("Installed by jman as wp-content/mu-plugins/%s from %s", file.name, file.source),
		SiteID:      siteID,
		Rule:        "mu-plugin-change",
		Path:        "wp-content/mu-plugins/" + file.name,
		AuditFields: models.AuditFields{CreatedBy: by},
	}); err != nil {
		return err
	}
	open, err := db.ListScanFindings(db.ScanFindingFilter{SHA256: hash, Status: models.ScanFindingOpen})
	if err != nil {
		return err
	}
	if len(open) == 0 {
		return nil
	}
	if _, err := db.SetScanFindingStatus(findingIDs(open), models.ScanFindingAllowlisted, by); err != nil {
		return err
	}
	closeEmptySecurityIncidents(findingIncidents(open), by)
	return nil
}
