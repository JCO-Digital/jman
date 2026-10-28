package api

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/fetch/jcore"
	"github.com/JCO-Digital/jman/internal/fetch/wporg"
	"github.com/JCO-Digital/jman/internal/utils"
	"github.com/JCO-Digital/jman/internal/verb"
)

var (
	coreVersionRe   = regexp.MustCompile(`^[0-9]+\.[0-9]+(\.[0-9]+)?([-.][A-Za-z0-9]+)*$`)
	coreLocaleRe    = regexp.MustCompile(`^[a-z]{2,3}(_[A-Z]{2})?(_[A-Za-z0-9]+)?$`)
	pluginVersionRe = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._+-]{0,39}$`)
)

// checksumsRetry is how long jman-api remembers that wordpress.org had no
// checksums for a release before asking again: a new release or
// translation can take a while to get them.
const checksumsRetry = 6 * time.Hour

// checksumsMu makes agents asking for the same release at once share one
// fetch from wordpress.org.
var checksumsMu sync.Mutex

// releaseChecksums returns the checksums of a release as JSON, from the
// cache or, through fetch, wordpress.org. "" means wordpress.org has none.
func releaseChecksums(kind, name, version string, fetch func() (any, error)) (string, error) {
	checksumsMu.Lock()
	defer checksumsMu.Unlock()

	data, fetchedAt, found, err := db.GetReleaseChecksums(kind, name, version)
	if err != nil {
		return "", err
	}
	if found && (data != "" || time.Since(fetchedAt) < checksumsRetry) {
		return data, nil
	}
	sums, err := fetch()
	if err != nil {
		return "", err
	}
	data = ""
	if sums != nil {
		b, err := json.Marshal(sums)
		if err != nil {
			return "", err
		}
		data = string(b)
	}
	if err := db.SaveReleaseChecksums(kind, name, version, data, time.Now()); err != nil {
		verb.LogPrintf(verb.Normal, "%v", err)
	}
	return data, nil
}

// writeChecksums writes a checksums response, with the release's JSON
// checksums under key.
func writeChecksums(w http.ResponseWriter, data string, fields map[string]any, key string) {
	if data == "" {
		WriteError(w, http.StatusNotFound, "wordpress.org has no checksums for this release")
		return
	}
	fields[key] = json.RawMessage(data)
	WriteJSON(w, http.StatusOK, fields)
}

// AgentCoreChecksumsHandler handles GET /api/agent/core-checksums: the MD5
// checksums of a WordPress release (?version=&locale=) outside wp-content,
// for jman-agent's scans. 404 when wordpress.org has none.
func AgentCoreChecksumsHandler(w http.ResponseWriter, r *http.Request) {
	version, locale := r.URL.Query().Get("version"), r.URL.Query().Get("locale")
	if !coreVersionRe.MatchString(version) || len(version) > 40 || !coreLocaleRe.MatchString(locale) {
		WriteError(w, http.StatusBadRequest, "Invalid version or locale")
		return
	}
	data, err := releaseChecksums("core", locale, version, func() (any, error) {
		all, err := wporg.GetCoreChecksums(version, locale)
		if all == nil || err != nil {
			return nil, err
		}
		sums := make(map[string]string, len(all))
		for p, sum := range all {
			if !strings.HasPrefix(p, "wp-content/") {
				sums[p] = sum
			}
		}
		return sums, nil
	})
	if err != nil {
		verb.LogPrintf(verb.Normal, "AgentCoreChecksumsHandler: %v", err)
		WriteError(w, http.StatusBadGateway, "Failed to get the checksums from wordpress.org")
		return
	}
	writeChecksums(w, data, map[string]any{"version": version, "locale": locale}, "md5")
}

// AgentPluginChecksumsHandler handles GET /api/agent/plugin-checksums: the
// SHA-256 checksums of a plugin release (?slug=&version=) for jman-agent's
// scans, from wordpress.org, or for J&Co Digital's jcore-* plugins from
// their GitHub releases. 404 when there are none, as for premium plugins.
func AgentPluginChecksumsHandler(w http.ResponseWriter, r *http.Request) {
	slug, version := r.URL.Query().Get("slug"), r.URL.Query().Get("version")
	if !utils.IsValidSlug(slug) || !pluginVersionRe.MatchString(version) {
		WriteError(w, http.StatusBadRequest, "Invalid slug or version")
		return
	}
	data, err := releaseChecksums("plugin", slug, version, func() (any, error) {
		sums, err := wporg.GetPluginChecksums(slug, version)
		if sums == nil && err == nil {
			// J&Co Digital's own plugins publish on GitHub instead.
			sums, err = jcore.GetPluginChecksums(slug, version)
		}
		if sums == nil || err != nil {
			return nil, err
		}
		return sums, nil
	})
	if err != nil {
		verb.LogPrintf(verb.Normal, "AgentPluginChecksumsHandler: %v", err)
		WriteError(w, http.StatusBadGateway, "Failed to get the checksums from wordpress.org")
		return
	}
	writeChecksums(w, data, map[string]any{"slug": slug, "version": version}, "sha256")
}
