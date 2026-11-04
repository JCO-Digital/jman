package ai

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/anthropics/anthropic-sdk-go/v2"
)

func setupDB(t *testing.T) {
	t.Helper()
	old := config.RunData
	config.RunData.DataDir = t.TempDir()
	if err := db.InitInventory(); err != nil {
		t.Fatal(err)
	}
	if err := db.InitAPI(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Close()
		config.RunData = old
	})
}

// scripted answers with its responses in order and records the requests.
type scripted struct {
	mu        sync.Mutex
	responses []string
	requests  []anthropic.MessageNewParams
	block     chan struct{} // when set, New waits for it or the context
}

func (s *scripted) New(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error) {
	if s.block != nil {
		select {
		case <-s.block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, params)
	if len(s.responses) == 0 {
		return nil, errors.New("no more scripted responses")
	}
	raw := s.responses[0]
	s.responses = s.responses[1:]
	var msg anthropic.Message
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func textResponse(text string) string {
	b, _ := json.Marshal(map[string]any{
		"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-opus-5-5",
		"content":     []map[string]any{{"type": "text", "text": text}},
		"stop_reason": "end_turn",
		"usage":       map[string]any{"input_tokens": 1000, "output_tokens": 500, "cache_read_input_tokens": 2000, "cache_creation_input_tokens": 100},
	})
	return string(b)
}

const toolUseResponse = `{"id":"msg_0","type":"message","role":"assistant","model":"claude-opus-5-5",
	"content":[{"type":"text","text":"Looking it up."},{"type":"tool_use","id":"toolu_1","name":"lookup","input":{"q":"akismet"}},
		{"type":"tool_use","id":"toolu_2","name":"missing","input":{}}],
	"stop_reason":"tool_use","usage":{"input_tokens":800,"output_tokens":100}}`

// testAgent echoes its answer's "value" with a prefix.
type testAgent struct {
	tools     []Tool
	reuseFor  time.Duration
	batchable bool
	data      string // prompt text; "data: <input>" when empty
}

func (a *testAgent) Spec() Spec {
	return Spec{
		Name: "test", Version: "1", System: "You test.",
		OutputSchema: map[string]any{"type": "object", "additionalProperties": false,
			"required": []string{"value"}, "properties": map[string]any{"value": map[string]any{"type": "string"}}},
		Tools:     a.tools,
		ReuseFor:  a.reuseFor,
		Batchable: a.batchable,
	}
}

func (a *testAgent) ValidateInput(in json.RawMessage) (json.RawMessage, error) {
	var v map[string]any
	if err := json.Unmarshal(in, &v); err != nil {
		return nil, err
	}
	if _, bad := v["bad"]; bad {
		return nil, errors.New("bad input")
	}
	return in, nil
}

func (a *testAgent) Prompt(ctx context.Context, in json.RawMessage) (Prompt, error) {
	text := a.data
	if text == "" {
		text = "data: " + string(in)
	}
	return Prompt{Text: text, State: json.RawMessage(`"state"`)}, nil
}

func (a *testAgent) Finalize(ctx context.Context, p Prompt, answer json.RawMessage) (json.RawMessage, error) {
	var v struct{ Value string }
	if err := json.Unmarshal(answer, &v); err != nil {
		return nil, err
	}
	if string(p.State) != `"state"` {
		return nil, errors.New("lost the prompt state")
	}
	return json.Marshal(map[string]string{"value": "final:" + v.Value})
}

func newTestGateway(a Agent, m Messenger) *Gateway {
	g := newGateway([]Agent{a}, nil, m)
	g.disabledReason = ""
	g.logf = func(string, ...any) {}
	return g
}

// runOne queues a run and runs it to its end.
func runOne(t *testing.T, g *Gateway, input string) *models.AIRun {
	t.Helper()
	queued, err := g.Enqueue("test", json.RawMessage(input), false, "tester", models.AIRunTriggerAPI)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := db.ClaimNextAIRun(time.Now())
	if err != nil || claimed == nil || claimed.ID != queued.ID {
		t.Fatalf("claim: %v %v", claimed, err)
	}
	g.execute(context.Background(), claimed)
	run, err := db.GetAIRun(queued.ID)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func TestSingleCallRun(t *testing.T) {
	setupDB(t)
	m := &scripted{responses: []string{textResponse(`{"value":"ok"}`)}}
	g := newTestGateway(&testAgent{}, m)

	run := runOne(t, g, `{"x":1}`)
	if run.Status != models.AIRunSucceeded || string(run.Output) != `{"value":"final:ok"}` {
		t.Fatalf("run = %s %s %s", run.Status, run.Output, run.Error)
	}
	if run.Turns != 1 || run.Usage.InputTokens != 1000 || run.Usage.CacheReadInputTokens != 2000 {
		t.Errorf("usage = %+v turns %d", run.Usage, run.Turns)
	}
	// 1000*4 + 100*4*1.25 + 2000*0.2 + 500*20 micro-dollars on Opus 5.5.
	if want := int64(4000 + 500 + 400 + 10000); run.CostMicros != want {
		t.Errorf("cost = %d, want %d", run.CostMicros, want)
	}

	req := m.requests[0]
	if req.Model != DefaultModel || req.OutputConfig.Effort != "medium" || len(req.OutputConfig.Format.Schema) == 0 {
		t.Errorf("request model/effort/format = %s %s %v", req.Model, req.OutputConfig.Effort, req.OutputConfig.Format.Schema)
	}
	raw, _ := json.Marshal(req)
	if !strings.Contains(string(raw), `"cache_control":{"type":"ephemeral"}`) {
		t.Errorf("system prompt isn't cached: %s", raw)
	}
	if strings.Contains(string(raw), "thinking") {
		t.Errorf("request sets thinking; Opus 5.5 runs adaptive without it: %s", raw)
	}
}

func TestToolLoopRun(t *testing.T) {
	setupDB(t)
	var gotInput string
	tool := Tool{Name: "lookup", Description: "Looks up.", Properties: map[string]any{"q": map[string]any{"type": "string"}},
		Required: []string{"q"}, Run: func(ctx context.Context, in json.RawMessage) (string, error) {
			gotInput = string(in)
			return "found", nil
		}}
	m := &scripted{responses: []string{toolUseResponse, textResponse(`{"value":"done"}`)}}
	g := newTestGateway(&testAgent{tools: []Tool{tool}}, m)

	run := runOne(t, g, `{}`)
	if run.Status != models.AIRunSucceeded || run.Turns != 2 {
		t.Fatalf("run = %s turns %d: %s", run.Status, run.Turns, run.Error)
	}
	if gotInput != `{"q":"akismet"}` {
		t.Errorf("tool input = %s", gotInput)
	}
	if len(run.Steps) != 2 || run.Steps[0].Output != "found" || !run.Steps[1].IsError {
		t.Errorf("steps = %+v", run.Steps)
	}

	// The second request has the assistant turn and both results in one
	// user message, and auto-caches the conversation.
	second := m.requests[1]
	if len(second.Messages) != 3 {
		t.Fatalf("second request has %d messages", len(second.Messages))
	}
	results, _ := json.Marshal(second.Messages[2])
	if strings.Count(string(results), `"tool_result"`) != 2 || !strings.Contains(string(results), `"is_error":true`) {
		t.Errorf("tool results = %s", results)
	}
	raw, _ := json.Marshal(second)
	if !strings.Contains(string(raw), `"strict":true`) || !strings.Contains(string(raw), `"additionalProperties":false`) {
		t.Errorf("tool isn't strict: %s", raw)
	}
	if err := checkZDR(second); err != nil {
		t.Errorf("checkZDR(second request) = %v", err)
	}
}

func TestRefusedAndCutOffRuns(t *testing.T) {
	setupDB(t)
	refusal := `{"id":"m","type":"message","role":"assistant","model":"claude-opus-5-5","content":[],
		"stop_reason":"refusal","stop_details":{"type":"refusal","category":"cyber","explanation":"policy"},
		"usage":{"input_tokens":10,"output_tokens":1}}`
	cut := strings.Replace(textResponse(`{"val`), `"end_turn"`, `"max_tokens"`, 1)
	m := &scripted{responses: []string{refusal, cut}}
	g := newTestGateway(&testAgent{}, m)

	run := runOne(t, g, `{}`)
	if run.Status != models.AIRunRefused || run.RefusalCategory != "cyber" || !strings.Contains(run.Error, "policy") {
		t.Errorf("refused run = %+v", run)
	}
	run = runOne(t, g, `{"again":true}`)
	if run.Status != models.AIRunFailed || !strings.Contains(run.Error, "cut off") {
		t.Errorf("cut-off run = %s %s", run.Status, run.Error)
	}
}

func TestReuseAndBudget(t *testing.T) {
	setupDB(t)
	m := &scripted{responses: []string{textResponse(`{"value":"a"}`), textResponse(`{"value":"b"}`)}}
	g := newTestGateway(&testAgent{reuseFor: time.Hour}, m)

	first := runOne(t, g, `{}`)
	second := runOne(t, g, `{}`)
	if second.Status != models.AIRunSucceeded || second.ReusedFrom != first.ID || string(second.Output) != string(first.Output) {
		t.Errorf("second run = %+v", second)
	}
	if len(m.requests) != 1 || second.CostMicros != 0 {
		t.Errorf("reuse called the model: %d requests, cost %d", len(m.requests), second.CostMicros)
	}

	// A different input calls the model, until the budget is used up.
	g.dailyBudgetMicros = first.CostMicros + 1
	third := runOne(t, g, `{"other":1}`)
	if third.Status != models.AIRunSucceeded {
		t.Fatalf("third run = %s %s", third.Status, third.Error)
	}
	fourth := runOne(t, g, `{"other":2}`)
	if fourth.Status != models.AIRunFailed || !strings.Contains(fourth.Error, "budget") {
		t.Errorf("fourth run = %s %s", fourth.Status, fourth.Error)
	}
}

func TestEnqueueChecks(t *testing.T) {
	setupDB(t)
	g := newTestGateway(&testAgent{}, &scripted{})
	if _, err := g.Enqueue("nope", nil, false, "u", models.AIRunTriggerAPI); !errors.Is(err, ErrUnknownAgent) {
		t.Errorf("unknown agent: %v", err)
	}
	var inputErr *InputError
	if _, err := g.Enqueue("test", json.RawMessage(`{"bad":1}`), false, "u", models.AIRunTriggerAPI); !errors.As(err, &inputErr) {
		t.Errorf("bad input: %v", err)
	}
	run, err := g.Enqueue("test", nil, false, "u", models.AIRunTriggerAPI)
	if err != nil || string(run.Input) != "{}" {
		t.Errorf("empty input: %v %s", err, run.Input)
	}
	g.disabledReason = "off"
	if _, err := g.Enqueue("test", nil, false, "u", models.AIRunTriggerAPI); !errors.Is(err, ErrDisabled) {
		t.Errorf("disabled: %v", err)
	}
}

func TestCancelRunningRun(t *testing.T) {
	setupDB(t)
	m := &scripted{block: make(chan struct{}), responses: []string{textResponse(`{"value":"late"}`)}}
	g := newTestGateway(&testAgent{}, m)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go g.work(ctx)

	run, err := g.Enqueue("test", nil, false, "u", models.AIRunTriggerAPI)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		r, _ := db.GetAIRun(run.ID)
		if r.Status == models.AIRunRunning {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("run never started")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if ok, err := g.Cancel(run.ID); !ok || err != nil {
		t.Fatalf("Cancel = %v %v", ok, err)
	}
	got, err := g.Wait(context.Background(), run.ID, 5*time.Second)
	if err != nil || got.Status != models.AIRunCanceled || got.FinishedAt == nil {
		t.Fatalf("after cancel: %+v %v", got, err)
	}
	if ok, _ := g.Cancel(run.ID); ok {
		t.Error("canceled a finished run")
	}
}

func TestCheckZDR(t *testing.T) {
	spec := (&testAgent{}).Spec()
	set := newTestGateway(&testAgent{}, nil).settings(spec)
	params := buildParams(spec, set, Prompt{Text: "hi"})
	if err := checkZDR(params); err != nil {
		t.Fatalf("built request refused: %v", err)
	}

	covered := params
	covered.Model = "claude-fable-5-1"
	if err := checkZDR(covered); err == nil {
		t.Error("covered model allowed")
	}

	server := params
	server.Tools = []anthropic.ToolUnionParam{{OfCodeExecutionTool20250825: &anthropic.CodeExecutionTool20250825Param{}}}
	if err := checkZDR(server); err == nil || !strings.Contains(err.Error(), "tool type") {
		t.Errorf("server tool allowed: %v", err)
	}

	geo := params
	geo.InferenceGeo = anthropic.String("us")
	if err := checkZDR(geo); err == nil {
		t.Error("unlisted request field allowed")
	}

	doc := params
	doc.Messages = []anthropic.MessageParam{anthropic.NewUserMessage(
		anthropic.NewDocumentBlock(anthropic.FileDocumentSourceParam{FileID: "file_1"}))}
	if err := checkZDR(doc); err == nil {
		t.Error("Files API document allowed")
	}
}

func TestCheckModel(t *testing.T) {
	for _, tc := range []struct {
		model, effort string
		ok            bool
	}{
		{"claude-opus-5-5", "high", true},
		{"claude-haiku-5-5", "low", true},
		{"claude-fable-5-1", "high", false},
		{"claude-mythos-5-1", "high", false},
		{"claude-haiku-4-5", "low", false},
		{"claude-sonnet-4-6", "xhigh", false},
		{"claude-opus-5-5", "extreme", false},
	} {
		if err := checkModel(tc.model, tc.effort); (err == nil) != tc.ok {
			t.Errorf("checkModel(%s, %s) = %v", tc.model, tc.effort, err)
		}
	}
}

func TestSettingsOverrides(t *testing.T) {
	g := newTestGateway(&testAgent{}, nil)
	g.overrides = map[string]config.AIAgentConfig{"test": {Model: "claude-sonnet-5-5", Effort: "high", ScheduleHours: -1}}
	spec := (&testAgent{}).Spec()
	spec.ScheduleEvery = time.Hour
	s := g.settings(spec)
	if s.Model != "claude-sonnet-5-5" || s.Effort != "high" || s.ScheduleEvery != 0 || s.MaxTurns != 1 {
		t.Errorf("settings = %+v", s)
	}
	if err := g.validate(); err != nil {
		t.Errorf("validate = %v", err)
	}
	g.overrides = map[string]config.AIAgentConfig{"tset": {}}
	if err := g.validate(); err == nil {
		t.Error("override of an unknown agent accepted")
	}
}

func TestHaikuLongContextPrice(t *testing.T) {
	short := costMicros("claude-haiku-5-5", models.AIUsage{InputTokens: 100_000, OutputTokens: 1000})
	long := costMicros("claude-haiku-5-5", models.AIUsage{InputTokens: 100_001, OutputTokens: 1000})
	if short != 10_500 || long != 52_501 {
		t.Errorf("haiku costs = %d, %d", short, long)
	}
}

func TestScheduleRetriesOnlyFreeFailures(t *testing.T) {
	setupDB(t)
	cut := strings.Replace(textResponse(`{"val`), `"end_turn"`, `"max_tokens"`, 1)
	m := &scripted{responses: []string{cut}}
	g := newTestGateway(&testAgent{}, m)
	g.overrides = map[string]config.AIAgentConfig{"test": {ScheduleHours: 24}}

	queued := func() int {
		runs, err := db.ListAIRuns(db.AIRunFilter{Agent: "test"})
		if err != nil {
			t.Fatal(err)
		}
		return len(runs)
	}
	runQueued := func() {
		run, err := db.ClaimNextAIRun(time.Now())
		if err != nil || run == nil {
			t.Fatalf("claim: %v %v", run, err)
		}
		g.execute(context.Background(), run)
	}

	g.queueDue()
	if queued() != 1 {
		t.Fatalf("%d runs after the first check", queued())
	}
	// A free failure (no scripted response: an API error) is retried at
	// the next check.
	m.responses = nil
	runQueued()
	g.queueDue()
	if queued() != 2 {
		t.Fatalf("%d runs after a free failure", queued())
	}
	// A failure that cost money waits for the interval.
	m.responses = []string{cut}
	runQueued()
	g.queueDue()
	if queued() != 2 {
		t.Errorf("%d runs after a paid failure", queued())
	}
}

// testBatcher holds one batch per run until ready is set.
type testBatcher struct {
	mu       sync.Mutex
	ready    bool
	response string
	params   []anthropic.MessageNewParams
	canceled []string
}

func (b *testBatcher) Submit(ctx context.Context, customID string, params anthropic.MessageNewParams) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.params = append(b.params, params)
	return "msgbatch_" + customID, nil
}

func (b *testBatcher) Result(ctx context.Context, batchID, customID string) (*BatchResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.ready {
		return nil, nil
	}
	var msg anthropic.Message
	if err := json.Unmarshal([]byte(b.response), &msg); err != nil {
		return nil, err
	}
	return &BatchResult{Message: &msg}, nil
}

func (b *testBatcher) Cancel(ctx context.Context, batchID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.canceled = append(b.canceled, batchID)
	return nil
}

// runBatched queues a batched run and runs it up to its submission.
func runBatched(t *testing.T, g *Gateway) *models.AIRun {
	t.Helper()
	queued, err := g.Enqueue("test", nil, true, "tester", models.AIRunTriggerSchedule)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := db.ClaimNextAIRun(time.Now())
	if err != nil || claimed == nil {
		t.Fatalf("claim: %v %v", claimed, err)
	}
	g.execute(context.Background(), claimed)
	run, err := db.GetAIRun(queued.ID)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func TestBatchedRun(t *testing.T) {
	setupDB(t)
	m := &scripted{}
	b := &testBatcher{response: textResponse(`{"value":"batched"}`)}
	g := newTestGateway(&testAgent{batchable: true}, m)
	g.batcher = b

	run := runBatched(t, g)
	if run.Status != models.AIRunRunning || run.BatchID == "" || !run.Batch || len(m.requests) != 0 || len(b.params) != 1 {
		t.Fatalf("submitted run = %+v, %d direct requests", run, len(m.requests))
	}
	if err := checkZDR(b.params[0]); err != nil {
		t.Errorf("batched request: %v", err)
	}
	// A restart leaves it waiting for its batch.
	if n, err := db.RequeueRunningAIRuns(); err != nil || n != 0 {
		t.Errorf("requeued %d runs: %v", n, err)
	}

	g.collectBatches(context.Background())
	if run, _ = db.GetAIRun(run.ID); run.Status != models.AIRunRunning {
		t.Fatalf("collected an unfinished batch: %s", run.Status)
	}
	b.ready = true
	g.collectBatches(context.Background())
	run, _ = db.GetAIRun(run.ID)
	if run.Status != models.AIRunSucceeded || string(run.Output) != `{"value":"final:batched"}` {
		t.Fatalf("collected run = %s %s %s", run.Status, run.Output, run.Error)
	}
	// Half of the single-call cost in TestSingleCallRun.
	if run.CostMicros != 14900/2 || len(run.PromptState) != 0 {
		t.Errorf("cost %d, prompt state %s", run.CostMicros, run.PromptState)
	}
}

func TestBatchFallsBackOnPersonalData(t *testing.T) {
	setupDB(t)
	m := &scripted{responses: []string{textResponse(`{"value":"direct"}`)}}
	b := &testBatcher{}
	g := newTestGateway(&testAgent{batchable: true, data: "Owner: Maija Meikäläinen <maija@example.fi>"}, m)
	g.batcher = b

	run := runBatched(t, g)
	if run.Status != models.AIRunSucceeded || run.Batch || run.BatchID != "" || len(b.params) != 0 || len(m.requests) != 1 {
		t.Errorf("run = %+v; %d batched, %d direct", run, len(b.params), len(m.requests))
	}
}

func TestBatchCancelAndChecks(t *testing.T) {
	setupDB(t)
	b := &testBatcher{}
	g := newTestGateway(&testAgent{batchable: true}, &scripted{})
	g.batcher = b

	run := runBatched(t, g)
	if ok, err := g.Cancel(run.ID); !ok || err != nil {
		t.Fatalf("Cancel = %v %v", ok, err)
	}
	if len(b.canceled) != 1 || b.canceled[0] != run.BatchID {
		t.Errorf("canceled batches = %v", b.canceled)
	}
	if runs, _ := db.ListPendingAIBatchRuns(); len(runs) != 0 {
		t.Errorf("%d pending batches after cancel", len(runs))
	}

	plain := newTestGateway(&testAgent{}, &scripted{})
	var inputErr *InputError
	if _, err := plain.Enqueue("test", nil, true, "u", models.AIRunTriggerAPI); !errors.As(err, &inputErr) {
		t.Errorf("batched run of an unbatchable agent: %v", err)
	}
	off := false
	g.overrides = map[string]config.AIAgentConfig{"test": {Batch: &off}}
	if g.settings((&testAgent{batchable: true}).Spec()).Batch {
		t.Error("batch = false override ignored")
	}
	withTools := newTestGateway(&testAgent{batchable: true, tools: []Tool{{Name: "t"}}}, &scripted{})
	if err := withTools.validate(); err == nil {
		t.Error("batchable agent with tools accepted")
	}
}

func TestFindPersonalData(t *testing.T) {
	clean := `CVE-2024-12345 SQL injection in Contact Form 2.3.0 (fixed in 2.3.1.4), CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H,
		9.8, EPSS 0.912, first seen 2026-10-20T10:00:00Z, uuid 01a1a204-25d5-7d12-a4c5-a1837840e007, 12 production sites,
		timestamp 1730000000, version 10.0.19045, "[email removed]"`
	if got := findPersonalDataInText(clean); len(got) != 0 {
		t.Errorf("false positives: %v", got)
	}
	for text, want := range map[string]string{
		"contact maija.meikalainen@example.fi": "email address",
		"call +358 40 123 4567":                "phone number",
		"puh. 040 123 4567":                    "phone number",
		"hetu 131052-308T":                     "personal identity code",
		"IBAN FI21 1234 5600 0007 85":          "IBAN",
		"card 4111 1111 1111 1111":             "payment card number",
		"key sk-ant-api03-abcdefghijklmnop":    "API key",
	} {
		got := findPersonalDataInText(text)
		if !slices.Contains(got, want) {
			t.Errorf("findPersonalDataInText(%q) = %v, want %s", text, got, want)
		}
	}
	// Patterns are matched in the decoded text of the request.
	params := buildParams((&testAgent{}).Spec(), newTestGateway(&testAgent{}, nil).settings((&testAgent{}).Spec()),
		Prompt{Text: "reported by <a href=\"mailto:x@y.org\">x</a>"})
	if got := findPersonalData(params); len(got) != 1 {
		t.Errorf("findPersonalData = %v", got)
	}
}
