package slack

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/slack-go/slack"
)

const slackTrackerFile = "slack_messages"

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

	hash := hashMessage(message)

	// Load tracker
	var tracker map[string]bool
	err := cache.ReadJSONData(slackTrackerFile, &tracker)
	if err != nil || tracker == nil {
		tracker = make(map[string]bool)
	}

	// Check if already sent
	if !force && tracker[hash] {
		// Already sent, skip
		return nil
	}

	api := slack.New(config.Cfg.TokenSlack)
	_, _, err = api.PostMessage(
		channel,
		slack.MsgOptionText(message, false),
	)

	if err != nil {
		return fmt.Errorf("failed to send Slack message: %w", err)
	}

	// Record the message as sent
	tracker[hash] = true
	if err := cache.WriteJSONData(slackTrackerFile, tracker); err != nil {
		log.Printf("Warning: failed to write Slack message tracker: %v\n", err)
	}

	return nil
}

func hashMessage(message string) string {
	h := sha256.New()
	h.Write([]byte(message))
	return hex.EncodeToString(h.Sum(nil))
}
