package db

import (
	"fmt"
	"strconv"

	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
)

// SyncSpinupWPIntoInventory imports or updates SpinupWP servers and sites
// into the authoritative managed servers and sites tables in inventory.db.
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
			HasMonitoring:  true,
			Status:         site.Status,
		}
		if err := SaveManagedSite(managedSite); err != nil {
			return fmt.Errorf("failed to sync site %s: %w", site.Domain, err)
		}
	}

	return nil
}
