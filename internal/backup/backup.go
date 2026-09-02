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

// PerformBackup creates a snapshot of each database (inventory.db and
// api.db) using VACUUM INTO. It also manages a "latest" symlink per
// database and cleans up files older than 48 hours. A failure backing up
// one database doesn't prevent the other from being attempted.
func PerformBackup() error {
	if err := os.MkdirAll(config.RunData.BackupDir, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	errInventory := backupOne("inventory", db.BackupInventory)
	errAPI := backupOne("api", db.BackupAPI)

	if errInventory != nil {
		return errInventory
	}
	return errAPI
}

// backupOne performs a VACUUM INTO backup for a single database, identified
// by prefix (e.g. "inventory" or "api"), and refreshes its "latest" symlink.
func backupOne(prefix string, backupFn func(destPath string) error) error {
	start := time.Now()

	timestamp := start.Format("20060102-150405")
	backupFileName := fmt.Sprintf("%s-%s.db", prefix, timestamp)
	backupPath := filepath.Join(config.RunData.BackupDir, backupFileName)

	if err := backupFn(backupPath); err != nil {
		return fmt.Errorf("%s database backup failed: %w", prefix, err)
	}

	// Update the '<prefix>-latest.db' symlink
	latestPath := filepath.Join(config.RunData.BackupDir, prefix+"-latest.db")

	// Remove existing symlink or file if it exists
	_ = os.Remove(latestPath)

	// Create new symlink pointing to the new backup file.
	// We use a relative path for the target so the directory remains portable.
	if err := os.Symlink(backupFileName, latestPath); err != nil {
		log.Printf("Warning: failed to update '%s-latest.db' symlink: %v", prefix, err)
	}

	log.Printf("%s database backup successful: %s (took %v)", prefix, backupFileName, time.Since(start))

	return cleanupOldBackups(prefix)
}

// cleanupOldBackups removes backup files for the given database prefix
// older than 48 hours.
func cleanupOldBackups(prefix string) error {
	files, err := os.ReadDir(config.RunData.BackupDir)
	if err != nil {
		return fmt.Errorf("failed to read backup directory for cleanup: %w", err)
	}

	now := time.Now()
	retentionPeriod := 48 * time.Hour
	deletedCount := 0
	fileNamePrefix := prefix + "-"

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		name := file.Name()
		// Only touch files that match our backup pattern <prefix>-YYYYMMDD-HHMMSS.db
		if !strings.HasPrefix(name, fileNamePrefix) || !strings.HasSuffix(name, ".db") || name == prefix+"-latest.db" {
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
		log.Printf("Cleaned up %d old %s backup(s)", deletedCount, prefix)
	}

	return nil
}
