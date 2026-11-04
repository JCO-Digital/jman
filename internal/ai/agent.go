// Package ai is jman-api's AI gateway: agents defined in Go that callers
// run through the API, with Claude doing the work. See docs/ai-gateway.md.
//
// Agents are advisory. Their tools only read, and a run's only effect is
// its row in ai_runs; people act on the results through jman's existing
// endpoints.
//
// Requests use only features zero data retention (ZDR) covers: the
// Messages API with custom tools, structured outputs, prompt caching and
// thinking; never the Files or Managed Agents APIs, server tools (code
// execution, MCP) or Covered Models (see zdr.go). The one exception is the
// Message Batches API, for agents whose requests hold no personal data
// (Spec.Batchable, batch.go).
package ai

import (
	"context"
	"encoding/json"
	"time"
)

// DefaultModel is the model an agent uses unless its Spec or the aiAgents
// config says otherwise.
const DefaultModel = "claude-opus-5-5"

// Tool is a function the model may call during a run. It must only read:
// agents are advisory. What it returns goes to the model as data, so it
// must not return secrets or personal data (contacts, emails, payment
// details).
type Tool struct {
	Name        string
	Description string
	// Properties and Required describe the input as a JSON schema object.
	// The tool is strict, so the model's input always matches them.
	Properties map[string]any
	Required   []string
	Run        func(ctx context.Context, input json.RawMessage) (string, error)
}

// Spec is an agent's fixed definition. Change Version whenever the prompt,
// schemas or tools change, so stored runs say which definition made them
// and results of an old definition aren't reused.
type Spec struct {
	Name        string
	Version     string
	Description string
	// Model and Effort are the defaults; the aiAgents config overrides
	// them. Effort is low, medium, high, xhigh or max.
	Model  string
	Effort string
	// System is the system prompt. Keep it free of anything that changes
	// between runs (dates, IDs) so it stays cached.
	System string
	// InputSchema documents the run input for GET /api/ai/agents.
	InputSchema json.RawMessage
	// OutputSchema is the JSON schema the final answer must match
	// (structured outputs). Objects need additionalProperties: false.
	OutputSchema map[string]any
	// Tools make the run an agentic loop; without them a run is one call.
	Tools []Tool
	// MaxTokens bounds each response, thinking included (default 32000).
	MaxTokens int64
	// MaxTurns bounds the model calls in a run (default 1 without tools,
	// 10 with).
	MaxTurns int
	// MaxCostUSD stops a run whose estimated cost goes over it; 0 means no
	// per-run limit (the daily budget still applies).
	MaxCostUSD float64
	// ReuseFor lets a run reuse the output of a run that sent the very
	// same first request at most this long ago, instead of calling the
	// model. 0 turns reuse off.
	ReuseFor time.Duration
	// ScheduleEvery makes jman-api run the agent itself this often, with
	// ScheduledInput. 0 means it only runs on request.
	ScheduleEvery  time.Duration
	ScheduledInput json.RawMessage
	// Timeout bounds a whole run (default 10 minutes).
	Timeout time.Duration
	// Batchable lets the agent's scheduled runs, and runs that ask for it,
	// go through the Message Batches API: half price, but outside zero data
	// retention (Anthropic keeps batches 29 days) and with results within
	// 24 hours. Set it only when the agent's requests never hold personal
	// data (names, contacts, site owners); requests that look like they do
	// are sent through the regular API anyway. Agents with tools can't be
	// batched.
	Batchable bool
}

// Agent is one kind of AI task.
type Agent interface {
	Spec() Spec
	// ValidateInput checks a caller's input before the run is queued and
	// returns it normalized. An empty input is "{}".
	ValidateInput(input json.RawMessage) (json.RawMessage, error)
	// Prompt gathers the data to work on and builds the first user message.
	// Data from outside jman (vulnerability feeds, scanned files) must be
	// marked as untrusted in it.
	Prompt(ctx context.Context, input json.RawMessage) (Prompt, error)
	// Finalize checks and completes the model's answer, which matches
	// OutputSchema, and returns the run's output.
	Finalize(ctx context.Context, p Prompt, answer json.RawMessage) (json.RawMessage, error)
}

// Prompt is a run's first user message, Text, with whatever the agent
// needs to keep from building it for Finalize, State. State is stored with
// a batched run until its batch ends, so it must be JSON.
type Prompt struct {
	Text  string
	State json.RawMessage
	// Skip, when set, is the run's output without calling the model, for
	// when there is nothing to work on.
	Skip json.RawMessage
}

// Faker is implemented by agents that can answer without the model, for
// local development (aiFake).
type Faker interface {
	FakeAnswer(ctx context.Context, p Prompt) (json.RawMessage, error)
}

// settings are an agent's Spec with the config overrides applied.
type settings struct {
	Model         string
	Effort        string
	MaxTokens     int64
	MaxTurns      int
	Timeout       time.Duration
	ScheduleEvery time.Duration
	Batch         bool
}
