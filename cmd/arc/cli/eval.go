package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"arca/internal/eval"
	indexingmodel "arca/internal/indexing/model"
	"arca/internal/indexing/store"
	"arca/internal/qa"
	"arca/internal/retrieval/dense"
	"arca/internal/retrieval/graphfusion"
	"arca/internal/retrieval/hybrid"
	retrievalseam "arca/internal/retrieval/seam"
)

// EvalOptions configures an arc eval benchmark run.
type EvalOptions struct {
	GoldSetPath      string
	Mode             retrievalseam.RetrievalMode
	TopK             int
	MinScore         float32
	ReportPath       string
	FusionPolicyName string
	SparseWeight     float64
	SparseCap        int
	Decompose        bool
	// M5Gate enables the pre-generation semantic evidence gate over each
	// query's assembled context, producing the M5 report section.
	M5Gate bool
	// GateRuns repeats each gate evaluation (default 1) and records the
	// median decision, stabilizing gate metrics against LLM variance.
	GateRuns int
	// ComparisonTopK is the M6 evidence budget for comparison queries
	// (ADR-0037), applied by the runner for calibration runs.
	ComparisonTopK int
	// GraphWeight is the M7 graph fusion weight (ADR-0041): >0 fuses the
	// dense and graph streams with the given weight; 0 keeps the mode's
	// default retriever.
	GraphWeight float64
	// GraphOnly measures the graph stream alone (kill-gate graphA
	// counterpart); it wins over GraphWeight.
	GraphOnly bool
}

// RunEval executes the retrieval benchmark against the real composition root:
// real embedding provider and real vector store — never mocks. The corpus
// fingerprint is verified before any query executes; a mismatch aborts with
// an error. Renders a human table and optionally writes the JSON report.
func (a *App) RunEval(ctx context.Context, opts EvalOptions) (string, error) {
	if strings.TrimSpace(opts.GoldSetPath) == "" {
		return "", fmt.Errorf("gold set path cannot be empty")
	}
	if opts.TopK <= 0 {
		opts.TopK = 5
	}

	retriever, err := a.runtime.RetrieverForMode(opts.Mode)
	if err != nil {
		return "", fmt.Errorf("retrieval mode %q unavailable: %w", opts.Mode, err)
	}

	// M7 graph surface (ADR-0041): --graph-only measures the graph stream
	// alone; --graph-weight > 0 fuses the dense and graph streams. Both
	// require the entity-only graph retriever; with neither, the mode's
	// default retriever is used unchanged.
	var graphFusionConfig *graphfusion.GraphFusionConfig
	if opts.GraphOnly || opts.GraphWeight > 0 {
		graphRet, err := a.runtime.GraphRetriever()
		if err != nil {
			return "", fmt.Errorf("graph retriever unavailable: %w", err)
		}
		switch {
		case opts.GraphOnly:
			retriever = graphRet
		default:
			denseRet, ok := retriever.(*dense.DenseRetriever)
			if !ok {
				return "", fmt.Errorf("graph fusion requires a dense base retriever, got %T", retriever)
			}
			graphFusionConfig = &graphfusion.GraphFusionConfig{
				DenseWeight: 1.0,
				GraphWeight: opts.GraphWeight,
			}
			retriever = graphfusion.NewGraphFusionRetriever(denseRet, graphRet, *graphFusionConfig)
		}
	}

	// Apply the fusion policy for hybrid sweeps. A named policy sets the
	// frozen base; raw flags override fields on top. The retriever owns the
	// policy; eval only records it in the manifest.
	var fusionPolicy *hybrid.FusionPolicy
	if opts.Mode == retrievalseam.RetrievalHybrid {
		hr, ok := retriever.(*hybrid.HybridRetriever)
		if !ok {
			return "", fmt.Errorf("hybrid mode requires the hybrid retriever, got %T", retriever)
		}
		p := hr.FusionPolicy()
		if opts.FusionPolicyName != "" {
			named, err := hybrid.PolicyByName(opts.FusionPolicyName)
			if err != nil {
				return "", fmt.Errorf("invalid fusion policy: %w", err)
			}
			p = named
		}
		if opts.SparseWeight > 0 {
			p.SparseWeight = opts.SparseWeight
		}
		if opts.SparseCap > 0 {
			p.SparseCap = opts.SparseCap
		}
		hr.SetFusionPolicy(p)
		fusionPolicy = &p
	}

	gsFile, err := os.Open(opts.GoldSetPath)
	if err != nil {
		return "", fmt.Errorf("failed to open gold set: %w", err)
	}
	defer gsFile.Close()
	gs, err := eval.LoadGoldSet(gsFile)
	if err != nil {
		return "", fmt.Errorf("invalid gold set: %w", err)
	}

	runner := eval.New(
		retriever,
		listPointsSource{store: a.runtime.vectorStore},
		eval.Options{
			Mode:              opts.Mode,
			TopK:              opts.TopK,
			MinScore:          opts.MinScore,
			EmbeddingProvider: a.runtime.embeddingProvider.Provider(),
			EmbeddingModel:    a.runtime.embeddingProvider.Model(),
			FusionPolicy:      fusionPolicy,
			Collection:        a.runtime.cfg.QdrantCollection,
			GitCommit:         gitHead(),
			Decompose:         decomposeFunc(opts.Decompose),
			Gate:              m5GateFunc(opts, a.runtime.cfg),
			GateProvider:      a.runtime.cfg.LLMProviderLabel,
			GateModel:         a.runtime.cfg.LLMModel,
			ComparisonTopK:    opts.ComparisonTopK,
			GraphWeight:       opts.GraphWeight,
			GraphOnly:         opts.GraphOnly,
			GraphFusionConfig: graphFusionConfig,
			GateRuns:          opts.GateRuns,
			CorpusTexts: func() ([]string, error) {
				return corpusSource{store: a.runtime.vectorStore}.CorpusTexts(context.Background())
			},
		},
	)

	report, err := runner.Run(ctx, gs)
	if err != nil {
		return "", err
	}

	if opts.ReportPath != "" {
		raw, err := report.JSON()
		if err != nil {
			return "", fmt.Errorf("failed to serialize report: %w", err)
		}
		if err := os.WriteFile(opts.ReportPath, raw, 0644); err != nil {
			return "", fmt.Errorf("failed to write report: %w", err)
		}
	}

	return renderReport(report), nil
}

