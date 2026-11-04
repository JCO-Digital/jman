package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
	"github.com/anthropics/anthropic-sdk-go/v2"
)

// workers is how many runs go on at the same time.
const workers = 2

// Errors returned to callers.
var (
	ErrDisabled     = errors.New("the AI gateway is disabled")
	ErrUnknownAgent = errors.New("unknown agent")
)

// InputError is a caller's invalid run input.
type InputError struct{ Err error }

func (e *InputError) Error() string { return "invalid input: " + e.Err.Error() }
func (e *InputError) Unwrap() error { return e.Err }

// Gateway runs agents. Default is jman-api's; it is set by Start and is
// never nil after it, even when disabled.
type Gateway struct {
	agents            map[string]Agent
	overrides         map[string]config.AIAgentConfig
	messenger         Messenger
	batcher           Batcher
	fakeBatches       *fakeBatches
	fake              bool
	disabledReason    string
	dailyBudgetMicros int64
	now               func() time.Time
	logf              func(format string, args ...any)

	wake    chan struct{}
	mu      sync.Mutex
	waiters map[string][]chan struct{}
	cancels map[string]context.CancelFunc
}

// Default is jman-api's gateway.
var Default = newGateway(nil, nil, nil)

func newGateway(agents []Agent, overrides map[string]config.AIAgentConfig, m Messenger) *Gateway {
	g := &Gateway{
		agents:      map[string]Agent{},
		overrides:   overrides,
		messenger:   m,
		now:         time.Now,
		logf:        log.Printf,
		wake:        make(chan struct{}, 1),
		waiters:     map[string][]chan struct{}{},
		cancels:     map[string]context.CancelFunc{},
		fakeBatches: &fakeBatches{results: map[string]*anthropic.Message{}},
	}
	for _, a := range agents {
		g.agents[a.Spec().Name] = a
	}
	if m == nil {
		g.disabledReason = "the AI gateway isn't started"
	}
	return g
}

// Start sets up Default from the config and starts its workers and
// scheduler. With the gateway off, Default still lists the agents and
// says why it is off. A bad agent setting is an error only when the
// gateway is on.
func Start(ctx context.Context, agents ...Agent) error {
	cfg := config.Cfg
	g := newGateway(agents, cfg.AIAgents, nil)
	g.dailyBudgetMicros = int64(math.Round(cfg.AIDailyBudgetUSD * 1e6))

	switch {
	case !cfg.AIEnabled:
		g.disabledReason = "aiEnabled is off"
	case cfg.AIFake:
		g.fake = true
		g.messenger = fakeMessenger{}
	case cfg.AnthropicAPIKey == "":
		g.disabledReason = "anthropicApiKey isn't set"
	case !cfg.AIZDRConfirmed:
		g.disabledReason = "aiZdrConfirmed isn't set: confirm that the Anthropic organization of anthropicApiKey has zero data retention enabled"
	default:
		claude := newClaudeMessenger(cfg.AnthropicAPIKey)
		g.messenger, g.batcher = claude, claude
	}
	if g.messenger != nil {
		g.disabledReason = ""
		if err := g.validate(); err != nil {
			return err
		}
	}
	Default = g

	if g.disabledReason != "" {
		log.Printf("AI gateway disabled: %s.", g.disabledReason)
		return nil
	}
	if n, err := db.RequeueRunningAIRuns(); err != nil {
		return fmt.Errorf("failed to requeue AI runs: %w", err)
	} else if n > 0 {
		log.Printf("AI gateway: requeued %d interrupted runs.", n)
	}
	mode := "Claude API"
	if g.fake {
		mode = "fake answers"
	}
	log.Printf("Starting AI gateway (%s, %d agents)...", mode, len(g.agents))
	for i := 0; i < workers; i++ {
		go g.work(ctx)
	}
	go g.schedule(ctx)
	go g.pollBatches(ctx)
	return nil
}

