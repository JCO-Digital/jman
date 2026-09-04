package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"strings"
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
	if err := processReminders(); err != nil {
		log.Printf("Error processing task reminders: %v", err)
	}
	if err := cleanupOrphanedTasks(); err != nil {
		log.Printf("Error cleaning up orphaned tasks: %v", err)
	}
	if err := pruneSiteTraffic(); err != nil {
		log.Printf("Error pruning old site traffic: %v", err)
	}
	if err := checkStaleAgents(); err != nil {
		log.Printf("Error checking for stale agents: %v", err)
	}
}

// siteTrafficHourlyRetention is how long site_traffic_hourly rows are kept
// before being pruned in favor of their already-computed site_traffic_daily
// rollup — hourly detail beyond this window has no consumer (jman-ui only
// ever requests a week of hourly data) but was otherwise accumulating
// forever. Must mirror hourlyRetentionWindow in
// internal/agent/logs/collect.go.
const siteTrafficHourlyRetention = 168 * time.Hour

func pruneSiteTraffic() error {
	// Finalize each completed day's site_traffic_daily rollup before pruning
	// touches any of its source hourly rows — see FinalizeCompletedDailyRollups
	// and PruneOldSiteTrafficHourly's doc comments for why the order matters.
	if err := db.FinalizeCompletedDailyRollups(); err != nil {
		return fmt.Errorf("failed to finalize completed daily rollups: %w", err)
	}
	return db.PruneOldSiteTrafficHourly(time.Now().Add(-siteTrafficHourlyRetention))
}

// SyncVulnerabilities checks for vulnerabilities and updates or creates tasks
// accordingly. Called by internal/refresh's slow tick right after it fetches
// fresh vulnerability data, rather than on this package's own hourly ticker —
// keeping it to a single call site avoids two independent tickers racing the
// same check-then-insert task creation for a site.
func SyncVulnerabilities() error {
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

	// Look up the configured default assignee once for this run — applied
	// only to newly-created tasks, not to updates of existing open tasks,
	// so a manual reassignment/unassignment is never overwritten.
	defaultAssignee := ""
	if setting, err := db.GetSetting(db.SystemSettingsUserID, db.DefaultVulnAssigneeSettingKey); err != nil {
		log.Printf("Warning: failed to load default vuln assignee setting: %v", err)
	} else if setting != nil {
		if v, ok := setting.Value.(string); ok {
			defaultAssignee = v
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
			pluginName := html.UnescapeString(link.Report.PluginName)
			if _, ok := pluginMap[link.Report.Slug]; !ok {
				score := cvss
				pluginMap[link.Report.Slug] = &models.VulnPlugin{
					PluginName: pluginName,
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
			before := *task
			task.Priority = priority
			task.Description = &description
			task.Metadata = &metadataStr
			if err := db.SaveTask(task, "system"); err != nil {
				log.Printf("Error updating vuln task for site %d: %v", siteID, err)
			} else {
				NotifyTaskChange(&before, task, "system")
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
			if defaultAssignee != "" {
				assignee := defaultAssignee
				newTask.AssignedTo = &assignee
			}
			if err := db.SaveTask(newTask, "system"); err != nil {
				log.Printf("Error creating vuln task for site %d: %v", siteID, err)
			} else {
				NotifyTaskChange(nil, newTask, "system")
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

// NotifyTaskChange notifies current's assignee about a task being created,
// (re)assigned to them, or otherwise edited — unless they are the one who
// made the change themselves (actor). old is nil for a newly-created task;
// actor is the username performing the change ("system" for scheduler-driven
// changes, which never matches a real assignee, so those always notify).
//
// A create or (re)assignment sends the full task; any other change sends
// only what changed (title/type/priority are always included for context).
func NotifyTaskChange(old, current *models.Task, actor string) {
	if current.AssignedTo == nil || *current.AssignedTo == "" {
		return
	}
	if actor != "" && actor == *current.AssignedTo {
		return
	}

	wasAlreadyAssignedToThem := old != nil && old.AssignedTo != nil && *old.AssignedTo == *current.AssignedTo

	var message string
	if !wasAlreadyAssignedToThem {
		message = formatFullTaskMessage(current, old == nil)
	} else {
		changes := diffTaskFields(old, current)
		if len(changes) == 0 {
			return
		}
		message = formatTaskChangeMessage(current, changes)
	}

	sendToAssignee(current, message)
}

// formatFullTaskMessage renders the complete task, used when a task is
// created or (re)assigned to someone.
func formatFullTaskMessage(task *models.Task, created bool) string {
	header := "📋 *Task Assigned to You*"
	if created {
		header = "🆕 *New Task Assigned to You*"
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s: %s\n", header, task.Title)
	fmt.Fprintf(&sb, "Type: %s | Priority: %s | Status: %s\n", task.Type, task.Priority, task.Status)
	if task.Description != nil && *task.Description != "" {
		fmt.Fprintf(&sb, "Description: %s\n", *task.Description)
	}
	if task.DueDate != nil {
		fmt.Fprintf(&sb, "Due: %s\n", task.DueDate.Format("2006-01-02"))
	}
	if task.ReminderDate != nil {
		fmt.Fprintf(&sb, "Reminder: %s\n", task.ReminderDate.Format("2006-01-02"))
	}
	if task.Interval != nil && *task.Interval != "" {
		fmt.Fprintf(&sb, "Repeats: every %s\n", *task.Interval)
	}
	return strings.TrimRight(sb.String(), "\n")
}

// formatTaskChangeMessage renders only what changed, with title/type/priority
// always shown for context.
func formatTaskChangeMessage(task *models.Task, changes []taskFieldChange) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "📝 *Task Updated: %s*\n", task.Title)
	fmt.Fprintf(&sb, "Type: %s | Priority: %s\n", task.Type, task.Priority)
	sb.WriteString("Changes:\n")
	for _, c := range changes {
		oldVal, newVal := c.Old, c.New
		if oldVal == "" {
			oldVal = "—"
		}
		if newVal == "" {
			newVal = "—"
		}
		fmt.Fprintf(&sb, "• %s: %s → %s\n", c.Label, oldVal, newVal)
	}
	return strings.TrimRight(sb.String(), "\n")
}

// taskFieldChange is one changed field, formatted for display.
type taskFieldChange struct {
	Label    string
	Old, New string
}

// diffTaskFields compares the user-editable fields of old and current
// (excluding AssignedTo, handled separately by NotifyTaskChange) and returns
// only those that differ.
func diffTaskFields(old, current *models.Task) []taskFieldChange {
	var changes []taskFieldChange
	add := func(label, oldVal, newVal string) {
		if oldVal != newVal {
			changes = append(changes, taskFieldChange{label, oldVal, newVal})
		}
	}

	add("Title", old.Title, current.Title)
	add("Type", string(old.Type), string(current.Type))
	add("Priority", string(old.Priority), string(current.Priority))
	add("Status", string(old.Status), string(current.Status))
	add("Description", derefStr(old.Description), derefStr(current.Description))
	add("Due Date", formatDatePtr(old.DueDate), formatDatePtr(current.DueDate))
	add("Reminder Date", formatDatePtr(old.ReminderDate), formatDatePtr(current.ReminderDate))
	add("Interval", derefStr(old.Interval), derefStr(current.Interval))

	return changes
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func formatDatePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02")
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
