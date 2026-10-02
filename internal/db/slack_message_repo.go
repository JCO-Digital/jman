package db

import "fmt"

// HasSlackMessage reports whether a message with this content hash has
// already been sent.
func HasSlackMessage(hash string) (bool, error) {
	db := GetAPIDB()
	if db == nil {
		return false, fmt.Errorf("database not initialized")
	}
	var exists bool
	err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM slack_messages WHERE hash = ?)`, hash).Scan(&exists)
	return exists, err
}

// RecordSlackMessage remembers a sent message's content hash; recording
// the same hash again is a no-op.
func RecordSlackMessage(hash, channel string) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := db.Exec(`INSERT INTO slack_messages (hash, channel) VALUES (?, ?) ON CONFLICT (hash) DO NOTHING`, hash, channel)
	return err
}
