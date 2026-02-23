package models

import (
	"encoding/json"
	"strconv"
)

// DiskSpace represents the disk space metrics of a server.
type DiskSpace struct {
	Total     int64  `json:"total"`
	Available int64  `json:"available"`
	Used      int64  `json:"used"`
	UpdatedAt string `json:"updated_at"`
}

// Database represents the database configuration of a server.
type Database struct {
	Server string `json:"server"`
	Host   string `json:"host"`
	Port   int    `json:"port"`
}

// UnmarshalJSON custom unmarshaler for Database to coerce port to int.
func (d *Database) UnmarshalJSON(data []byte) error {
	type Alias Database
	aux := &struct {
		Port interface{} `json:"port"`
		*Alias
	}{
		Alias: (*Alias)(d),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	switch v := aux.Port.(type) {
	case float64:
		d.Port = int(v)
	case string:
		if v == "" {
			d.Port = 0
		} else {
			p, err := strconv.Atoi(v)
			if err != nil {
				return err
			}
			d.Port = p
		}
	}

	return nil
}

// Server represents a SpinupWP server.
type Server struct {
	ID               int       `json:"id"`
	Name             string    `json:"name"`
	ProviderName     string    `json:"provider_name"`
	UbuntuVersion    string    `json:"ubuntu_version"`
	IPAddress        string    `json:"ip_address"`
	SSHPort          int       `json:"ssh_port"`
	Timezone         string    `json:"timezone"`
	Region           string    `json:"region"`
	Size             string    `json:"size"`
	DiskSpace        DiskSpace `json:"disk_space"`
	Database         Database  `json:"database"`
	SSHPublicKey     string    `json:"ssh_publickey"`
	GitPublicKey     string    `json:"git_publickey"`
	ConnectionStatus string    `json:"connection_status"`
	RebootRequired   bool      `json:"reboot_required"`
	UpgradeRequired  bool      `json:"upgrade_required"`
	InstallNotes     string    `json:"install_notes"`
	CreatedAt        string    `json:"created_at"`
	Status           string    `json:"status"`
}
