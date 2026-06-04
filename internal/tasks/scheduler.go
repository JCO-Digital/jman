package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/slack"
	"github.com/JCO-Digital/jman/internal/vuln"
)

// StartScheduler starts the background routine for tasks in the API process.
func StartScheduler(ctx context.Context) {
	go func() {
		log.Println("Starting Task scheduler...")

		// Run initially after a short delay to let the system settle
		time.Sleep(10 * time.Second)
		runTick()

		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				runTick()
			case <-ctx.Done():
				log.Println("Task scheduler stopped.")
				return
			}
		}
	}()
}

func runTick() {
	log.Println("Running task scheduler tick...")
	if err := syncVulnerabilities(); err != nil {
		log.Printf("Error syncing vulnerabilities to tasks: %v", err)
	}
	if err := processReminders(); err != nil {
		log.Printf("Error processing task reminders: %v", err)
	}
	if err := cleanupOrphanedTasks(); err != nil {
		log.Printf("Error cleaning up orphaned tasks: %v", err)
	}
}

// syncVulnerabilities checks for vulnerabilities and updates or creates tasks accordingly.
func syncVulnerabilities() error {
	matcher, err := db.NewVulnIgnoreMatcher()
	if err != nil {
		log.Printf("Warning: failed to load ignore entries: %v", err)
	}

	// Build the site-centric vulnerability map using existing logic
	reports, err := vuln.ProcessVulnerabilities(matcher)
	if err != nil {
		return err
	}

	// Group vulnerabilities by site
	type siteVulnLink struct {
		Report models.VulnReport
		Vuln   models.Vulnerability
	}
	siteVulns := make(map[int][]siteVulnLink)
	for _, report := range reports {
		if report.Suppressed {
			continue
		}
		for _, v := range report.Vulnerabilities {
			if v.Suppressed {
				continue
			}
			for _, site := range v.Sites {
				if site.Suppressed {
					continue
				}
				siteVulns[site.SiteID] = append(siteVulns[site.SiteID], siteVulnLink{report, v})
			}
		}
	}

	// Load site names for titles
	sites, _ := cache.GetFastSiteList()
	siteNameMap := make(map[int]string)
	for _, s := range sites {
		siteNameMap[s.ID] = s.Name
	}

	for siteID, links := range siteVulns {
		var maxCvss float64
		var totalVulns int
		var uuids []string
		pluginMap := make(map[string]*models.VulnPlugin)

		for _, link := range links {
			totalVulns++
			uuids = append(uuids, link.Vuln.Uuid)

			// Find CVSS
			var cvss float64
			if link.Vuln.Impact != nil && link.Vuln.Impact.Cvss != nil {
				fmt.Sscanf(link.Vuln.Impact.Cvss.Score, "%f", &cvss)
			}
			if cvss > maxCvss {
				maxCvss = cvss
			}

			siteVersion := ""
			for _, site := range link.Vuln.Sites {
				if site.SiteID == siteID {
					siteVersion = site.Version
					break
				}
			}

			// Group for description
			if _, ok := pluginMap[link.Report.Slug]; !ok {
				score := cvss
				pluginMap[link.Report.Slug] = &models.VulnPlugin{
					PluginName: link.Report.PluginName,
					Version:    siteVersion,
					Cvss:       &score,
				}
			} else {
				if cvss > *pluginMap[link.Report.Slug].Cvss {
					score := cvss
					pluginMap[link.Report.Slug].Cvss = &score
				}
			}
		}

		// Apply thresholds
		if maxCvss < config.Cfg.CVSSThreshold && float64(totalVulns) < config.Cfg.VulnThreshold {
			continue
		}

		// Check for existing open task
		task, err := db.GetOpenVulnerabilityTask(siteID)
		if err != nil {
			log.Printf("Error looking for existing task: %v", err)
			continue
		}

		// Determine priority
		priority := models.TaskPriorityLow
		if maxCvss >= 7.0 {
			priority = models.TaskPriorityHigh
		} else if maxCvss >= 4.0 {
			priority = models.TaskPriorityMedium
		}

		// Prepare description
		siteName := siteNameMap[siteID]
		if siteName == "" {
			siteName = fmt.Sprintf("Site #%d", siteID)
		}

		description := "Automated vulnerability report.\n\n"
		for _, p := range pluginMap {
			description += fmt.Sprintf("- %s (%s) Max CVSS: %.1f\n", p.PluginName, p.Version, *p.Cvss)
		}

		metadataJSON, _ := json.Marshal(map[string]interface{}{"vuln_uuids": uuids})
		metadataStr := string(metadataJSON)

		if task != nil {
			// Update existing task
			task.Priority = priority
			task.Description = &description
			task.Metadata = &metadataStr
			if err := db.SaveTask(task, "system"); err != nil {
				log.Printf("Error updating vuln task for site %d: %v", siteID, err)
			}
		} else {
			// Create new task
			now := time.Now()
			due := now.Add(7 * 24 * time.Hour)

			newTask := &models.Task{
				Type:         models.TaskTypeOneTime,
				Status:       models.TaskStatusPending,
				Priority:     priority,
				Title:        fmt.Sprintf("Security Vulnerabilities - %s", siteName),
				Description:  &description,
				SiteID:       &siteID,
				Metadata:     &metadataStr,
				ReminderDate: &now,
				DueDate:      &due,
			}
			if err := db.SaveTask(newTask, "system"); err != nil {
				log.Printf("Error creating vuln task for site %d: %v", siteID, err)
			}
		}
	}

	return nil
}

