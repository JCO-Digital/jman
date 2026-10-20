package db

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/JCO-Digital/jman/internal/models"
)

// updateJobPayloadMigrationName is the api.db schema_migrations key
// recording that the update jobs' plugins, target, source, activate and
// allow_major columns were folded into payload.
const updateJobPayloadMigrationName = "update_job_payload"

// MigrateUpdateJobPayloads moves each existing update job's kind-specific
// columns into the JSON payload column. It must run before migrateTable
// rebuilds plugin_update_jobs, which drops the old columns. A database
// without the table, or whose table has no plugins column, has nothing to
// convert.
func MigrateUpdateJobPayloads(dbConn *sql.DB) error {
	return runOnce(dbConn, updateJobPayloadMigrationName, func(tx *sql.Tx) error {
		exists, err := tableExists(tx, "plugin_update_jobs")
		if err != nil || !exists {
			return err
		}
		hasPlugins, err := columnExists(tx, "plugin_update_jobs", "plugins")
		if err != nil || !hasPlugins {
			return err
		}
		hasPayload, err := columnExists(tx, "plugin_update_jobs", "payload")
		if err != nil {
			return err
		}
		if !hasPayload {
			if _, err := tx.Exec(`ALTER TABLE plugin_update_jobs ADD COLUMN payload TEXT NOT NULL DEFAULT '{}'`); err != nil {
				return err
			}
		}

		type oldJob struct {
			id         int64
			kind       string
			plugins    string
			target     sql.NullString
			source     sql.NullString
			activate   bool
			allowMajor bool
		}
		rows, err := tx.Query(`SELECT id, kind, plugins, target, source, activate, allow_major FROM plugin_update_jobs`)
		if err != nil {
			return err
		}
		var jobs []oldJob
		for rows.Next() {
			var j oldJob
			if err := rows.Scan(&j.id, &j.kind, &j.plugins, &j.target, &j.source, &j.activate, &j.allowMajor); err != nil {
				rows.Close()
				return err
			}
			jobs = append(jobs, j)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		for _, j := range jobs {
			var payload any
			switch j.kind {
			case models.UpdateJobKindCore:
				payload = models.CorePayload{Target: j.target.String, AllowMajor: j.allowMajor}
			case models.UpdateJobKindInstall:
				payload = models.InstallPayload{Source: j.source.String, Activate: j.activate}
			default:
				p := models.PluginsPayload{Plugins: []models.PluginUpdateRequest{}, AllowMajor: j.allowMajor}
				if j.plugins != "" {
					if err := json.Unmarshal([]byte(j.plugins), &p.Plugins); err != nil {
						return fmt.Errorf("invalid plugins in job %d: %w", j.id, err)
					}
				}
				payload = p
			}
			b, err := json.Marshal(payload)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(`UPDATE plugin_update_jobs SET payload = ? WHERE id = ?`, string(b), j.id); err != nil {
				return err
			}
		}
		return nil
	})
}

func columnExists(tx *sql.Tx, table, column string) (bool, error) {
	var count int
	err := tx.QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&count)
	return count > 0, err
}
