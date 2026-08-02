package agent

import (
	"fmt"
)

// AgentPolicy enforces runtime boundary limits for autonomous agent execution.
type AgentPolicy struct {
	MaxSteps        int  `json:"max_steps"`
	MaxToolCalls    int  `json:"max_tool_calls"`
	TokenBudget     int  `json:"token_budget"`
	RequireApproval bool `json:"require_approval"`
}

// Validate checks policy parameter sanity.
func (p AgentPolicy) Validate() error {
	if p.MaxSteps <= 0 {
		return fmt.Errorf("MaxSteps must be greater than 0, got %d", p.MaxSteps)
	}
	return nil
}
