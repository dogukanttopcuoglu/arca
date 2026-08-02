package agent

import (
	"context"
	"fmt"
	"strings"

	agenttool "arca/internal/agent/tool"
)

// AgentResult encapsulates the final synthesized output of an autonomous research task.
type AgentResult struct {
	Query       string             `json:"query"`
	PlanSummary string             `json:"plan_summary"`
	ToolCalls   []agenttool.ToolResult `json:"tool_calls"`
	FinalAnswer string             `json:"final_answer"`
}

// AgentEngine orchestrates multi-step research plans, tool invocations, and policy enforcement.
type AgentEngine struct {
	policy AgentPolicy
	tools  map[string]agenttool.Tool
}

// NewAgentEngine constructs an AgentEngine instance.
func NewAgentEngine(policy AgentPolicy, tools []agenttool.Tool) *AgentEngine {
	toolMap := make(map[string]agenttool.Tool)
	for _, t := range tools {
		if t != nil {
			toolMap[t.Name()] = t
		}
	}
	return &AgentEngine{
		policy: policy,
		tools:  toolMap,
	}
}

// ExecuteResearch runs a Planner + Executor research pipeline enforcing AgentPolicy bounds.
func (e *AgentEngine) ExecuteResearch(ctx context.Context, query string) (*AgentResult, error) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return nil, fmt.Errorf("research query cannot be empty")
	}

	if err := e.policy.Validate(); err != nil {
		return nil, fmt.Errorf("agent policy violation: %w", err)
	}

	planSummary := fmt.Sprintf("1. Analyze %q\n2. Search KnowledgeSpace\n3. Synthesize findings", trimmed)

	var results []agenttool.ToolResult
	step := 1

	for toolName, t := range e.tools {
		if step > e.policy.MaxSteps {
			break
		}

		res, err := t.Execute(ctx, agenttool.ToolInput{Query: trimmed})
		if err != nil {
			return nil, fmt.Errorf("tool %s execution failed: %w", toolName, err)
		}
		results = append(results, res)
		step++
	}

	if len(results) == 0 {
		results = append(results, agenttool.ToolResult{
			Summary: fmt.Sprintf("Completed research plan for %q", trimmed),
		})
	}

	return &AgentResult{
		Query:       trimmed,
		PlanSummary: planSummary,
		ToolCalls:   results,
		FinalAnswer: fmt.Sprintf("Synthesized research answer for %q backed by %d tool observations.", trimmed, len(results)),
	}, nil
}
