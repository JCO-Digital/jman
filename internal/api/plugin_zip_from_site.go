package api

import (
	"archive/tar"
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/updatejobs"
	"github.com/JCO-Digital/jman/internal/wpcli"
)

// Limits on the plugin files copied off a site.
const (
	maxCopyFiles = 20000
	maxCopyBytes = 512 << 20 // uncompressed
	// maxSkippedListed bounds the skipped files reported.
	maxSkippedListed = 20
	// minScanMaxAge is the least time a source site's scan stays recent
	// enough, however often sites are scanned.
	minScanMaxAge = 2 * time.Hour
)

// Seams for tests.
var (
	pluginDir     = wpcli.PluginDir
	tarPluginDir  = wpcli.TarPluginDir
	lookupCliSite = getCliSite
)

// errZipTooLarge is returned when copied files make a ZIP over
// maxPluginUploadBytes.
var errZipTooLarge = fmt.Errorf("The plugin is larger than %d MB zipped", maxPluginUploadBytes>>20)

// copiedPlugin is a ZIP of a plugin's files copied off a site.
type copiedPlugin struct {
	path, filename string
	size           int64
	source         updatejobs.ZipSourceSite
}

// copyPluginFromSite zips a plugin's installed files on a site, for
// updating other sites to the same version. Because those files are
// spread to every site updated from them, the site must be clean: its
// latest malware scan succeeded, is recent, ran after the plugin's files
// last changed, and the site has no open scan findings. On error, status
// is the HTTP status to answer with.
func copyPluginFromSite(rawSiteID, slug string) (c copiedPlugin, status int, err error) {
	siteID, err := resolveSiteUUID(rawSiteID)
	if err != nil {
		return c, http.StatusBadRequest, err
	}
	if !pluginSlugRegex.MatchString(slug) || strings.Contains(slug, "/") {
		return c, http.StatusBadRequest, fmt.Errorf("Invalid plugin slug")
	}
	site, err := lookupCliSite(siteID)
	if err != nil {
		return c, http.StatusNotFound, fmt.Errorf("Site %s not found or not reachable via WP-CLI", siteID)
	}
	if _, err := zipUpdateTarget(siteID, slug); err != nil {
		return c, http.StatusBadRequest, err
	}
	scannedAt, status, err := checkSourceSiteScan(siteID, site.Name)
	if err != nil {
		return c, status, err
	}

	dir, err := pluginDir(siteID, site.SSH, site.Path, slug)
	if err != nil {
		return c, http.StatusBadGateway, fmt.Errorf("Couldn't copy %s from %s: %v", slug, site.Name, err)
	}
	out, zipPath, err := newPluginZipFile()
	if err != nil {
		return c, http.StatusInternalServerError, err
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.Remove(zipPath)
		}
	}()

	counter := &countingWriter{w: out, max: maxPluginUploadBytes}
	var res tarZipResult
	copyErr := tarPluginDir(site.SSH, dir, func(r io.Reader) error {
		var err error
		res, err = tarToZip(r, slug, counter)
		return err
	})
	if cerr := out.Close(); copyErr == nil && cerr != nil {
		return c, http.StatusInternalServerError, cerr
	}
	switch {
	case errors.Is(copyErr, errZipTooLarge):
		return c, http.StatusRequestEntityTooLarge, copyErr
	case errors.Is(copyErr, errCopyRefused):
		return c, http.StatusBadRequest, fmt.Errorf("Couldn't copy %s from %s: %v", slug, site.Name, copyErr)
	case copyErr != nil:
		return c, http.StatusBadGateway, fmt.Errorf("Couldn't copy %s from %s: %v", slug, site.Name, copyErr)
	case res.files == 0:
		return c, http.StatusBadGateway, fmt.Errorf("Couldn't copy %s from %s: the plugin's directory has no files", slug, site.Name)
	}
	if res.newest.After(scannedAt) {
		return c, http.StatusConflict, fmt.Errorf("%s changed on %s after its latest malware scan (%s, at %s UTC), so the change hasn't been scanned. Try again after the next scan",
			slug, site.Name, res.newestPath, res.newest.UTC().Format("2006-01-02 15:04"))
	}

	keep = true
	return copiedPlugin{
		path:     zipPath,
		filename: slug + ".zip",
		size:     counter.n,
		source: updatejobs.ZipSourceSite{
			ID:        siteID,
			Name:      site.Name,
			ScannedAt: scannedAt,
			Skipped:   res.skipped,
		},
	}, http.StatusOK, nil
}

