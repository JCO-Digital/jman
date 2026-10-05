package db

import "testing"

func TestVulnAlertRoundTrip(t *testing.T) {
	setupTestAPIDB(t)

	got, err := GetAlertedVulnUUIDs()
	if err != nil || len(got) != 0 {
		t.Fatalf("empty table: got %v, err %v", got, err)
	}

	if err := RecordVulnAlert("uuid-1", "akismet", 8.1); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := RecordVulnAlert("uuid-1", "akismet", 9.0); err != nil {
		t.Fatalf("re-record should be a no-op, got: %v", err)
	}
	if err := RecordVulnAlert("uuid-2", "jetpack", 7.0); err != nil {
		t.Fatalf("record: %v", err)
	}

	got, err = GetAlertedVulnUUIDs()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got) != 2 || !got["uuid-1"] || !got["uuid-2"] {
		t.Errorf("unexpected alerted set %v", got)
	}
}
