package db

import (
	"fmt"
	"strconv"

	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
	"github.com/JCO-Digital/jman/internal/verb"
)

// SyncSpinupWPIntoInventory imports or updates SpinupWP servers and sites
// into the authoritative managed servers and sites tables in inventory.db,
// and marks the SpinupWP sites it no longer lists as removed.
func SyncSpinupWPIntoInventory(servers []models.Server, sites []models.Site) error {
	dbConn := GetInventoryDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}

	serverMap := make(map[int]models.Server, len(servers))
	for _, srv := range servers {
		serverMap[srv.ID] = srv
		managedServer := models.ManagedServer{
			ID:               utils.SpinupWPServerUUID(srv.ID),
			Provider:         "spinupwp",
			ProviderServerID: strconv.Itoa(srv.ID),
			Name:             srv.Name,
			IsLogical:        false,
			IPAddress:        srv.IPAddress,
			SSHPort:          srv.SSHPort,
		}
		if managedServer.SSHPort == 0 {
			managedServer.SSHPort = 22
		}
		if err := SaveManagedServer(managedServer); err != nil {
			return fmt.Errorf("failed to sync server %s: %w", srv.Name, err)
		}
	}

	for _, site := range sites {
		serverUUID := utils.SpinupWPServerUUID(site.ServerID)
		sshHost := ""
		sshPort := 22
		if srv, ok := serverMap[site.ServerID]; ok {
			sshHost = srv.Name
			if srv.SSHPort > 0 {
				sshPort = srv.SSHPort
			}
		}

		managedSite := models.ManagedSite{
			ID:             utils.SpinupWPSiteUUID(site.ID),
			ServerID:       &serverUUID,
			Provider:       "spinupwp",
			ProviderSiteID: strconv.Itoa(site.ID),
			Domain:         site.Domain,
			Environment:    site.Environment,
			IsWordpress:    site.IsWordpress,
			PHPVersion:     site.PHPVersion,
			ConnectionType: "agent",
			SSHHost:        sshHost,
			SSHPort:        sshPort,
			SSHUser:        site.SiteUser,
			SitePath:       "files",
			CanWPCLI:       true,
			HasAgent:       true,
			Status:         site.Status,
		}
		if err := SaveManagedSite(managedSite); err != nil {
			return fmt.Errorf("failed to sync site %s: %w", site.Domain, err)
		}
	}

	return markRemovedSpinupWPSites(sites)
}

// SiteStatusRemoved marks a SpinupWP site that SpinupWP no longer lists. It
// stays in inventory.db, so what api.db links to it (organizations, notes,
// update history) keeps its site, but drops out of the site list, the
// agent manifest and monitoring.
const SiteStatusRemoved = "removed"

// markRemovedSpinupWPSites marks the SpinupWP sites in inventory.db that
// aren't in sites, SpinupWP's current list, as removed. An empty list is
// ignored: it more likely means a failed fetch than no sites at all.
func markRemovedSpinupWPSites(sites []models.Site) error {
	if len(sites) == 0 {
		return nil
	}
	listed := make(map[string]bool, len(sites))
	for _, site := range sites {
		listed[utils.SpinupWPSiteUUID(site.ID)] = true
	}

	known, err := ListManagedSites("spinupwp")
	if err != nil {
		return fmt.Errorf("failed to list SpinupWP sites: %w", err)
	}
	dbConn := GetInventoryDB()
	for _, ms := range known {
		if listed[ms.ID] || ms.Status == SiteStatusRemoved {
			continue
		}
		if _, err := dbConn.Exec(`UPDATE sites SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, SiteStatusRemoved, ms.ID); err != nil {
			return fmt.Errorf("failed to mark site %s removed: %w", ms.Domain, err)
		}
		verb.LogPrintf(verb.Normal, "SpinupWP no longer lists %s; marked it removed", ms.Domain)
	}
	return nil
}