// validate checks every agent's settings.
func (g *Gateway) validate() error {
	for name := range g.overrides {
		if _, ok := g.agents[name]; !ok {
			return fmt.Errorf("aiAgents.%s: no such agent", name)
		}
	}
	for name, a := range g.agents {
		spec := a.Spec()
		set := g.settings(spec)
		if err := checkModel(set.Model, set.Effort); err != nil {
			return fmt.Errorf("AI agent %s: %w", name, err)
		}
		if spec.Version == "" || spec.System == "" || len(spec.OutputSchema) == 0 {
			return fmt.Errorf("AI agent %s: Version, System and OutputSchema are required", name)
		}
		if spec.Batchable && len(spec.Tools) > 0 {
			return fmt.Errorf("AI agent %s: an agent with tools can't be batched", name)
		}
		if o, ok := g.overrides[name]; ok && o.Batch != nil && *o.Batch && !spec.Batchable {
			return fmt.Errorf("aiAgents.%s: batch can only turn batching off; the agent isn't batchable", name)
		}
	}
	return nil
}

// settings applies the aiAgents config to spec's defaults.
func (g *Gateway) settings(spec Spec) settings {
	s := settings{
		Model:         spec.Model,
		Effort:        spec.Effort,
		MaxTokens:     spec.MaxTokens,
		MaxTurns:      spec.MaxTurns,
		Timeout:       spec.Timeout,
		ScheduleEvery: spec.ScheduleEvery,
		Batch:         spec.Batchable,
	}
	if s.Model == "" {
		s.Model = DefaultModel
	}
	if s.Effort == "" {
		s.Effort = "medium"
	}
	if s.MaxTokens <= 0 {
		s.MaxTokens = 32000
	}
	if s.MaxTurns <= 0 {
		s.MaxTurns = 1
		if len(spec.Tools) > 0 {
			s.MaxTurns = 10
		}
	}
	if s.Timeout <= 0 {
		s.Timeout = 10 * time.Minute
	}
	if o, ok := g.overrides[spec.Name]; ok {
		if o.Model != "" {
			s.Model = o.Model
		}
		if o.Effort != "" {
			s.Effort = o.Effort
		}
		if o.Batch != nil && !*o.Batch {
			s.Batch = false
		}
		switch {
		case o.ScheduleHours < 0:
			s.ScheduleEvery = 0
		case o.ScheduleHours > 0:
			s.ScheduleEvery = time.Duration(o.ScheduleHours) * time.Hour
		}
	}
	return s
}

// Status reports whether the gateway runs agents and today's spend.
func (g *Gateway) Status() models.AIStatus {
	s := models.AIStatus{
		Enabled:        g.disabledReason == "",
		Fake:           g.fake,
		Reason:         g.disabledReason,
		DailyBudgetUSD: MicrosToUSD(g.dailyBudgetMicros),
	}
	if spent, err := db.SumAICostSince(startOfDayUTC(g.now())); err == nil {
		s.SpentTodayUSD = MicrosToUSD(spent)
	}
	return s
}

