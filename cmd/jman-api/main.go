package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/JCO-Digital/jman/internal/api"
	"github.com/JCO-Digital/jman/internal/backup"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/monitor"
	"github.com/JCO-Digital/jman/internal/refresh"
	"github.com/JCO-Digital/jman/internal/tasks"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "jman-api",
	Short: "REST API server and admin tools for jman",
	Long: `jman-api serves cached jman data over a REST API with JWT authentication.

Run without a subcommand to start the API server.
Use subcommands (useradd, hashpw, totp-setup) to manage users and credentials.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Init(); err != nil {
			return err
		}
		if err := db.CheckSplitState(); err != nil {
			return err
		}
		if err := db.InitInventory(); err != nil {
			return err
		}
		return db.InitAPI()
	},
	RunE: runServe,
}

func runServe(cmd *cobra.Command, args []string) error {
	// Load users config (fatal if missing or invalid — authentication is mandatory).
	usersCfg, err := config.LoadUsersConfig(config.RunData.ConfigDir)
	if err != nil {
		return fmt.Errorf("failed to load users config: %w", err)
	}

	port := os.Getenv("JMAN_API_PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()

	// Register API routes from the internal/api package
	api.RegisterHandlers(mux, config.AppVersion, usersCfg, config.Cfg.BehindProxy)

	// Wrap mux with middleware
	handler := api.LoggingMiddleware(
		api.SecurityHeadersMiddleware(
			api.CorsMiddleware(
				api.MaxBodyMiddleware(
					api.JsonMiddleware(mux),
				),
			),
		),
	)

	// Start the database backup scheduler
	backup.StartScheduler(cmd.Context())

	// Start the task scheduler
	tasks.StartScheduler(cmd.Context())

	// Start the data refresh scheduler (replaces the external `jman fetch` cron job)
	refresh.StartScheduler(cmd.Context())

	// Start the site-monitoring scheduler (replaces the standalone jman-monitor daemon)
	if !config.Cfg.MonitorDisabled {
		if err := monitor.StartScheduler(cmd.Context()); err != nil {
			return err
		}
	}

	log.Printf("Starting jman-api (version: %s) on :%s", config.AppVersion, port)
	return http.ListenAndServe(":"+port, handler)
}

func main() {
	os.Exit(run())
}

// run executes the root command and returns the process exit code. Unlike
// calling os.Exit directly from main, this lets deferred cleanup (closing
// the database) run before the process exits on an error path.
func run() int {
	defer db.Close()
	if err := rootCmd.Execute(); err != nil {
		return 1
	}
	return 0
}
