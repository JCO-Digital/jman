package models

type Enabled struct {
	Enabled bool `json:"enabled"`
}

type AdditionalDomain struct {
	Domain    string  `json:"domain"`
	Redirect  Enabled `json:"redirect"`
	CreatedAt string  `json:"created_at"`
}

type HTTPS struct {
	Enabled            bool   `json:"enabled"`
	CertificateExpires string `json:"certificate_expires"`
	CertificateRenews  string `json:"certificate_renews"`
}

type Nginx struct {
	UploadsDirectoryProtected  bool `json:"uploads_directory_protected"`
	XMLRPCProtected            bool `json:"xmlrpc_protected"`
	SubdirectoryRewriteInPlace bool `json:"subdirectory_rewrite_in_place"`
}

type SiteDatabase struct {
	ID          *int    `json:"id"`
	UserID      *int    `json:"user_id"`
	TablePrefix *string `json:"table_prefix"`
}

type StorageProvider struct {
	ID     *int    `json:"id"`
	Region *string `json:"region"`
	Bucket *string `json:"bucket"`
}

type Backups struct {
	Files                           bool             `json:"files"`
	Database                        bool             `json:"database"`
	PathsToExclude                  *string          `json:"paths_to_exclude"`
	IsBackupsRetentionPeriodEnabled *bool            `json:"is_backups_retention_period_enabled"`
	RetentionPeriod                 *int             `json:"retention_period"`
	NextRunTime                     *string          `json:"next_run_time"`
	StorageProvider                 *StorageProvider `json:"storage_provider"`
}

type BasicAuth struct {
	Enabled  bool    `json:"enabled"`
	Username *string `json:"username"`
}

type Site struct {
	ID                int                `json:"id"`
	ServerID          int                `json:"server_id"`
	Domain            string             `json:"domain"`
	AdditionalDomains []AdditionalDomain `json:"additional_domains"`
	SiteUser          string             `json:"site_user"`
	UserAuth          string             `json:"user_auth"`
	PHPVersion        string             `json:"php_version"`
	PublicFolder      string             `json:"public_folder"`
	IsWordpress       bool               `json:"is_wordpress"`
	PageCache         Enabled            `json:"page_cache"`
	HTTPS             HTTPS              `json:"https"`
	Nginx             Nginx              `json:"nginx"`
	Database          SiteDatabase       `json:"database"`
	Backups           Backups            `json:"backups"`
	WPCoreUpdate      bool               `json:"wp_core_update"`
	WPThemeUpdates    int                `json:"wp_theme_updates"`
	WPPluginUpdates   int                `json:"wp_plugin_updates"`
	BasicAuth         BasicAuth          `json:"basic_auth"`
	CreatedAt         string             `json:"created_at"`
	Status            string             `json:"status"`
}

type IgnoredSite struct {
	Domain    string `json:"domain"`
	Reason    string `json:"reason"`
	CreatedAt string `json:"created_at"`
}

type CliSite struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	ServerID   int    `json:"serverId"`
	ServerName string `json:"serverName"`
	SSH        string `json:"ssh"`
	Path       string `json:"path"`
}
