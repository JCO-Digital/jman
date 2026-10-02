package db

import "testing"

// TestMigrateTable_StableSchemaIsNotRebuilt guards against migrateTable
// treating an unchanged table as changed. A rebuild copies the rows into a
// fresh table, which resets its AUTOINCREMENT sequence to the current max id,
// so a deleted row's id would be handed out again.
func TestMigrateTable_StableSchemaIsNotRebuilt(t *testing.T) {
	setupTaskRepoTest(t)
	api := GetAPIDB().DB

	for _, title := range []string{"one", "two", "three"} {
		if _, err := api.Exec(`INSERT INTO tasks (type, title) VALUES ('one-time', ?)`, title); err != nil {
			t.Fatalf("failed to insert task: %v", err)
		}
	}
	if _, err := api.Exec(`DELETE FROM tasks WHERE title = 'three'`); err != nil {
		t.Fatalf("failed to delete task: %v", err)
	}
	seqBefore := queryInt(t, api, `SELECT seq FROM sqlite_sequence WHERE name = 'tasks'`)

	// Simulate a restart: re-run the schema setup against the same files.
	Close()
	if err := InitInventory(); err != nil {
		t.Fatalf("InitInventory failed: %v", err)
	}
	if err := InitAPI(); err != nil {
		t.Fatalf("InitAPI failed: %v", err)
	}
	api = GetAPIDB().DB

	if got := queryInt(t, api, `SELECT seq FROM sqlite_sequence WHERE name = 'tasks'`); got != seqBefore {
		t.Fatalf("tasks sequence changed across restart (%d -> %d): table was rebuilt", seqBefore, got)
	}
	if _, err := api.Exec(`INSERT INTO tasks (type, title) VALUES ('one-time', 'four')`); err != nil {
		t.Fatalf("failed to insert task: %v", err)
	}
	if got := queryInt(t, api, `SELECT id FROM tasks WHERE title = 'four'`); got <= seqBefore {
		t.Fatalf("new task reused id %d (sequence was %d)", got, seqBefore)
	}
}
