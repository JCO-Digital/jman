package db

import (
	"testing"

	"github.com/JCO-Digital/jman/internal/models"
)

func TestMigrateUpdateJobPayloads(t *testing.T) {
	setupTestDBDir(t)
	if err := InitAPI(); err != nil {
		t.Fatal(err)
	}
	api := GetAPIDB().DB

	// The jobs table as it was before payload.
	for _, q := range []string{
		`DROP TABLE plugin_update_jobs`,
		`CREATE TABLE plugin_update_jobs (id INTEGER PRIMARY KEY AUTOINCREMENT, kind TEXT NOT NULL DEFAULT 'plugins',
			site_id TEXT NOT NULL, target TEXT, core TEXT, source TEXT, activate INTEGER NOT NULL DEFAULT 0, upload_path TEXT,
			allow_major INTEGER NOT NULL DEFAULT 0, status TEXT NOT NULL, plugins TEXT NOT NULL, results TEXT, error TEXT,
			created_by TEXT, created_at DATETIME NOT NULL, started_at DATETIME, finished_at DATETIME)`,
		`INSERT INTO plugin_update_jobs (kind, site_id, status, plugins, allow_major, created_at) VALUES
			('plugins', 's', 'done', '[{"name":"akismet","old_version":"5.0"}]', 1, '2026-10-01 00:00:00')`,
		`INSERT INTO plugin_update_jobs (kind, site_id, status, plugins, target, created_at) VALUES
			('core', 's', 'done', '[]', 'major', '2026-10-01 00:00:00')`,
		`INSERT INTO plugin_update_jobs (kind, site_id, status, plugins, source, activate, upload_path, created_at) VALUES
			('install', 's', 'queued', '[]', 'my-plugin.zip', 1, '/tmp/x.zip', '2026-10-01 00:00:00')`,
		`INSERT INTO plugin_update_jobs (kind, site_id, status, plugins, created_at) VALUES
			('deactivate', 's', 'done', '[{"name":"hello","old_version":""}]', '2026-10-01 00:00:00')`,
		`DELETE FROM schema_migrations WHERE name = 'update_job_payload'`,
	} {
		if _, err := api.Exec(q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}

	if err := initAPISchema(); err != nil {
		t.Fatal(err)
	}

	var hasOld int
	if err := api.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('plugin_update_jobs') WHERE name IN ('plugins', 'target', 'source', 'activate', 'allow_major')`).Scan(&hasOld); err != nil {
		t.Fatal(err)
	}
	if hasOld != 0 {
		t.Errorf("%d old columns are left", hasOld)
	}

	jobs := map[int64]models.UpdateJob{}
	for id := int64(1); id <= 4; id++ {
		job, err := GetUpdateJob(id)
		if err != nil || job == nil {
			t.Fatalf("job %d: %v", id, err)
		}
		jobs[id] = *job
	}

	plugins, err := models.DecodePayload[models.PluginsPayload](jobs[1])
	if err != nil || len(plugins.Plugins) != 1 || plugins.Plugins[0].Name != "akismet" || plugins.Plugins[0].OldVersion != "5.0" || !plugins.AllowMajor {
		t.Errorf("plugins payload = %+v, %v", plugins, err)
	}
	core, err := models.DecodePayload[models.CorePayload](jobs[2])
	if err != nil || core.Target != "major" || core.AllowMajor {
		t.Errorf("core payload = %+v, %v", core, err)
	}
	install, err := models.DecodePayload[models.InstallPayload](jobs[3])
	if err != nil || install.Source != "my-plugin.zip" || !install.Activate {
		t.Errorf("install payload = %+v, %v", install, err)
	}
	if jobs[3].UploadPath != "/tmp/x.zip" {
		t.Errorf("upload path = %q, want it kept", jobs[3].UploadPath)
	}
	action, err := models.DecodePayload[models.PluginsPayload](jobs[4])
	if err != nil || len(action.Plugins) != 1 || action.Plugins[0].Name != "hello" {
		t.Errorf("deactivate payload = %+v, %v", action, err)
	}
}
