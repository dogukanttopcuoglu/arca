package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"arca/internal/agent"
	agenttool "arca/internal/agent/tool"
	indexingmodel "arca/internal/indexing/model"
	"arca/internal/pdfinspector/model"
	"arca/internal/qa"
	qaverification "arca/internal/qa/verification"
	"arca/internal/retrieval/rerank"
	retrievalseam "arca/internal/retrieval/seam"
)

// App encapsulates CLI tool execution handlers wired through the composition root.
type App struct {
	runtime           *Runtime
	answerEngine      *qa.AnswerEngine
	agentEngine       *agent.AgentEngine
	retrievalMinScore float32
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
	ansEng, err := runtime.AnswerEngine()
	if err != nil {
		panic(fmt.Sprintf("failed to construct retriever for mode %s: %v", runtime.cfg.RetrievalMode, err))
	}
	agentEng := agent.NewAgentEngine(agent.AgentPolicy{MaxSteps: 5, MaxToolCalls: 10}, []agenttool.Tool{
		agenttool.NewKnowledgeTool(ansEng),
	})

	return &App{
		runtime:           runtime,
		answerEngine:      ansEng,
		agentEngine:       agentEng,
		retrievalMinScore: runtime.cfg.RetrievalMinScore,
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

	jobObj, err := a.runtime.indexingWorker.ExecuteSync(ctx, result.Document.DocumentID, result.Document.Title, result.Chunks, &result.Document)
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
// docFilter, when non-empty, is a case-insensitive substring of the target
// document ID; it is resolved against the live index before retrieval and
// scopes the query to that document (retrieval and graph legs).
func (a *App) RunAsk(ctx context.Context, query string, docFilter string) (string, error) {
	if strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("query string cannot be empty")
	}

	queryFilter, err := a.resolveDocFilter(ctx, docFilter)
	if err != nil {
		return "", err
	}

	ans, err := a.answerEngine.Answer(ctx, retrievalseam.RetrievalQuery{
		QueryText: query,
		TopK:      5,
		MinScore:  a.retrievalMinScore,
		Filter:    queryFilter,
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

	var reranker *rerank.HTTPReranker
	if a.runtime != nil {
		reranker = a.runtime.reranker
	}
	sb.WriteString("\n" + renderRerankerStats(reranker))

	return sb.String(), nil
}

// renderRerankerStats renders the M10 observability block (ADR-0049):
// whether reranking is configured and, when it is, the adapter counters.
// A nil adapter means default-off (empty RETRIEVAL_RERANK_URL).
func renderRerankerStats(r *rerank.HTTPReranker) string {
	var sb strings.Builder
	sb.WriteString("Reranker:\n")
	if r == nil {
		sb.WriteString("  Enabled: false\n")
		return sb.String()
	}
	requests := r.RequestsTotal()
	avg := int64(0)
	if requests > 0 {
		avg = r.LatencyMsTotal() / requests
	}
	fmt.Fprintf(&sb, "  Enabled: true\n")
	fmt.Fprintf(&sb, "  Requests: %d\n", requests)
	fmt.Fprintf(&sb, "  Failures: %d\n", r.FailuresTotal())
	fmt.Fprintf(&sb, "  AvgLatencyMs: %d\n", avg)
	fmt.Fprintf(&sb, "  Degraded: %t\n", r.LastDegraded())
	return sb.String()
}

// DocumentEntry is one indexed document in the live index: its ID and the
// number of indexed chunks (for display and interactive selection).
type DocumentEntry struct {
	DocumentID string
	Chunks     int
}

// listDocuments enumerates the distinct documents of the live index with
// their chunk counts (one full ListPoints scan, used by --doc resolution,
// `arc docs`, and the interactive picker).
func (a *App) listDocuments(ctx context.Context) ([]DocumentEntry, error) {
	points, err := a.runtime.vectorStore.ListPoints(ctx, indexingmodel.MetadataFilter{})
	if err != nil {
		return nil, fmt.Errorf("failed to list indexed documents: %w", err)
	}
	counts := make(map[string]int)
	for _, pt := range points {
		if pt.Metadata.DocumentID != "" {
			counts[pt.Metadata.DocumentID]++
		}
	}
	docs := make([]DocumentEntry, 0, len(counts))
	for id, n := range counts {
		docs = append(docs, DocumentEntry{DocumentID: id, Chunks: n})
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].DocumentID < docs[j].DocumentID })
	return docs, nil
}

// RunListDocs renders the indexed documents with their chunk counts —
// `arc docs`.
func (a *App) RunListDocs(ctx context.Context) (string, error) {
	docs, err := a.listDocuments(ctx)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d indexed document(s):\n", len(docs)))
	for _, d := range docs {
		sb.WriteString(fmt.Sprintf("  %-3d chunks  %s\n", d.Chunks, d.DocumentID))
	}
	return sb.String(), nil
}

// RunAskInteractive drives the interactive picker: lists the indexed
// documents, asks the user to select one (0 = whole corpus), then asks for
// the question and delegates to RunAsk.
func (a *App) RunAskInteractive(ctx context.Context, in *bufio.Reader, out io.Writer) error {
	docs, err := a.listDocuments(ctx)
	if err != nil {
		return err
	}
	if len(docs) == 0 {
		return fmt.Errorf("no indexed documents — index a PDF first (arc inspect <file.pdf>)")
	}

	fmt.Fprintf(out, "Indexed library (%d document(s)):\n", len(docs))
	for i, d := range docs {
		fmt.Fprintf(out, "  %2d) %s (%d chunks)\n", i+1, friendlyDocName(d.DocumentID), d.Chunks)
	}
	fmt.Fprintf(out, "   0) hepsi / all documents\n")
	fmt.Fprintf(out, "\nKütüphane seçin (0-9): ")

	choice, err := in.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read selection: %w", err)
	}
	choice = strings.TrimSpace(choice)

	docFilter := ""
	if choice != "" && choice != "0" {
		idx := -1
		if _, err := fmt.Sscanf(choice, "%d", &idx); err != nil || idx < 1 || idx > len(docs) {
			return fmt.Errorf("geçersiz seçim %q (0-%d)", choice, len(docs))
		}
		docFilter = docs[idx-1].DocumentID
	}

	fmt.Fprintf(out, "Sorunuz: ")
	query, err := in.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read query: %w", err)
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return fmt.Errorf("boş soru")
	}

	result, err := a.RunAsk(ctx, query, docFilter)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "\n%s\n", result)
	return nil
}

