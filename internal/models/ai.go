package models

import (
	"encoding/json"
	"time"
)

// AI run statuses. A run is queued, then running, then ends in one of the
// other four.
const (
	AIRunQueued    = "queued"
	AIRunRunning   = "running"
	AIRunSucceeded = "succeeded"
	AIRunFailed    = "failed"
	// AIRunRefused is a run that the model declined (stop_reason
	// "refusal"); RefusalCategory says why.
	AIRunRefused  = "refused"
	AIRunCanceled = "canceled"
)

// AI run triggers.
const (
	AIRunTriggerAPI      = "api"
	AIRunTriggerSchedule = "schedule"
)

// AIRunFinished reports whether status is one a run ends in.
func AIRunFinished(status string) bool {
	switch status {
	case AIRunSucceeded, AIRunFailed, AIRunRefused, AIRunCanceled:
		return true
	}
	return false
}

// AIUsage counts the tokens a run used, summed over its turns.
type AIUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
}

// Add adds o to u.
func (u *AIUsage) Add(o AIUsage) {
	u.InputTokens += o.InputTokens
	u.OutputTokens += o.OutputTokens
	u.CacheReadInputTokens += o.CacheReadInputTokens
	u.CacheCreationInputTokens += o.CacheCreationInputTokens
}

// AIRun is one call of an AI agent: its input, its result and what it cost.
// Agents are advisory: a run never changes anything outside ai_runs.
type AIRun struct {
	ID           string          `json:"id"`
	Agent        string          `json:"agent"`
	AgentVersion string          `json:"agent_version"`
	Model        string          `json:"model"`
	Status       string          `json:"status"`
	Input        json.RawMessage `json:"input"`
	// Batch is set for a run sent through the Message Batches API; BatchID
	// is its batch once submitted.
	Batch   bool   `json:"batch"`
	BatchID string `json:"batch_id,omitempty"`
	// PromptState is kept for a batched run until its batch ends.
	PromptState json.RawMessage `json:"-"`
	Output      json.RawMessage `json:"output,omitempty"`
	Error       string          `json:"error,omitempty"`
	StopReason  string          `json:"stop_reason,omitempty"`
	// RefusalCategory is set on a refused run, e.g. "cyber".
	RefusalCategory string `json:"refusal_category,omitempty"`
	// RequestHash identifies the first request sent to the model, so an
	// identical request can reuse a recent result (see ReusedFrom).
	RequestHash string `json:"-"`
	// ReusedFrom is the run whose output this run reused instead of calling
	// the model again.
	ReusedFrom string  `json:"reused_from,omitempty"`
	Turns      int     `json:"turns"`
	Usage      AIUsage `json:"usage"`
	// CostMicros is the estimated cost in millionths of a US dollar, from
	// the usage and the gateway's price table.
	CostMicros  int64       `json:"cost_micros"`
	RequestedBy string      `json:"requested_by"`
	Trigger     string      `json:"trigger"`
	CreatedAt   time.Time   `json:"created_at"`
	StartedAt   *time.Time  `json:"started_at,omitempty"`
	FinishedAt  *time.Time  `json:"finished_at,omitempty"`
	Steps       []AIRunStep `json:"steps,omitempty"`
}

// AIRunStep is one tool call the model made during a run.
type AIRunStep struct {
	Seq       int             `json:"seq"`
	Turn      int             `json:"turn"`
	Tool      string          `json:"tool"`
	Input     json.RawMessage `json:"input"`
	Output    string          `json:"output"`
	IsError   bool            `json:"is_error"`
	CreatedAt time.Time       `json:"created_at"`
}

// AIAgentInfo describes an agent for GET /api/ai/agents.
type AIAgentInfo struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Model       string   `json:"model"`
	Effort      string   `json:"effort"`
	Tools       []string `json:"tools"`
	// InputSchema is the JSON schema of the run input.
	InputSchema json.RawMessage `json:"input_schema"`
	// ScheduleHours is how often jman-api runs the agent itself; 0 when it
	// only runs on request.
	ScheduleHours int `json:"schedule_hours"`
	// Batch says the agent's scheduled runs go through the Message Batches
	// API, and that callers may ask for a batched run.
	Batch bool `json:"batch"`
}

// AIStatus is the gateway's state for GET /api/ai/status.
type AIStatus struct {
	Enabled bool `json:"enabled"`
	// Fake is set when jman-api answers with canned responses instead of
	// calling the Claude API (local development).
	Fake bool `json:"fake"`
	// Reason says why the gateway is disabled.
	Reason         string  `json:"reason,omitempty"`
	DailyBudgetUSD float64 `json:"daily_budget_usd"`
	SpentTodayUSD  float64 `json:"spent_today_usd"`
}

// AIRunRequest is the body of POST /api/ai/agents/{name}/runs.
type AIRunRequest struct {
	Input json.RawMessage `json:"input,omitempty"`
	// Batch sends the run through the Message Batches API: half price,
	// results within 24 hours. Only for agents that allow it.
	Batch bool `json:"batch,omitempty"`
}

// vuln-rank output.

// AI vulnerability priorities and actions.
const (
	AIPriorityP1 = "P1"
	AIPriorityP2 = "P2"
	AIPriorityP3 = "P3"
	AIPriorityP4 = "P4"

	AIActionUpdateNow      = "update_now"
	AIActionUpdateThisWeek = "update_this_week"
	AIActionRemovePlugin   = "remove_plugin"
	AIActionMuteCandidate  = "mute_candidate"
	AIActionInvestigate    = "investigate"
)

// AIVulnRanking is the vuln-rank agent's output.
type AIVulnRanking struct {
	Rankings []AIVulnRank `json:"rankings"`
	// Unranked lists the keys of groups that were sent but that the model
	// left out.
	Unranked []string `json:"unranked,omitempty"`
}

// AIVulnRank ranks one exposure group. Key is VulnExposureGroupKey of the
// group.
type AIVulnRank struct {
	Key        string `json:"key"`
	Rank       int    `json:"rank"`
	Priority   string `json:"priority"`
	Score      int    `json:"score"`
	Action     string `json:"action"`
	Rationale  string `json:"rationale"`
	Confidence string `json:"confidence"`
}

// VulnExposureGroupKey identifies an exposure group: "plugin:<slug>" or
// "core:<version>".
func VulnExposureGroupKey(kind, slug, version string) string {
	if kind == VulnExposureCore {
		return kind + ":" + version
	}
	return kind + ":" + slug
}
