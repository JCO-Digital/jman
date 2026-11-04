package ai

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/v2"
)

// coveredModelPrefixes are model families that need 30-day data retention
// ("Covered Models") and so aren't available under ZDR.
var coveredModelPrefixes = []string{"claude-fable", "claude-mythos"}

var validEfforts = map[string]bool{"low": true, "medium": true, "high": true, "xhigh": true, "max": true}

// checkModel reports whether an agent may use model at effort.
func checkModel(model, effort string) error {
	for _, prefix := range coveredModelPrefixes {
		if strings.HasPrefix(model, prefix) {
			return fmt.Errorf("model %s needs 30-day data retention and isn't available under zero data retention", model)
		}
	}
	p, ok := prices[model]
	if !ok {
		return fmt.Errorf("model %s isn't supported; use one of %s", model, strings.Join(SupportedModels(), ", "))
	}
	if !validEfforts[effort] {
		return fmt.Errorf("effort %q must be low, medium, high, xhigh or max", effort)
	}
	if effort == "xhigh" && p.NoXHigh {
		return fmt.Errorf("model %s has no xhigh effort", model)
	}
	return nil
}

// SupportedModels lists the models agents may use, sorted.
func SupportedModels() []string {
	out := make([]string, 0, len(prices))
	for m := range prices {
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}

// zdrRequestFields are the Messages API request fields the gateway sends,
// all covered by zero data retention. Anything else (container, MCP
// servers, message threads, diagnostics, ...) is refused.
var zdrRequestFields = map[string]bool{
	"model": true, "max_tokens": true, "messages": true, "system": true, "tools": true,
	"tool_choice": true, "output_config": true, "thinking": true, "cache_control": true,
	"stop_sequences": true,
}

// zdrContentTypes are the message content blocks the gateway sends. A
// document or image could reference the Files API, which ZDR doesn't
// cover; agents send their data as text.
var zdrContentTypes = map[string]bool{
	"text": true, "tool_use": true, "tool_result": true, "thinking": true, "redacted_thinking": true,
}

// checkZDR refuses a request that uses anything zero data retention
// doesn't cover. It is a second line of defense: the gateway only builds
// requests from these parts anyway.
func checkZDR(params anthropic.MessageNewParams) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("failed to encode request: %w", err)
	}
	var req struct {
		Model    string           `json:"model"`
		Tools    []map[string]any `json:"tools"`
		Messages []struct {
			Content []map[string]any `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return fmt.Errorf("failed to decode request: %w", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return fmt.Errorf("failed to decode request: %w", err)
	}
	for name := range fields {
		if !zdrRequestFields[name] {
			return fmt.Errorf("request field %q isn't allowed under zero data retention", name)
		}
	}
	for _, prefix := range coveredModelPrefixes {
		if strings.HasPrefix(req.Model, prefix) {
			return fmt.Errorf("model %s isn't available under zero data retention", req.Model)
		}
	}
	for _, t := range req.Tools {
		if typ, ok := t["type"].(string); ok && typ != "custom" {
			return fmt.Errorf("tool type %q isn't allowed: only custom tools run under zero data retention", typ)
		}
	}
	for _, m := range req.Messages {
		for _, block := range m.Content {
			if typ, _ := block["type"].(string); !zdrContentTypes[typ] {
				return fmt.Errorf("content block type %q isn't allowed under zero data retention", typ)
			}
		}
	}
	return nil
}