// processReminders sends Slack notifications for due tasks.
func processReminders() error {
	tasks, err := db.GetTasks(db.TaskFilter{Status: models.TaskStatusPending})
	if err != nil {
		return err
	}

	now := time.Now()
	// Cache for user reminder times to avoid N+1 queries
	userReminderTimes := make(map[string]time.Time)

	for _, task := range tasks {
		if (task.Priority == models.TaskPriorityHigh || task.Priority == models.TaskPriorityMedium) &&
			task.ReminderDate != nil && task.ReminderDate.Before(now) && task.LastNotifiedAt == nil {

			// Get user's preferred reminder time
			assignee := "default"
			if task.AssignedTo != nil && *task.AssignedTo != "" {
				assignee = *task.AssignedTo
			}

			reminderTime, ok := userReminderTimes[assignee]
			if !ok {
				reminderTimeStr := "10:00"
				if assignee != "default" {
					setting, err := db.GetSetting(assignee, "slack_reminder_time")
					if err == nil && setting != nil {
						if val, ok := setting.Value.(string); ok && val != "" {
							reminderTimeStr = val
						}
					}
				}

				// Parse reminderTimeStr (expecting HH:mm)
				parsed, err := time.ParseInLocation("15:04", reminderTimeStr, now.Location())
				if err != nil {
					// Fallback to default if user setting is invalid
					parsed, _ = time.ParseInLocation("15:04", "10:00", now.Location())
				}
				reminderTime = parsed
				userReminderTimes[assignee] = reminderTime
			}

			todayReminderTime := time.Date(now.Year(), now.Month(), now.Day(), reminderTime.Hour(), reminderTime.Minute(), 0, 0, now.Location())
			if now.Before(todayReminderTime) {
				// It's before the set time today, skip sending for now
				continue
			}

			sendSlackReminder(&task)
		}
	}
	return nil
}

// sendSlackReminder sends a reminder notification for a task.
func sendSlackReminder(task *models.Task) {
	message := fmt.Sprintf("🔔 *Task Reminder: %s*\nPriority: %s", task.Title, task.Priority)
	if task.DueDate != nil {
		message += fmt.Sprintf("\nDue: %s", task.DueDate.Format("2006-01-02"))
	}

	if sendToAssignee(task, message) {
		now := time.Now()
		task.LastNotifiedAt = &now
		_ = db.SaveTask(task, "system")
	}
}

// NotifyTaskAssigned sends a notification to a user when a task is assigned to them.
func NotifyTaskAssigned(task *models.Task) {
	message := fmt.Sprintf("📋 *New Task Assigned: %s*\nPriority: %s", task.Title, task.Priority)
	if task.DueDate != nil {
		message += fmt.Sprintf("\nDue: %s", task.DueDate.Format("2006-01-02"))
	}

	sendToAssignee(task, message)
}

func sendToAssignee(task *models.Task, message string) bool {
	var sent bool
	if task.AssignedTo != nil && *task.AssignedTo != "" {
		setting, err := db.GetSetting(*task.AssignedTo, "slack_id")
		if err == nil && setting != nil {
			if slackID, ok := setting.Value.(string); ok && slackID != "" {
				// Send as a direct message to the user
				if err := slack.SendMessageToChannel(message, slackID, false); err == nil {
					sent = true
				}
			}
		}
	} else if config.Cfg.SlackTasksChannel != "" {
		if err := slack.SendMessageToChannel(message, config.Cfg.SlackTasksChannel, false); err == nil {
			sent = true
		}
	}
	return sent
}

// cleanupOrphanedTasks marks tasks as skipped if they are linked to non-existent entities.
func cleanupOrphanedTasks() error {
	tasks, err := db.GetTasks(db.TaskFilter{})
	if err != nil {
		return err
	}

	sites, err := cache.GetFastSiteList()
	if err != nil {
		return fmt.Errorf("load site cache for orphaned task cleanup: %w", err)
	}
	siteExists := make(map[int]bool)
	for _, s := range sites {
		siteExists[s.ID] = true
	}

	servers, err := cache.GetFastCachedServers()
	if err != nil {
		return fmt.Errorf("load server cache for orphaned task cleanup: %w", err)
	}
	serverExists := make(map[int]bool)
	for _, s := range servers {
		serverExists[s.ID] = true
	}

	for _, task := range tasks {
		if task.Status == models.TaskStatusCompleted || task.Status == models.TaskStatusSkipped {
			continue
		}

		orphaned := false
		if task.SiteID != nil && !siteExists[*task.SiteID] {
			orphaned = true
		} else if task.ServerID != nil && !serverExists[*task.ServerID] {
			orphaned = true
		}

		if orphaned {
			task.Status = models.TaskStatusSkipped
			_ = db.SaveTask(&task, "system")
			log.Printf("Cleaned up orphaned task #%d (%s)", task.ID, task.Title)
		}
	}

	return nil
}
