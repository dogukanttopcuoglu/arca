package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"arca/internal/agent"
	agenttool "arca/internal/agent/tool"
	"arca/internal/pdfinspector/model"
	"arca/internal/qa"
	qaverification "arca/internal/qa/verification"
	retrievalseam "arca/internal/retrieval/seam"
)

// App encapsulates CLI tool execution handlers wired through the composition root.
type App struct {
	runtime      *Runtime
	answerEngine *qa.AnswerEngine
	agentEngine  *agent.AgentEngine
}

// NewApp constructs an App CLI instance using the composition root populated from
// environment configuration.
func NewApp() *App {
	runtime, err := NewRuntime(LoadFromEnv())
	if err != nil {
		panic(fmt.Sprintf("failed to construct ARC runtime: %v", err))
	}
	return NewAppWithRuntime(runtime)
}

// NewAppWithRuntime constructs an App CLI instance with an explicit composition root,
// allowing tests and alternative entrypoints to inject mock adapters.
func NewAppWithRuntime(runtime *Runtime) *App {
	ansEng := buildAnswerEngine(runtime.cfg, runtime.denseRetriever)
	agentEng := agent.NewAgentEngine(agent.AgentPolicy{MaxSteps: 5, MaxToolCalls: 10}, []agenttool.Tool{
		agenttool.NewKnowledgeTool(ansEng),
	})

	return &App{
		runtime:      runtime,
		answerEngine: ansEng,
		agentEngine:  agentEng,
	}
}

// RunInspect executes the real PDF inspection and indexes the resulting chunks.
func (a *App) RunInspect(ctx context.Context, filePath string) (string, error) {
	if strings.TrimSpace(filePath) == "" {
		return "", fmt.Errorf("file path cannot be empty")
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read PDF file %s: %w", filePath, err)
	}

	docID := filepath.Base(strings.TrimSuffix(filePath, filepath.Ext(filePath)))

	result, err := a.runtime.inspector.InspectPDF(ctx, docID, strings.NewReader(string(data)))
	if err != nil {
		if result != nil && result.Diagnostics.Status == model.StatusFailed {
			return "", fmt.Errorf("inspection failed: %v (errors: %v)", err, result.Diagnostics.Errors)
		}
		return "", fmt.Errorf("inspection failed: %w", err)
	}

	jobObj, err := a.runtime.indexingWorker.ExecuteSync(ctx, result.Document.DocumentID, result.Chunks)
	if err != nil {
		return "", fmt.Errorf("indexing failed: %w", err)
	}

	return fmt.Sprintf(
		"✓ Inspected %s (%d pages, %d chunks)\n✓ Indexed %d chunks (skipped %d, deleted %d) via %s/%s\n✓ Store: %d points",
		filepath.Base(filePath),
		result.Document.PageCount,
		len(result.Chunks),
		jobObj.IndexedChunks,
		jobObj.SkippedChunks,
		jobObj.DeletedChunks,
		jobObj.EmbeddingProvider,
		jobObj.EmbeddingModel,
		a.runtime.StoredPoints(),
	), nil
}

// RunAsk executes a synchronous RAG question over indexed KnowledgeSpace
// documents and renders the final Answer with its citation sources.
func (a *App) RunAsk(ctx context.Context, query string) (string, error) {
	if strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("query string cannot be empty")
	}

	ans, err := a.answerEngine.Answer(ctx, retrievalseam.RetrievalQuery{
		QueryText: query,
		TopK:      5,
	})
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Q: %s\nA: %s", query, ans.Text))

	if len(ans.Citations) > 0 {
		sb.WriteString("\n\nSources:\n")
		for _, c := range ans.Citations {
			sb.WriteString(fmt.Sprintf("%s document: %s · section: %q · page(s) %s\n",
				c.CitationKey, c.DocumentID, c.SectionPath, formatPages(c.PageNumbers)))
		}
	}

	if ans.Status == qaverification.StatusUnverified {
		sb.WriteString("\n⚠ answer contains unverified reference(s) — treat with caution\n")
	}

	return sb.String(), nil
}

// formatPages renders page numbers as a compact comma-separated list.
func formatPages(pages []int) string {
	if len(pages) == 0 {
		return "unknown"
	}
	parts := make([]string, len(pages))
	for i, p := range pages {
		parts[i] = fmt.Sprintf("%d", p)
	}
	return strings.Join(parts, ", ")
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
