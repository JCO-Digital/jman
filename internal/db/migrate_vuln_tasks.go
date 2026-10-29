package db

import (
	"database/sql"
	"time"

	"github.com/JCO-Digital/jman/internal/verb"
)

// vulnTasksMigrationName is the api.db schema_migrations key recording that
// the open auto-generated vulnerability tasks were skipped when the
// vulnerability sync stopped creating them.
const vulnTasksMigrationName = "skip_auto_vuln_tasks"

// SkipAutoVulnTasks marks every open task the old vulnerability sync created
// ("Security Vulnerabilities - <site>", created by system, vuln_uuids in its
// metadata) as skipped. Nothing creates or updates them any more, so they
// would otherwise stay open. It writes the rows directly, so no assignee
// gets a Slack notification per task. Manually created tasks are left alone.
func SkipAutoVulnTasks(dbConn *sql.DB) error {
	return runOnce(dbConn, vulnTasksMigrationName, func(tx *sql.Tx) error {
		res, err := tx.Exec(`UPDATE tasks SET status = 'skipped', updated_at = ?
			WHERE created_by = 'system'
			AND title LIKE 'Security Vulnerabilities%'
			AND metadata LIKE '%"vuln_uuids"%'
			AND status NOT IN ('completed', 'skipped')`, time.Now())
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		verb.LogPrintf(verb.Normal, "Skipped %d open auto-generated vulnerability task(s)", n)
		return nil
	})
}
