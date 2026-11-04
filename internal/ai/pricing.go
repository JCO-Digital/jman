package ai

import (
	"math"

	"github.com/JCO-Digital/jman/internal/models"
)

// modelPrice is a model's Claude API price in US dollars per million
// tokens. Writing the prompt cache (5-minute TTL) costs 1.25 times the
// input price.
type modelPrice struct {
	Input, Output, CacheRead float64
	// Prompts with more input tokens than LongContextAbove (0: no such
	// tier) cost LongInput and LongOutput instead.
	LongContextAbove      int64
	LongInput, LongOutput float64
	// NoXHigh marks models without the xhigh effort level.
	NoXHigh bool
}

// prices lists the models agents may use: the ones zero data retention
// covers and that take the effort parameter. The costs they give are
// estimates for the daily budget; the Claude Console has the real bill.
var prices = map[string]modelPrice{
	"claude-opus-5-5":   {Input: 4, Output: 20, CacheRead: 0.20},
	"claude-opus-5":     {Input: 5, Output: 25, CacheRead: 0.50},
	"claude-opus-4-8":   {Input: 5, Output: 25, CacheRead: 0.50},
	"claude-opus-4-7":   {Input: 5, Output: 25, CacheRead: 0.50},
	"claude-opus-4-6":   {Input: 5, Output: 25, CacheRead: 0.50, NoXHigh: true},
	"claude-sonnet-5-5": {Input: 2, Output: 10, CacheRead: 0.20},
	"claude-sonnet-5":   {Input: 2, Output: 10, CacheRead: 0.20},
	"claude-sonnet-4-6": {Input: 3, Output: 15, CacheRead: 0.30, NoXHigh: true},
	"claude-haiku-5-5": {Input: 0.10, Output: 0.50, CacheRead: 0.01,
		LongContextAbove: 100_000, LongInput: 0.50, LongOutput: 2.50},
}

// costMicros estimates the cost of one response's usage in millionths of a
// US dollar. Tokens times dollars per million tokens is exactly that.
func costMicros(model string, u models.AIUsage) int64 {
	p, ok := prices[model]
	if !ok {
		return 0
	}
	in, out, read := p.Input, p.Output, p.CacheRead
	prompt := u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens
	if p.LongContextAbove > 0 && prompt > p.LongContextAbove {
		in, out, read = p.LongInput, p.LongOutput, p.LongInput/10
	}
	cost := float64(u.InputTokens)*in +
		float64(u.CacheCreationInputTokens)*in*1.25 +
		float64(u.CacheReadInputTokens)*read +
		float64(u.OutputTokens)*out
	return int64(math.Round(cost))
}

// MicrosToUSD converts millionths of a dollar to dollars.
func MicrosToUSD(micros int64) float64 {
	return float64(micros) / 1e6
}