// listPointsSource implements eval.FingerprintSource over the real vector
// store, reading content hashes from the indexed points.
type listPointsSource struct {
	store store.VectorStore
}

// ContentHashes returns the ContentHash of every indexed chunk for the
// document via ListPoints.
func (s listPointsSource) ContentHashes(documentID string) ([]string, error) {
	points, err := s.store.ListPoints(context.Background(), indexingmodel.MetadataFilter{DocumentIDs: []string{documentID}})
	if err != nil {
		return nil, err
	}
	hashes := make([]string, len(points))
	for i, p := range points {
		hashes[i] = p.Metadata.ContentHash
	}
	return hashes, nil
}

// decomposeFunc wires the rule-based analyzer's deterministic decomposition
// into the eval runner when enabled; nil otherwise.
func decomposeFunc(enabled bool) func(string) []string {
	if !enabled {
		return nil
	}
	analyzer := qa.NewRuleBasedAnalyzer()
	return func(query string) []string {
		analyzed, err := analyzer.Analyze(context.Background(), query)
		if err != nil || analyzed == nil {
			return nil
		}
		return analyzed.SubQueries
	}
}

// m5GateFunc returns the real EvidenceGate adapter when the M5 gate flag is
// enabled; nil otherwise. The gate shares the configured LLM provider, so the
// benchmark measures the production adapter.
func m5GateFunc(opts EvalOptions, cfg Config) qa.EvidenceGate {
	if !opts.M5Gate {
		return nil
	}
	return qa.NewLLMEvidenceGate(buildLLMProvider(cfg))
}

// gitHead returns the current git commit hash, or "unknown" when unavailable.
func gitHead() string {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// renderReport formats the benchmark report as a human-readable table.
func renderReport(report *eval.Report) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "=== ARC EVAL — %s ===\n", report.Retrieval.Mode)
	fmt.Fprintf(&sb, "corpus: %s (%d chunks, fingerprint %.16s…)\n",
		report.Corpus.DocumentID, report.Corpus.ChunkCount, report.Corpus.Fingerprint)
	fmt.Fprintf(&sb, "config: mode=%s top_k=%d min_score=%v model=%s/%s commit=%s\n\n",
		report.Retrieval.Mode, report.Retrieval.TopK, report.Retrieval.MinScore,
		report.Retrieval.EmbeddingProvider, report.Retrieval.EmbeddingModel, report.GitCommit)

	fmt.Fprintf(&sb, "%-8s %-12s %-9s %-11s %-6s %-8s %s\n",
		"id", "intent", "recall@k", "precision@k", "mrr", "ndcg@k", "retrieved")
	for _, q := range report.PerQuery {
		fmt.Fprintf(&sb, "%-8s %-12s %-9.3f %-11.3f %-6.3f %-8.3f %d\n",
			q.ID, q.Intent, q.RecallAtK, q.PrecisionAtK, q.MRR, q.NDCGAtK, len(q.RetrievedChunkIDs))
	}

	sb.WriteString("\nAGGREGATE\n")
	fmt.Fprintf(&sb, "  recall@5            %.3f\n", report.Metrics.RecallAtK)
	fmt.Fprintf(&sb, "  precision@5         %.3f\n", report.Metrics.PrecisionAtK)
	fmt.Fprintf(&sb, "  mrr                 %.3f\n", report.Metrics.MRR)
	fmt.Fprintf(&sb, "  ndcg@5              %.3f\n", report.Metrics.NDCGAtK)
	fmt.Fprintf(&sb, "  no_evidence_precision %.3f\n", report.Metrics.NoEvidencePrecision)
	fmt.Fprintf(&sb, "  queries             %d (%.0f ms)\n", report.Metrics.Queries, float64(report.DurationMs))
	return sb.String()
}
