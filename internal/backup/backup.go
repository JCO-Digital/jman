package backup

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/slack"
)

// StartScheduler starts a background goroutine that performs periodic backups.
// It performs an initial backup immediately and then every hour.
func StartScheduler(ctx context.Context) {
	go func() {
		log.Println("Starting database backup scheduler...")

		// Initial backup on startup
		if err := PerformBackup(); err != nil {
			msg := fmt.Sprintf("🚨 Initial database backup failed: %v", err)
			log.Println(msg)
			_ = slack.SendMessage(msg, true)
		}

		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := PerformBackup(); err != nil {
					msg := fmt.Sprintf("🚨 Periodic database backup failed: %v", err)
					log.Println(msg)
					_ = slack.SendMessage(msg, true)
				}
			case <-ctx.Done():
				log.Println("Database backup scheduler stopped.")
				return
			}
		}
	}()
}

// PerformBackup creates a snapshot of the current database using VACUUM INTO.
// It also manages a symlink to the latest backup and cleans up files older than 48 hours.
func PerformBackup() error {
	start := time.Now()
	// Ensure backup directory exists
	if err := os.MkdirAll(config.RunData.BackupDir, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	timestamp := start.Format("20060102-150405")
	backupFileName := fmt.Sprintf("jman-%s.db", timestamp)
	backupPath := filepath.Join(config.RunData.BackupDir, backupFileName)

	// Perform the backup using the database package
	if err := db.Backup(backupPath); err != nil {
		return err
	}

	// Update the 'latest.db' symlink
	latestPath := filepath.Join(config.RunData.BackupDir, "latest.db")

	// Remove existing symlink or file if it exists
	_ = os.Remove(latestPath)

	// Create new symlink pointing to the new backup file.
	// We use a relative path for the target so the directory remains portable.
	if err := os.Symlink(backupFileName, latestPath); err != nil {
		log.Printf("Warning: failed to update 'latest.db' symlink: %v", err)
	}

	log.Printf("Database backup successful: %s (took %v)", backupFileName, time.Since(start))

	return cleanupOldBackups()
}

// cleanupOldBackups removes backup files older than 48 hours.
func cleanupOldBackups() error {
	files, err := os.ReadDir(config.RunData.BackupDir)
	if err != nil {
		return fmt.Errorf("failed to read backup directory for cleanup: %w", err)
	}

	now := time.Now()
	retentionPeriod := 48 * time.Hour
	deletedCount := 0

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		name := file.Name()
		// Only touch files that match our backup pattern jman-YYYYMMDD-HHMMSS.db
		if !strings.HasPrefix(name, "jman-") || !strings.HasSuffix(name, ".db") || name == "latest.db" {
			continue
		}

		info, err := file.Info()
		if err != nil {
			continue
		}

		if now.Sub(info.ModTime()) > retentionPeriod {
			path := filepath.Join(config.RunData.BackupDir, name)
			if err := os.Remove(path); err != nil {
				log.Printf("Warning: failed to remove old backup %s: %v", name, err)
			} else {
				deletedCount++
			}
		}
	}

	if deletedCount > 0 {
		log.Printf("Cleaned up %d old backup(s)", deletedCount)
	}

	return nil
}
