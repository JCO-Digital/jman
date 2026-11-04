package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

// Messenger sends one Messages API request and returns the complete
// response.
type Messenger interface {
	New(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error)
}

// claudeMessenger calls the Claude API. It streams every response, so long
// answers (thinking included) don't run into HTTP timeouts.
type claudeMessenger struct {
	client anthropic.Client
}

func newClaudeMessenger(apiKey string) *claudeMessenger {
	return &claudeMessenger{client: anthropic.NewClient(
		option.WithAPIKey(apiKey),
		option.WithMaxRetries(3),
		option.WithRequestTimeout(10*time.Minute),
	)}
}

func (m *claudeMessenger) New(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error) {
	stream := m.client.Messages.NewStreaming(ctx, params)
	defer stream.Close()
	msg := anthropic.Message{}
	for stream.Next() {
		if err := msg.Accumulate(stream.Current()); err != nil {
			return nil, fmt.Errorf("failed to read the response stream: %w", err)
		}
	}
	if err := stream.Err(); err != nil {
		return nil, err
	}
	return &msg, nil
}

// fakeMessenger answers with an agent's FakeAnswer, for local development.
// It reports no usage, so fake runs cost nothing.
type fakeMessenger struct {
	answer json.RawMessage
}

func (m fakeMessenger) New(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error) {
	return fakeMessage(string(params.Model), string(m.answer), anthropic.StopReasonEndTurn, 0, 0)
}

// fakeMessage builds a response with one text block.
func fakeMessage(model, text string, stop anthropic.StopReason, in, out int64) (*anthropic.Message, error) {
	raw, err := json.Marshal(map[string]any{
		"id": "msg_fake", "type": "message", "role": "assistant", "model": model,
		"content":     []map[string]any{{"type": "text", "text": text}},
		"stop_reason": stop,
		"usage":       map[string]any{"input_tokens": in, "output_tokens": out},
	})
	if err != nil {
		return nil, err
	}
	var msg anthropic.Message
	if err := json.Unmarshal(raw, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}