// friendlyDocName derives a readable label from a document ID: the trailing
// " - libgen.li" is dropped, underscores become spaces, and the label is
// truncated to a display width.
func friendlyDocName(docID string) string {
	label := docID
	if i := strings.LastIndex(label, " - libgen.li"); i > 0 {
		label = label[:i]
	}
	label = strings.ReplaceAll(label, "_", " ")
	label = strings.Join(strings.Fields(label), " ")
	label = strings.TrimSpace(label)
	const max = 90
	if len(label) > max {
		label = label[:max] + "…"
	}
	return label
}

// resolveDocFilter turns a user-supplied document substring into the exact
// document ID(s) present in the live index. Empty input returns an empty
// filter (whole corpus). A substring matching no document is an error that
// lists the indexed documents; one matching several is an error listing the
// candidates — the user must disambiguate.
func (a *App) resolveDocFilter(ctx context.Context, docFilter string) (indexingmodel.MetadataFilter, error) {
	if strings.TrimSpace(docFilter) == "" {
		return indexingmodel.MetadataFilter{}, nil
	}

	docs, err := a.listDocuments(ctx)
	if err != nil {
		return indexingmodel.MetadataFilter{}, err
	}

	needle := strings.ToLower(strings.TrimSpace(docFilter))
	var matched []string
	for _, d := range docs {
		if strings.Contains(strings.ToLower(d.DocumentID), needle) {
			matched = append(matched, d.DocumentID)
		}
	}

	switch len(matched) {
	case 0:
		names := make([]string, 0, len(docs))
		for _, d := range docs {
			names = append(names, d.DocumentID)
		}
		return indexingmodel.MetadataFilter{}, fmt.Errorf(
			"no indexed document matches %q (indexed: %s)", docFilter, strings.Join(names, ", "))
	case 1:
		return indexingmodel.MetadataFilter{DocumentIDs: matched}, nil
	default:
		return indexingmodel.MetadataFilter{}, fmt.Errorf(
			"%d documents match %q: %s — use a more specific substring", len(matched), docFilter, strings.Join(matched, ", "))
	}
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
