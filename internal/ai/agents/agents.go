// Package agents lists jman-api's AI agents.
package agents

import (
	"github.com/JCO-Digital/jman/internal/ai"
	"github.com/JCO-Digital/jman/internal/ai/agents/vulnrank"
)

// All returns every agent.
func All() []ai.Agent {
	return []ai.Agent{vulnrank.New()}
}
