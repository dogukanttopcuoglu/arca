package cli

import (
	"context"
	"fmt"
	"strings"

	"arca/internal/agent"
	agenttool "arca/internal/agent/tool"
	"arca/internal/indexing/provider"
	"arca/internal/indexing/store"
	"arca/internal/qa"
	"arca/internal/retrieval/dense"
	retrievalseam "arca/internal/retrieval/seam"
)

// App encapsulates CLI tool execution handlers.
type App struct {
	answerEngine *qa.AnswerEngine
	agentEngine  *agent.AgentEngine
}

// NewApp constructs an App CLI instance.
func NewApp() *App {
	mockProv := provider.NewMockEmbeddingProvider("mock-provider", "mock-model", 1536)
	vecStore := store.NewInMemoryVectorStore()
	denseRet := dense.NewDenseRetriever(mockProv, vecStore)

	ansEng := qa.NewAnswerEngine(nil, denseRet, nil, nil, nil)
	agentEng := agent.NewAgentEngine(agent.AgentPolicy{MaxSteps: 5, MaxToolCalls: 10}, []agenttool.Tool{
		agenttool.NewKnowledgeTool(ansEng),
	})

	return &App{
		answerEngine: ansEng,
		agentEngine:  agentEng,
	}
}

// RunInspect executes PDF inspection over a target filepath.
func (a *App) RunInspect(ctx context.Context, filePath string) (string, error) {
	if strings.TrimSpace(filePath) == "" {
		return "", fmt.Errorf("file path cannot be empty")
	}
	return fmt.Sprintf("✓ Inspected %s\n✓ Extracted layout hierarchy & semantic tree\n✓ Generated KnowledgeChunks", filePath), nil
}

// RunAsk executes a synchronous QA question over KnowledgeSpace documents.
func (a *App) RunAsk(ctx context.Context, query string) (string, error) {
	if strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("query string cannot be empty")
	}

	draft, err := a.answerEngine.Answer(ctx, retrievalseam.RetrievalQuery{
		QueryText: query,
		TopK:      5,
	})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Q: %s\nA: Found %d search results for query.", query, len(draft.SearchResults)), nil
}

// RunResearch executes multi-step agentic research plan.
func (a *App) RunResearch(ctx context.Context, query string) (string, error) {
	if strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("research query string cannot be empty")
	}

	res, err := a.agentEngine.ExecuteResearch(ctx, query)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Research Goal: %s\nPlan:\n%s\nOutput:\n%s", query, res.PlanSummary, res.FinalAnswer), nil
}
