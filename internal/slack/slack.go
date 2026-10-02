package slack

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/utils"
	"github.com/slack-go/slack"
)

const slackTrackerFile = "slack_messages"

var migrationOnce sync.Once

// SendMessage sends a message to the configured Slack channel.
// It tracks sent messages to avoid duplicates. If force is true, it will send even if previously sent.
func SendMessage(message string, force bool) error {
	return SendMessageToChannel(message, config.Cfg.SlackChannel, force)
}

// SendMessageToChannel sends a message to a specific Slack channel.
func SendMessageToChannel(message string, channel string, force bool) error {
	if config.Cfg.TokenSlack == "" {
		return fmt.Errorf("Slack token is not configured")
	}

	if channel == "" {
		channel = "#testing"
	}

	message = utils.StripANSI(message)
	hash := hashMessage(message)
	// The CLI never opens api.db, so it sends without dedup (see internal/vuln/vuln.go).
	tracked := db.GetAPIDB() != nil

	if tracked {
		migrationOnce.Do(migrateSlackTracker)
	}

	if !force && tracked {
		if exists, err := db.HasSlackMessage(hash); err == nil && exists {
			return nil
		}
	}

	api := slack.New(config.Cfg.TokenSlack)
	_, _, err := api.PostMessage(
		channel,
		slack.MsgOptionText(message, false),
	)

	if err != nil {
		return fmt.Errorf("failed to send Slack message: %w", err)
	}

	// Record the message as sent
	if tracked {
		if err := db.RecordSlackMessage(hash, channel); err != nil {
			log.Printf("Warning: failed to record Slack message hash: %v\n", err)
		}
	}

	return nil
}

func migrateSlackTracker() {
	// Only migrate if the file exists
	var tracker map[string]bool
	err := cache.ReadJSONData(slackTrackerFile, &tracker)
	if err != nil {
		return
	}

	log.Printf("Migrating Slack message tracker to database...\n")

	for hash := range tracker {
		if err := db.RecordSlackMessage(hash, "unknown"); err != nil {
			log.Printf("Warning: failed to migrate Slack message hash %s: %v\n", hash, err)
		}
	}

	// Delete the old file after successful migration (or at least attempt)
	oldPath := cache.GetDataFilePath(slackTrackerFile)
	if err := os.Remove(oldPath); err != nil {
		log.Printf("Warning: failed to remove old Slack tracker file: %v\n", err)
	}
}

func hashMessage(message string) string {
	h := sha256.New()
	h.Write([]byte(message))
	return hex.EncodeToString(h.Sum(nil))
}
