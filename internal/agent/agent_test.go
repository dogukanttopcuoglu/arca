package agent_test

import (
	"context"
	"testing"

	"arca/internal/agent"
	agenttool "arca/internal/agent/tool"
	"arca/internal/qa"
)

func TestAgentEngine(t *testing.T) {
	ctx := context.Background()

	knowledgeTool := agenttool.NewKnowledgeTool(qa.NewAnswerEngine(nil, nil, nil, nil, nil, nil, nil))
	policy := agent.AgentPolicy{
		MaxSteps:     5,
		MaxToolCalls: 10,
	}

	engine := agent.NewAgentEngine(policy, []agenttool.Tool{knowledgeTool})

	t.Run("executes multi-step research plan using tool seams", func(t *testing.T) {
		res, err := engine.ExecuteResearch(ctx, "Synthesize creativity and discipline principles")
		if err != nil {
			t.Fatalf("unexpected error executing research: %v", err)
		}

		if res == nil {
			t.Fatal("expected non-nil AgentResult")
		}
		if res.PlanSummary == "" {
			t.Error("expected non-empty PlanSummary")
		}
		if len(res.ToolCalls) == 0 {
			t.Error("expected at least 1 ToolCall in result")
		}
	})

	t.Run("enforces policy limits when steps exceed MaxSteps", func(t *testing.T) {
		strictPolicy := agent.AgentPolicy{
			MaxSteps: 0,
		}
		strictEngine := agent.NewAgentEngine(strictPolicy, []agenttool.Tool{knowledgeTool})

		_, err := strictEngine.ExecuteResearch(ctx, "Query")
		if err == nil {
			t.Error("expected error when MaxSteps exceeded, got nil")
		}
	})
}
