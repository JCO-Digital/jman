package ai

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/anthropics/anthropic-sdk-go/v2"
)

// Batched runs go through the Message Batches API: half price, results
// within 24 hours. Batches are outside zero data retention (Anthropic keeps
// them 29 days), so only agents that set Spec.Batchable use them: agents
// whose requests hold no personal data. Each batched request is also
// scanned for personal data (findPersonalData) and sent through the regular
// API when anything is found.

// batchPollInterval is how often pending batches are checked.
const batchPollInterval = time.Minute

// BatchResult is the outcome of a run's request in a batch: the response,
// or why there is none.
type BatchResult struct {
	Message *anthropic.Message
	Failure string
}

// Batcher sends single requests through the Message Batches API.
type Batcher interface {
	Submit(ctx context.Context, customID string, params anthropic.MessageNewParams) (batchID string, err error)
	// Result returns nil while the batch is still being processed.
	Result(ctx context.Context, batchID, customID string) (*BatchResult, error)
	Cancel(ctx context.Context, batchID string) error
}

// batchParams copies the request fields checkZDR allows into a batch
// request.
func batchParams(p anthropic.MessageNewParams) anthropic.MessageBatchNewParamsRequestParams {
	return anthropic.MessageBatchNewParamsRequestParams{
		MaxTokens:     p.MaxTokens,
		Messages:      p.Messages,
		Model:         p.Model,
		System:        p.System,
		Tools:         p.Tools,
		ToolChoice:    p.ToolChoice,
		OutputConfig:  p.OutputConfig,
		Thinking:      p.Thinking,
		CacheControl:  p.CacheControl,
		StopSequences: p.StopSequences,
	}
}

func (m *claudeMessenger) Submit(ctx context.Context, customID string, params anthropic.MessageNewParams) (string, error) {
	b, err := m.client.Messages.Batches.New(ctx, anthropic.MessageBatchNewParams{
		Requests: []anthropic.MessageBatchNewParamsRequest{{CustomID: customID, Params: batchParams(params)}},
	})
	if err != nil {
		return "", err
	}
	return b.ID, nil
}

func (m *claudeMessenger) Result(ctx context.Context, batchID, customID string) (*BatchResult, error) {
	b, err := m.client.Messages.Batches.Get(ctx, batchID, anthropic.MessageBatchGetParams{})
	if err != nil {
		return nil, err
	}
	if b.ProcessingStatus != anthropic.MessageBatchProcessingStatusEnded {
		return nil, nil
	}
	stream := m.client.Messages.Batches.ResultsStreaming(ctx, batchID, anthropic.MessageBatchResultsParams{})
	defer stream.Close()
	for stream.Next() {
		r := stream.Current()
		if r.CustomID != customID {
			continue
		}
		switch r.Result.Type {
		case "succeeded":
			msg := r.Result.Message
			return &BatchResult{Message: &msg}, nil
		case "errored":
			return &BatchResult{Failure: "Claude API: " + r.Result.Error.Error.Message}, nil
		case "expired":
			return &BatchResult{Failure: "the batch expired before the request was processed"}, nil
		default:
			return &BatchResult{Failure: "the batch request was " + r.Result.Type}, nil
		}
	}
	if err := stream.Err(); err != nil {
		return nil, err
	}
	return &BatchResult{Failure: "the batch has no result for the run"}, nil
}

func (m *claudeMessenger) Cancel(ctx context.Context, batchID string) error {
	_, err := m.client.Messages.Batches.Cancel(ctx, batchID, anthropic.MessageBatchCancelParams{})
	return err
}

// fakeBatches keeps fake batch results in memory, for local development.
// They are lost when jman-api restarts.
type fakeBatches struct {
	mu      sync.Mutex
	results map[string]*anthropic.Message
}

// fakeBatcher answers batches with one run's fake messenger.
type fakeBatcher struct {
	store *fakeBatches
	msgr  Messenger
}

func (f fakeBatcher) Submit(ctx context.Context, customID string, params anthropic.MessageNewParams) (string, error) {
	if f.msgr == nil {
		return "", fmt.Errorf("no fake answer")
	}
	msg, err := f.msgr.New(ctx, params)
	if err != nil {
		return "", err
	}
	id := "fakebatch_" + customID
	f.store.mu.Lock()
	defer f.store.mu.Unlock()
	f.store.results[id] = msg
	return id, nil
}

func (f fakeBatcher) Result(ctx context.Context, batchID, customID string) (*BatchResult, error) {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()
	msg, ok := f.store.results[batchID]
	if !ok {
		return &BatchResult{Failure: "the fake batch was lost when jman-api restarted"}, nil
	}
	delete(f.store.results, batchID)
	return &BatchResult{Message: msg}, nil
}

func (f fakeBatcher) Cancel(ctx context.Context, batchID string) error {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()
	delete(f.store.results, batchID)
	return nil
}

// batcherFor returns the batcher; in fake mode, one that answers with msgr.
func (g *Gateway) batcherFor(msgr Messenger) Batcher {
	if g.fake {
		return fakeBatcher{store: g.fakeBatches, msgr: msgr}
	}
	return g.batcher
}

// submitBatch sends run's request as a batch and leaves the run running
// with its batch ID and prompt state, for collectBatches.
func (g *Gateway) submitBatch(ctx context.Context, run *models.AIRun, msgr Messenger,
	params anthropic.MessageNewParams, prompt Prompt) {
	if err := g.checkBudget(run); err != nil {
		run.Status, run.Error = models.AIRunFailed, err.Error()
		return
	}
	b := g.batcherFor(msgr)
	if b == nil {
		run.Status, run.Error = models.AIRunFailed, "batches aren't available"
		return
	}
	id, err := b.Submit(ctx, run.ID, params)
	if err != nil {
		if ctx.Err() != nil {
			g.endOnContext(ctx, run)
			return
		}
		run.Status, run.Error = models.AIRunFailed, "Claude API: "+truncate(err.Error(), 500)
		return
	}
	run.BatchID = id
	run.PromptState = prompt.State
}

// pollBatches collects finished batches until ctx ends.
func (g *Gateway) pollBatches(ctx context.Context) {
	ticker := time.NewTicker(batchPollInterval)
	defer ticker.Stop()
	for {
		g.collectBatches(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// collectBatches finishes the batched runs whose batch has ended.
func (g *Gateway) collectBatches(ctx context.Context) {
	runs, err := db.ListPendingAIBatchRuns()
	if err != nil {
		g.logf("AI gateway: failed to list pending batches: %v", err)
		return
	}
	for i := range runs {
		run := &runs[i]
		res, err := g.batcherFor(nil).Result(ctx, run.BatchID, run.ID)
		if err != nil {
			g.logf("AI run %s: failed to check batch %s: %v", run.ID, run.BatchID, err)
			continue
		}
		if res == nil {
			continue
		}
		a, ok := g.agents[run.Agent]
		switch {
		case !ok:
			run.Status, run.Error = models.AIRunFailed, fmt.Sprintf("unknown agent %q", run.Agent)
		case res.Failure != "":
			run.Status, run.Error = models.AIRunFailed, res.Failure
		default:
			run.Turns = 1
			addUsage(run, run.Model, res.Message, true)
			applyAnswer(ctx, run, a, Prompt{State: run.PromptState}, res.Message)
		}
		g.finish(run)
	}
}
