package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JCO-Digital/jman/internal/ai"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/models"
)

// echoAgent answers {"echo": <input>} in fake mode.
type echoAgent struct{}

func (echoAgent) Spec() ai.Spec {
	return ai.Spec{Name: "echo", Version: "1", System: "Echo.",
		OutputSchema: map[string]any{"type": "object", "additionalProperties": false,
			"required": []string{"echo"}, "properties": map[string]any{"echo": map[string]any{"type": "object"}}}}
}

func (echoAgent) ValidateInput(in json.RawMessage) (json.RawMessage, error) {
	if strings.Contains(string(in), "bad") {
		return nil, errors.New("bad input")
	}
	return in, nil
}

func (echoAgent) Prompt(ctx context.Context, in json.RawMessage) (ai.Prompt, error) {
	return ai.Prompt{Text: string(in)}, nil
}

func (echoAgent) Finalize(ctx context.Context, p ai.Prompt, answer json.RawMessage) (json.RawMessage, error) {
	return answer, nil
}

func (echoAgent) FakeAnswer(ctx context.Context, p ai.Prompt) (json.RawMessage, error) {
	return json.Marshal(map[string]json.RawMessage{"echo": json.RawMessage(p.Text)})
}

func setupAITest(t *testing.T, enabled bool) {
	t.Helper()
	setupIncidentTest(t)
	oldCfg, oldGateway := config.Cfg, ai.Default
	config.Cfg.AIEnabled, config.Cfg.AIFake, config.Cfg.AIDailyBudgetUSD = enabled, true, 1
	ctx, cancel := context.WithCancel(context.Background())
	if err := ai.Start(ctx, echoAgent{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		config.Cfg, ai.Default = oldCfg, oldGateway
	})
}

func aiRequest(t *testing.T, h http.HandlerFunc, method, target, body string, pathValues ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	for i := 0; i+1 < len(pathValues); i += 2 {
		req.SetPathValue(pathValues[i], pathValues[i+1])
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func TestAIRunLifecycle(t *testing.T) {
	setupAITest(t, true)

	rec := aiRequest(t, CreateAIRunHandler, "POST", "/api/ai/agents/echo/runs?wait=10", `{"input":{"n":1}}`, "name", "echo")
	if rec.Code != http.StatusOK {
		t.Fatalf("create = %d %s", rec.Code, rec.Body)
	}
	var run models.AIRun
	if err := json.Unmarshal(rec.Body.Bytes(), &run); err != nil {
		t.Fatal(err)
	}
	if run.Status != models.AIRunSucceeded || string(run.Output) != `{"echo":{"n":1}}` || run.Trigger != models.AIRunTriggerAPI {
		t.Fatalf("run = %+v", run)
	}

	rec = aiRequest(t, GetAIRunHandler, "GET", "/api/ai/runs/"+run.ID, "", "id", run.ID)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), run.ID) {
		t.Errorf("get = %d %s", rec.Code, rec.Body)
	}
	rec = aiRequest(t, ListAIRunsHandler, "GET", "/api/ai/runs?agent=echo&status=succeeded", "")
	var runs []models.AIRun
	if err := json.Unmarshal(rec.Body.Bytes(), &runs); err != nil || len(runs) != 1 {
		t.Errorf("list = %s", rec.Body)
	}
	rec = aiRequest(t, CancelAIRunHandler, "POST", "/api/ai/runs/"+run.ID+"/cancel", "", "id", run.ID)
	if rec.Code != http.StatusConflict {
		t.Errorf("cancel finished run = %d", rec.Code)
	}
	rec = aiRequest(t, GetAIRunHandler, "GET", "/api/ai/runs/nope", "", "id", "nope")
	if rec.Code != http.StatusNotFound {
		t.Errorf("get unknown = %d", rec.Code)
	}

	rec = aiRequest(t, ListAIAgentsHandler, "GET", "/api/ai/agents", "")
	if !strings.Contains(rec.Body.String(), `"name":"echo"`) || !strings.Contains(rec.Body.String(), `"model":"claude-opus-5-5"`) {
		t.Errorf("agents = %s", rec.Body)
	}
	rec = aiRequest(t, AIStatusHandler, "GET", "/api/ai/status", "")
	if !strings.Contains(rec.Body.String(), `"enabled":true`) || !strings.Contains(rec.Body.String(), `"fake":true`) {
		t.Errorf("status = %s", rec.Body)
	}
}

func TestAIRunRequestErrors(t *testing.T) {
	setupAITest(t, true)
	for _, tc := range []struct {
		name, target, body string
		want               int
	}{
		{"unknown agent", "/api/ai/agents/nope/runs", ``, http.StatusNotFound},
		{"bad wait", "/api/ai/agents/echo/runs?wait=999", ``, http.StatusBadRequest},
		{"bad body", "/api/ai/agents/echo/runs", `{`, http.StatusBadRequest},
		{"bad input", "/api/ai/agents/echo/runs", `{"input":{"bad":true}}`, http.StatusBadRequest},
		{"no body, no wait", "/api/ai/agents/echo/runs", ``, http.StatusAccepted},
	} {
		name := strings.Split(strings.TrimPrefix(tc.target, "/api/ai/agents/"), "/")[0]
		rec := aiRequest(t, CreateAIRunHandler, "POST", tc.target, tc.body, "name", name)
		if rec.Code != tc.want {
			t.Errorf("%s: %d %s", tc.name, rec.Code, rec.Body)
		}
	}
}

func TestAIRunDisabled(t *testing.T) {
	setupAITest(t, false)
	rec := aiRequest(t, CreateAIRunHandler, "POST", "/api/ai/agents/echo/runs", ``, "name", "echo")
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "aiEnabled is off") {
		t.Errorf("create while disabled = %d %s", rec.Code, rec.Body)
	}
	rec = aiRequest(t, AIStatusHandler, "GET", "/api/ai/status", "")
	if !strings.Contains(rec.Body.String(), `"enabled":false`) {
		t.Errorf("status = %s", rec.Body)
	}
}
