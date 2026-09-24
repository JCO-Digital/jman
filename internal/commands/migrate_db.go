package commands

import (
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/spf13/cobra"
)

// MigratedJustNow is set to true by the CLI entrypoint if an automatic migration
// was performed on startup before command execution.
var MigratedJustNow bool

var migrateDbCmd = &cobra.Command{
	Use:   "migrate-db",
	Short: "Split the legacy single jman.db into inventory.db and api.db",
	Long: `One-time migration for existing installs: copies data out of the legacy
jman.db file into the new split databases (inventory.db, shared with the
jman CLI, and api.db, jman-api's own database), verifies row counts match,
then renames jman.db to jman.db.pre-split-backup.

Safe to re-run if interrupted partway through: it skips any split database
that already exists and only renames the legacy file once both are complete.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if MigratedJustNow {
			return nil
		}
		return db.MigrateSplitDB()
	},
}

func init() {
	rootCmd.AddCommand(migrateDbCmd)
}