// checkSourceSiteScan checks that a site's malware scan results allow
// copying files from it, and returns when it was last scanned.
func checkSourceSiteScan(siteID, siteName string) (time.Time, int, error) {
	interval := config.AgentScanIntervalMinutes()
	if interval == 0 {
		return time.Time{}, http.StatusConflict, fmt.Errorf("Malware scanning is off (agentScanInterval), so plugins can't be copied from sites")
	}
	scannedAt, scanErr, scanned, err := db.GetSiteScan(siteID)
	switch {
	case err != nil:
		return time.Time{}, http.StatusInternalServerError, err
	case !scanned:
		return time.Time{}, http.StatusConflict, fmt.Errorf("%s has never been scanned for malware (it needs jman-agent), so plugins can't be copied from it", siteName)
	case scanErr != "":
		return time.Time{}, http.StatusConflict, fmt.Errorf("The latest malware scan of %s failed, so plugins can't be copied from it", siteName)
	}
	maxAge := max(2*time.Duration(interval)*time.Minute, minScanMaxAge)
	if age := time.Since(scannedAt); age > maxAge {
		return time.Time{}, http.StatusConflict, fmt.Errorf("%s was last scanned for malware %s ago, so plugins can't be copied from it until it's scanned again", siteName, age.Round(time.Minute))
	}
	open, err := db.CountOpenSiteScanFindings(siteID)
	if err != nil {
		return time.Time{}, http.StatusInternalServerError, err
	}
	if open > 0 {
		return time.Time{}, http.StatusConflict, fmt.Errorf("%s has %d open malware scan finding(s). Plugins can only be copied from sites without any; review them under Monitoring → Security.", siteName, open)
	}
	return scannedAt, http.StatusOK, nil
}

// errCopyRefused marks a tar stream tarToZip won't turn into a ZIP.
var errCopyRefused = errors.New("refused")

type tarZipResult struct {
	files int
	// newest is the latest modification time of a copied file.
	newest     time.Time
	newestPath string
	skipped    []string
}

// tarToZip writes the regular files and directories of a tar stream whose
// entries are all under "<slug>/" to a ZIP. Symlinks and other special
// files are skipped: a symlink could point at a file outside the plugin,
// such as wp-config.php.
func tarToZip(r io.Reader, slug string, w io.Writer) (res tarZipResult, err error) {
	zw := zip.NewWriter(w)
	tr := tar.NewReader(r)
	var total int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return res, fmt.Errorf("reading the files failed: %w", err)
		}
		name := strings.TrimPrefix(hdr.Name, "./")
		clean := path.Clean(name)
		if (clean != slug && !strings.HasPrefix(clean, slug+"/")) || hasDotSegment(name) {
			return res, fmt.Errorf("%w: unexpected path %q", errCopyRefused, hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if _, err := zw.CreateHeader(&zip.FileHeader{Name: clean + "/", Modified: hdr.ModTime}); err != nil {
				return res, err
			}
			continue
		case tar.TypeReg:
		default:
			if len(res.skipped) < maxSkippedListed {
				res.skipped = append(res.skipped, clean)
			}
			continue
		}

		res.files++
		total += hdr.Size
		if res.files > maxCopyFiles {
			return res, fmt.Errorf("%w: the plugin has more than %d files", errCopyRefused, maxCopyFiles)
		}
		if total > maxCopyBytes {
			return res, fmt.Errorf("%w: the plugin is larger than %d MB", errCopyRefused, maxCopyBytes>>20)
		}
		if hdr.ModTime.After(res.newest) {
			res.newest, res.newestPath = hdr.ModTime, clean
		}
		fh := &zip.FileHeader{Name: clean, Method: zip.Deflate, Modified: hdr.ModTime}
		fh.SetMode(os.FileMode(hdr.Mode).Perm() | 0o600)
		fw, err := zw.CreateHeader(fh)
		if err != nil {
			return res, err
		}
		if _, err := io.Copy(fw, tr); err != nil {
			return res, err
		}
	}
	return res, zw.Close()
}

// countingWriter counts what's written through it and fails with
// errZipTooLarge past max bytes.
type countingWriter struct {
	w   io.Writer
	n   int64
	max int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	if c.n+int64(len(p)) > c.max {
		return 0, errZipTooLarge
	}
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}
