package tasks

import (
	"fmt"
	"log"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/slack"
)

// Task notifications go to a task's assignee and its watchers, never to the
// person who made the change. They're grouped per task and recipient: a
// message goes out once the task has been left alone for notifyQuiet, or
// notifyMaxWait after the first change at the latest, so editing a task one
// field at a time sends one message rather than one per field.
var (
	notifyQuiet   = time.Minute
	notifyMaxWait = 5 * time.Minute
	// sendDM sends a Slack DM to a jman user; tests replace it.
	sendDM = sendDMToUser
)

type notifyKey struct {
	taskID   int
	username string
}

type noteEvent struct {
	author, content string
}

// pendingNotice is what has happened to one task since its recipient was
// last told.
type pendingNotice struct {
	first   time.Time
	timer   *time.Timer
	full    bool // created, or (re)assigned to the recipient
	created bool
	changes []taskFieldChange
	notes   []noteEvent
}

var notifier = struct {
	mu      sync.Mutex
	pending map[notifyKey]*pendingNotice
}{pending: map[notifyKey]*pendingNotice{}}

// NotifyTaskChange queues a notice of a task being created (old is nil),
// (re)assigned or otherwise edited by actor, for current's assignee and
// watchers. A create or (re)assignment tells the new assignee the whole
// task; everyone else gets only what changed.
func NotifyTaskChange(old, current *models.Task, actor string) {
	changes := []taskFieldChange{}
	if old != nil {
		changes = diffTaskFields(old, current)
	}
	for _, user := range recipients(current, actor) {
		isAssignee := current.AssignedTo != nil && *current.AssignedTo == user
		wasAssignee := old != nil && old.AssignedTo != nil && *old.AssignedTo == user
		if isAssignee && !wasAssignee {
			queueNotice(current.ID, user, func(p *pendingNotice) {
				p.full = true
				p.created = p.created || old == nil
			})
			continue
		}
		if len(changes) == 0 {
			continue
		}
		queueNotice(current.ID, user, func(p *pendingNotice) {
			p.changes = mergeChanges(p.changes, changes)
		})
	}
}

// NotifyTaskNote queues a notice of a note author wrote on task.
func NotifyTaskNote(task *models.Task, author, content string) {
	for _, user := range recipients(task, author) {
		queueNotice(task.ID, user, func(p *pendingNotice) {
			p.notes = append(p.notes, noteEvent{author, content})
		})
	}
}

// recipients is a task's assignee and watchers, without actor.
func recipients(task *models.Task, actor string) []string {
	var users []string
	if task.AssignedTo != nil && *task.AssignedTo != "" {
		users = append(users, *task.AssignedTo)
	}
	watchers, err := db.GetTaskWatchers(task.ID)
	if err != nil {
		log.Printf("task notifications: watchers of task %d: %v", task.ID, err)
	}
	for _, w := range watchers {
		if !slices.Contains(users, w) {
			users = append(users, w)
		}
	}
	return slices.DeleteFunc(users, func(u string) bool { return u == actor })
}

func queueNotice(taskID int, user string, apply func(*pendingNotice)) {
	key := notifyKey{taskID, user}
	notifier.mu.Lock()
	defer notifier.mu.Unlock()

	p := notifier.pending[key]
	now := time.Now()
	if p == nil {
		p = &pendingNotice{first: now}
		notifier.pending[key] = p
	}
	apply(p)

	delay := min(notifyQuiet, p.first.Add(notifyMaxWait).Sub(now))
	if p.timer != nil {
		p.timer.Stop()
	}
	p.timer = time.AfterFunc(max(delay, 0), func() { flushNotice(key) })
}

// FlushTaskNotifications sends every queued notice now, for shutdown.
func FlushTaskNotifications() {
	notifier.mu.Lock()
	keys := make([]notifyKey, 0, len(notifier.pending))
	for k, p := range notifier.pending {
		p.timer.Stop()
		keys = append(keys, k)
	}
	notifier.mu.Unlock()
	for _, k := range keys {
		flushNotice(k)
	}
}

func flushNotice(key notifyKey) {
	notifier.mu.Lock()
	p := notifier.pending[key]
	delete(notifier.pending, key)
	notifier.mu.Unlock()
	if p == nil {
		return
	}

	task, err := db.GetTask(key.taskID)
	if err != nil {
		log.Printf("task notifications: task %d: %v", key.taskID, err)
		return
	}
	if task == nil {
		return // deleted in the meantime
	}
	// Someone who stopped watching, or was unassigned, in the meantime
	// isn't told.
	if task.AssignedTo == nil || *task.AssignedTo != key.username {
		watchers, err := db.GetTaskWatchers(task.ID)
		if err != nil || !slices.Contains(watchers, key.username) {
			return
		}
	}

	message := formatNotice(task, p)
	if message == "" {
		return
	}
	if link := taskLink(task); link != "" {
		message += "\n" + link
	}
	sendDM(key.username, message)
}

// formatNotice renders everything in p as one message.
func formatNotice(task *models.Task, p *pendingNotice) string {
	var parts []string
	switch {
	case p.full:
		parts = append(parts, formatFullTaskMessage(task, p.created))
	case len(p.changes) > 0:
		parts = append(parts, formatTaskChangeMessage(task, p.changes))
	}
	if len(p.notes) > 0 {
		var sb strings.Builder
		if len(parts) == 0 {
			fmt.Fprintf(&sb, "💬 *New note on task: %s*\n", task.Title)
		} else {
			sb.WriteString("Notes:\n")
		}
		for _, n := range p.notes {
			fmt.Fprintf(&sb, "*%s*: %s\n", n.author, n.content)
		}
		parts = append(parts, strings.TrimRight(sb.String(), "\n"))
	}
	return strings.Join(parts, "\n")
}

// mergeChanges folds next into prev: a field changed twice shows its first
// old and last new value, and one changed back drops out.
func mergeChanges(prev, next []taskFieldChange) []taskFieldChange {
	out := slices.Clone(prev)
	for _, c := range next {
		i := slices.IndexFunc(out, func(o taskFieldChange) bool { return o.Label == c.Label })
		if i == -1 {
			out = append(out, c)
			continue
		}
		out[i].New = c.New
	}
	return slices.DeleteFunc(out, func(c taskFieldChange) bool { return c.Old == c.New })
}

// sendDMToUser sends message to username's Slack ID setting, if they have one.
func sendDMToUser(username, message string) {
	setting, err := db.GetSetting(username, "slack_id")
	if err != nil || setting == nil {
		return
	}
	slackID, ok := setting.Value.(string)
	if !ok || slackID == "" {
		return
	}
	if err := slack.SendMessageToChannel(message, slackID, false); err != nil {
		log.Printf("task notifications: DM to %s: %v", username, err)
	}
}
