package tool

import (
	"context"
	"fmt"

	"arca/internal/qa"
	retrievalseam "arca/internal/retrieval/seam"
)

// KnowledgeTool adapts AnswerEngine into a tool callable by AgentEngine.
type KnowledgeTool struct {
	engine *qa.AnswerEngine
}

// NewKnowledgeTool constructs a KnowledgeTool instance.
func NewKnowledgeTool(engine *qa.AnswerEngine) *KnowledgeTool {
	return &KnowledgeTool{
		engine: engine,
	}
}

// Name returns the unique tool identifier.
func (k *KnowledgeTool) Name() string {
	return "knowledge_search"
}

// Description describes tool capability for Agent planning.
func (k *KnowledgeTool) Description() string {
	return "Search and answer questions over KnowledgeSpace documents"
}

// Execute calls AnswerEngine.Answer to produce intermediate research findings.
func (k *KnowledgeTool) Execute(ctx context.Context, input ToolInput) (ToolResult, error) {
	if input.Query == "" {
		return ToolResult{}, fmt.Errorf("knowledge search query cannot be empty")
	}

	if k.engine == nil {
		return ToolResult{
			Summary: fmt.Sprintf("Executed mock knowledge search for %q", input.Query),
		}, nil
	}

	ans, err := k.engine.Answer(ctx, retrievalseam.RetrievalQuery{
		QueryText: input.Query,
	})
	if err != nil {
		return ToolResult{}, err
	}

	return ToolResult{
		Summary: fmt.Sprintf("Answer for query %q grounded in %d sources", input.Query, len(ans.Citations)),
		Data:    ans,
	}, nil
}
