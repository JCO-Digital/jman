package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
)

// SiteTiers returns the site tiers in effect: siteTiers from the
// configuration, or the defaults. jman-api checks the configuration at
// startup, so handlers can rely on it being valid.
func SiteTiers() ([]models.SiteTier, error) {
	configured := make([]models.SiteTier, 0, len(config.Cfg.SiteTiers))
	for _, t := range config.Cfg.SiteTiers {
		configured = append(configured, models.SiteTier{ID: t.ID, Name: t.Name})
	}
	return models.ResolveSiteTiers(configured)
}

// ListSiteTiersHandler returns the site tiers, lowest first.
func ListSiteTiersHandler(w http.ResponseWriter, r *http.Request) {
	tiers, err := SiteTiers()
	if err != nil {
		verb.LogPrintf(verb.Normal, "ListSiteTiersHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Invalid siteTiers configuration")
		return
	}
	WriteJSON(w, http.StatusOK, tiers)
}

// SetSiteTierHandler sets a production site's tier: {"tier": 30}. A null or
// 0 tier clears it.
func SetSiteTierHandler(w http.ResponseWriter, r *http.Request) {
	siteID, err := resolveSiteUUID(r.PathValue("id"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	var body struct {
		Tier *int `json:"tier"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if body.Tier == nil || *body.Tier == 0 {
		if err := db.ClearSiteTier(siteID); err != nil {
			verb.LogPrintf(verb.Normal, "SetSiteTierHandler: %v", err)
			WriteError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		writeSiteTierState(w, siteID)
		return
	}

	tiers, err := SiteTiers()
	if err != nil {
		verb.LogPrintf(verb.Normal, "SetSiteTierHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Invalid siteTiers configuration")
		return
	}
	known := false
	for _, t := range tiers {
		if t.ID == *body.Tier {
			known = true
			break
		}
	}
	if !known {
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("Unknown tier: %d", *body.Tier))
		return
	}

	env, found, err := db.GetEffectiveSiteEnvironment(siteID)
	if err != nil {
		verb.LogPrintf(verb.Normal, "SetSiteTierHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if !found {
		WriteError(w, http.StatusNotFound, "Site not found")
		return
	}
	if env != string(models.SiteEnvironmentProduction) {
		WriteError(w, http.StatusBadRequest, "Only production sites have a tier")
		return
	}

	if err := db.SetSiteTier(siteID, *body.Tier, getUsername(r)); err != nil {
		verb.LogPrintf(verb.Normal, "SetSiteTierHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeSiteTierState(w, siteID)
}

// siteTierState is what a change of a site's environment or tier returns:
// the tier it now reports (0 for none, as GET /sites) and the default core
// update policy that follows, so the UI needn't work either out itself.
type siteTierState struct {
	Tier              int                     `json:"tier,omitempty"`
	CoreUpdateDefault models.CoreUpdatePolicy `json:"core_update_default"`
}

// writeSiteTierState answers 200 with the site's siteTierState.
func writeSiteTierState(w http.ResponseWriter, siteID string) {
	env, _, err := db.GetEffectiveSiteEnvironment(siteID)
	if err != nil {
		verb.LogPrintf(verb.Normal, "writeSiteTierState: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	var state siteTierState
	if env == string(models.SiteEnvironmentProduction) {
		tiers, err := db.GetAllSiteTiers()
		if err != nil {
			verb.LogPrintf(verb.Normal, "writeSiteTierState: %v", err)
			WriteError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		state.Tier = tiers[siteID]
	}
	state.CoreUpdateDefault = models.DefaultCoreUpdatePolicy(models.SiteEnvironmentType(env), state.Tier)
	WriteJSON(w, http.StatusOK, state)
}

// SetSiteProductionLinkHandler links a staging, development or archived site
// to its production site: {"production_site_id": "<uuid>"}. A site has one
// production site and links don't chain: the production site must be
// production and not linked itself, and a site other sites are linked to
// can't be linked.
func SetSiteProductionLinkHandler(w http.ResponseWriter, r *http.Request) {
	siteID, err := resolveSiteUUID(r.PathValue("id"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	var body struct {
		ProductionSiteID FlexID `json:"production_site_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if body.ProductionSiteID == "" {
		WriteError(w, http.StatusBadRequest, "production_site_id is required")
		return
	}
	productionID, err := resolveSiteUUID(string(body.ProductionSiteID))
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if productionID == siteID {
		WriteError(w, http.StatusBadRequest, "A site can't be linked to itself")
		return
	}

	if msg, status, err := checkSiteProductionLink(siteID, productionID); err != nil {
		verb.LogPrintf(verb.Normal, "SetSiteProductionLinkHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	} else if msg != "" {
		WriteError(w, status, msg)
		return
	}

	if err := db.SetSiteProductionLink(siteID, productionID, getUsername(r)); err != nil {
		verb.LogPrintf(verb.Normal, "SetSiteProductionLinkHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	w.WriteHeader(http.StatusOK)
}

// checkSiteProductionLink returns why siteID can't be linked to
// productionID, with the HTTP status to answer with, or "" if it can.
func checkSiteProductionLink(siteID, productionID string) (string, int, error) {
	env, found, err := db.GetEffectiveSiteEnvironment(siteID)
	if err != nil {
		return "", 0, err
	}
	if !found {
		return "Site not found", http.StatusNotFound, nil
	}
	switch models.SiteEnvironmentType(env) {
	case models.SiteEnvironmentStaging, models.SiteEnvironmentDevelopment, models.SiteEnvironmentArchived:
	default:
		return "Only staging, development and archived sites can be linked to a production site", http.StatusBadRequest, nil
	}

	prodEnv, found, err := db.GetEffectiveSiteEnvironment(productionID)
	if err != nil {
		return "", 0, err
	}
	if !found {
		return "Production site not found", http.StatusNotFound, nil
	}
	if prodEnv != string(models.SiteEnvironmentProduction) {
		return "The site to link to isn't a production site", http.StatusBadRequest, nil
	}

	links, err := db.GetAllSiteProductionLinks()
	if err != nil {
		return "", 0, err
	}
	if _, linked := links[productionID]; linked {
		return "The production site is itself linked to another site", http.StatusBadRequest, nil
	}
	n, err := db.CountSitesLinkedTo(siteID)
	if err != nil {
		return "", 0, err
	}
	if n > 0 {
		return "Other sites are linked to this site; unlink them first", http.StatusBadRequest, nil
	}
	return "", 0, nil
}

// DeleteSiteProductionLinkHandler removes a site's link to its production
// site.
func DeleteSiteProductionLinkHandler(w http.ResponseWriter, r *http.Request) {
	siteID, err := resolveSiteUUID(r.PathValue("id"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := db.DeleteSiteProductionLink(siteID); err != nil {
		verb.LogPrintf(verb.Normal, "DeleteSiteProductionLinkHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	w.WriteHeader(http.StatusOK)
}

// SetSiteCoreUpdatePolicyHandler sets a site's nightly core update policy:
// {"policy": "vuln_minor" | "minor" | "major"}. An empty or null policy
// clears the override, so the default for its environment and tier applies.
func SetSiteCoreUpdatePolicyHandler(w http.ResponseWriter, r *http.Request) {
	siteID, err := resolveSiteUUID(r.PathValue("id"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	var body struct {
		Policy *models.CoreUpdatePolicy `json:"policy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// The inventory itself: a deleted site's environment classification
	// can outlive it.
	ms, err := db.GetManagedSite(siteID)
	if err != nil {
		verb.LogPrintf(verb.Normal, "SetSiteCoreUpdatePolicyHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if ms == nil {
		WriteError(w, http.StatusNotFound, "Site not found")
		return
	}

	if body.Policy == nil || *body.Policy == "" {
		if err := db.ClearSiteCoreUpdatePolicy(siteID); err != nil {
			verb.LogPrintf(verb.Normal, "SetSiteCoreUpdatePolicyHandler: %v", err)
			WriteError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	}
	if !body.Policy.Valid() {
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("Unknown policy: %q", *body.Policy))
		return
	}
	if err := db.SetSiteCoreUpdatePolicy(siteID, *body.Policy, getUsername(r)); err != nil {
		verb.LogPrintf(verb.Normal, "SetSiteCoreUpdatePolicyHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	w.WriteHeader(http.StatusOK)
}

// NightlyCoreUpdatesHandler reports whether the nightly core update runs
// (nightlyCoreUpdates) and the hour it starts at, so the UI can say when
// the sites' policies have no effect.
func NightlyCoreUpdatesHandler(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]any{
		"enabled":    config.Cfg.NightlyCoreUpdates,
		"start_hour": config.NightlyCoreUpdateStartHour(),
	})
}