// Agents describes the agents, sorted by name.
func (g *Gateway) Agents() []models.AIAgentInfo {
	out := make([]models.AIAgentInfo, 0, len(g.agents))
	for _, a := range g.agents {
		spec := a.Spec()
		set := g.settings(spec)
		info := models.AIAgentInfo{
			Name: spec.Name, Version: spec.Version, Description: spec.Description,
			Model: set.Model, Effort: set.Effort, Tools: []string{},
			InputSchema:   spec.InputSchema,
			ScheduleHours: int(set.ScheduleEvery / time.Hour),
			Batch:         set.Batch,
		}
		if len(info.InputSchema) == 0 {
			info.InputSchema = json.RawMessage(`{"type":"object"}`)
		}
		for _, t := range spec.Tools {
			info.Tools = append(info.Tools, t.Name)
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// HasAgent reports whether name is an agent.
func (g *Gateway) HasAgent(name string) bool {
	_, ok := g.agents[name]
	return ok
}

// Enqueue validates input and queues a run of agent, batched when batch is
// set (the agent must be batchable).
func (g *Gateway) Enqueue(agent string, input json.RawMessage, batch bool, requestedBy, trigger string) (*models.AIRun, error) {
	a, ok := g.agents[agent]
	if !ok {
		return nil, ErrUnknownAgent
	}
	if g.disabledReason != "" {
		return nil, fmt.Errorf("%w: %s", ErrDisabled, g.disabledReason)
	}
	if len(input) == 0 || string(input) == "null" {
		input = json.RawMessage("{}")
	}
	normalized, err := a.ValidateInput(input)
	if err != nil {
		return nil, &InputError{Err: err}
	}
	spec := a.Spec()
	if batch && !g.settings(spec).Batch {
		return nil, &InputError{Err: fmt.Errorf("agent %s can't be batched", agent)}
	}
	run := &models.AIRun{
		ID:           utils.NewV7UUID(),
		Agent:        agent,
		AgentVersion: spec.Version,
		Model:        g.settings(spec).Model,
		Status:       models.AIRunQueued,
		Input:        normalized,
		Batch:        batch,
		RequestedBy:  requestedBy,
		Trigger:      trigger,
		CreatedAt:    g.now(),
	}
	if err := db.InsertAIRun(run); err != nil {
		return nil, err
	}
	select {
	case g.wake <- struct{}{}:
	default:
	}
	return run, nil
}

// Wait waits up to timeout for run id to finish and returns it as it is
// then.
func (g *Gateway) Wait(ctx context.Context, id string, timeout time.Duration) (*models.AIRun, error) {
	ch := make(chan struct{})
	g.mu.Lock()
	g.waiters[id] = append(g.waiters[id], ch)
	g.mu.Unlock()
	defer g.dropWaiter(id, ch)

	run, err := db.GetAIRun(id)
	if err != nil || models.AIRunFinished(run.Status) || timeout <= 0 {
		return run, err
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ch:
	case <-timer.C:
	case <-ctx.Done():
	}
	return db.GetAIRun(id)
}

func (g *Gateway) dropWaiter(id string, ch chan struct{}) {
	g.mu.Lock()
	defer g.mu.Unlock()
	list := g.waiters[id]
	for i, c := range list {
		if c == ch {
			g.waiters[id] = append(list[:i], list[i+1:]...)
			break
		}
	}
	if len(g.waiters[id]) == 0 {
		delete(g.waiters, id)
	}
}

// notify wakes the callers waiting for run id.
func (g *Gateway) notify(id string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, ch := range g.waiters[id] {
		close(ch)
	}
	delete(g.waiters, id)
}

// Cancel cancels a queued or running run and reports whether it was. A
// submitted batch is canceled too.
func (g *Gateway) Cancel(id string) (bool, error) {
	ok, err := db.CancelAIRun(id, g.now())
	if err != nil || !ok {
		return ok, err
	}
	if run, err := db.GetAIRun(id); err == nil && run.BatchID != "" {
		if b := g.batcherFor(nil); b != nil {
			if err := b.Cancel(context.Background(), run.BatchID); err != nil {
				g.logf("AI run %s: failed to cancel batch %s: %v", id, run.BatchID, err)
			}
		}
	}
	g.mu.Lock()
	cancel := g.cancels[id]
	g.mu.Unlock()
	if cancel != nil {
		cancel()
	} else {
		g.notify(id)
	}
	return true, nil
}

// work runs queued runs until ctx ends.
func (g *Gateway) work(ctx context.Context) {
	poll := time.NewTicker(30 * time.Second)
	defer poll.Stop()
	for {
		run, err := db.ClaimNextAIRun(g.now())
		if err != nil {
			g.logf("AI gateway: failed to claim a run: %v", err)
		}
		if run != nil {
			runCtx, cancel := context.WithCancel(ctx)
			g.mu.Lock()
			g.cancels[run.ID] = cancel
			g.mu.Unlock()
			g.execute(runCtx, run)
			cancel()
			g.mu.Lock()
			delete(g.cancels, run.ID)
			g.mu.Unlock()
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-g.wake:
		case <-poll.C:
		}
	}
}

// schedule queues the scheduled agents' runs when they are due.
func (g *Gateway) schedule(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(2 * time.Minute):
	}
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	for {
		g.queueDue()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// queueDue queues a run of every scheduled agent whose last scheduled run
// is older than its interval. Failed and canceled runs don't count, so a
// failure is retried on the next check.
func (g *Gateway) queueDue() {
	for name, a := range g.agents {
		spec := a.Spec()
		set := g.settings(spec)
		every := set.ScheduleEvery
		if every <= 0 {
			continue
		}
		last, err := db.LastAIRunCreatedAt(name, models.AIRunTriggerSchedule)
		if err != nil {
			g.logf("AI gateway: failed to check %s's schedule: %v", name, err)
			continue
		}
		if last != nil && g.now().Sub(*last) < every {
			continue
		}
		if _, err := g.Enqueue(name, spec.ScheduledInput, set.Batch, "system", models.AIRunTriggerSchedule); err != nil {
			g.logf("AI gateway: failed to queue scheduled %s run: %v", name, err)
		}
	}
}
