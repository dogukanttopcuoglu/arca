package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"arca/internal/eval"
	"arca/internal/eval/probe"
	indexingmodel "arca/internal/indexing/model"
	"arca/internal/qa"
	qacontext "arca/internal/qa/context"
	"arca/internal/retrieval/dense"
	"arca/internal/retrieval/graphfusion"
	retrievalseam "arca/internal/retrieval/seam"
	"arca/internal/retrieval/rerank"
)

// ProbeCollectOptions configures M8 candidate artifact generation
// (ADR-0045): the unchanged production baseline records candidate lists.
type ProbeCollectOptions struct {
	GoldSetPath   string
	ArtifactPath  string
	CandidateTopN int
}

// ProbeRunOptions configures the M8 probe simulation (ADR-0045): rerankers
// over the recorded artifact, the kill gate, and the frozen operational
// budget.
type ProbeRunOptions struct {
	ArtifactPath    string
	GoldSetPath     string
	BGECommand      string
	CandidateNs     []int
	MaxP95Ms        int64
	MaxRSSBytes     int64
	ReportPath      string
	M5Gate          bool
	// Structure enables the deterministic structure-bonus reranker
	// (research E2): model-free heading-overlap reordering. StructureIntents
	// gates it to the given intent set (comma-separated); empty = all.
	Structure        bool
	StructureIntents string
}

// RunProbeCollect generates the candidate artifact from the production
// baseline (GraphFusionRetriever, w=1.0): one run, fingerprint-verified,
// candidates recorded at the top N for later reranker simulation.
func (a *App) RunProbeCollect(ctx context.Context, opts ProbeCollectOptions) (string, error) {
	if strings.TrimSpace(opts.GoldSetPath) == "" {
		return "", fmt.Errorf("gold set path cannot be empty")
	}
	if strings.TrimSpace(opts.ArtifactPath) == "" {
		return "", fmt.Errorf("artifact path cannot be empty")
	}
	if opts.CandidateTopN <= 0 {
		opts.CandidateTopN = 100
	}

	retriever, err := a.productionFusionRetriever()
	if err != nil {
		return "", err
	}

	gs, err := loadGoldSetFile(opts.GoldSetPath)
	if err != nil {
		return "", err
	}

	art, err := eval.CollectCandidateArtifact(
		ctx,
		retriever,
		listPointsSource{store: a.runtime.vectorStore},
		gs,
		eval.Options{
			Mode:              retrievalseam.RetrievalDense,
			TopK:              5,
			MinScore:          a.retrievalMinScore,
			EmbeddingProvider: a.runtime.embeddingProvider.Provider(),
			EmbeddingModel:    a.runtime.embeddingProvider.Model(),
			Collection:        a.runtime.cfg.QdrantCollection,
			GitCommit:         gitHead(),
			GraphWeight:       1.0,
			GraphFusionConfig: &graphfusion.GraphFusionConfig{DenseWeight: 1.0, GraphWeight: 1.0},
		},
		opts.CandidateTopN,
	)
	if err != nil {
		return "", err
	}

	raw, err := json.MarshalIndent(art, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to serialize artifact: %w", err)
	}
	if err := os.WriteFile(opts.ArtifactPath, raw, 0644); err != nil {
		return "", fmt.Errorf("failed to write artifact: %w", err)
	}

	return fmt.Sprintf("artifact written: %s\nfingerprint: %s\ncandidate top N: %d (queries: %d)",
		opts.ArtifactPath, art.BenchmarkFingerprint, art.CandidateTopK, len(art.Queries)), nil
}

// RunProbe runs the reranker probe over a recorded artifact, evaluates the
// frozen kill gate, and writes the manifest (artifact fingerprint, budgets,
// thresholds, outcome, report).
func (a *App) RunProbe(ctx context.Context, opts ProbeRunOptions) (string, error) {
	if strings.TrimSpace(opts.ArtifactPath) == "" || strings.TrimSpace(opts.GoldSetPath) == "" {
		return "", fmt.Errorf("artifact and gold set paths are required")
	}

	raw, err := os.ReadFile(opts.ArtifactPath)
	if err != nil {
		return "", fmt.Errorf("failed to read artifact: %w", err)
	}
	var art eval.CandidateArtifact
	if err := json.Unmarshal(raw, &art); err != nil {
		return "", fmt.Errorf("invalid artifact: %w", err)
	}
	gs, err := loadGoldSetFile(opts.GoldSetPath)
	if err != nil {
		return "", err
	}

	// Model adapters are probe-side exec rerankers; each command speaks the
	// NDJSON protocol of the probe exec adapter.
	if opts.BGECommand == "" {
		return "", fmt.Errorf("no reranker command configured (--bge-command)")
	}
	execReranker := probe.NewExecReranker(parseCommand(opts.BGECommand)...)
	defer execReranker.Close()

	rerankerMap := map[string]rerank.Reranker{"bge": execReranker}
	var structureIntents []string
	if opts.Structure {
		rerankerMap["structure"] = &probe.StructureReranker{BonusAlpha: 0.5}
		for _, part := range strings.Split(opts.StructureIntents, ",") {
			if p := strings.TrimSpace(part); p != "" {
				structureIntents = append(structureIntents, p)
			}
		}
	}

	runner := probe.NewRunner(rerankerMap, probe.Options{
		Content:        a.probeContent(),
		Gate:           buildProbeGate(opts.M5Gate, a.runtime.cfg),
		GateMaxTokens:  a.runtime.cfg.LLMContextBudget,
	})

	var combos []probe.Combination
	for _, n := range opts.CandidateNs {
		combos = append(combos, probe.Combination{Model: "bge", N: n})
		if opts.Structure {
			combos = append(combos, probe.Combination{Model: "structure", N: n, Intents: structureIntents})
		}
	}
	if len(combos) == 0 {
		return "", fmt.Errorf("no candidate budgets configured (--n)")
	}

	rep, err := runner.Run(ctx, &art, gs, combos)
	if err != nil {
		return "", err
	}

	budget := probe.Budget{MaxRerankP95Ms: opts.MaxP95Ms, MaxRSSBytes: opts.MaxRSSBytes}
	outcome := probe.Evaluate(rep, budget)

	manifest := ProbeManifest{
		ArtifactFingerprint: rep.ArtifactFingerprint,
		GoldSetVersion:      rep.GoldSetVersion,
		GitCommit:           gitHead(),
		Budget:              budget,
		MPI:                 probe.MPI,
		MARMRR:              probe.MARMRR,
		MARVerified:         probe.MARVerified,
		Outcome:             outcome,
		Report:              rep,
	}
	if opts.ReportPath != "" {
		mraw, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return "", fmt.Errorf("failed to serialize manifest: %w", err)
		}
		if err := os.WriteFile(opts.ReportPath, mraw, 0644); err != nil {
			return "", fmt.Errorf("failed to write manifest: %w", err)
		}
	}

	return renderProbeManifest(manifest), nil
}

// ProbeManifest is the M8 benchmark manifest (ADR-0045/0046): fingerprint,
// frozen thresholds and budget, the gate outcome, and the full report.
type ProbeManifest struct {
	ArtifactFingerprint string            `json:"artifact_fingerprint"`
	GoldSetVersion      string            `json:"goldset_version"`
	GitCommit           string            `json:"git_commit"`
	Budget              probe.Budget      `json:"budget"`
	MPI                 float64           `json:"mpi_ndcg5_pp"`
	MARMRR              float64           `json:"mar_mrr_pp"`
	MARVerified         float64           `json:"mar_verified_pp"`
	Outcome             probe.Outcome     `json:"outcome"`
	Report              *probe.ProbeReport `json:"report"`
}

// productionFusionRetriever builds the M7 production retrieval path: dense
// base fused with the entity graph at the frozen weight 1.0 (ADR-0041).
func (a *App) productionFusionRetriever() (retrievalseam.Retriever, error) {
	denseRet, err := a.runtime.RetrieverForMode(retrievalseam.RetrievalDense)
	if err != nil {
		return nil, fmt.Errorf("dense retriever unavailable: %w", err)
	}
	d, ok := denseRet.(*dense.DenseRetriever)
	if !ok {
		return nil, fmt.Errorf("graph fusion requires a dense base retriever, got %T", denseRet)
	}
	graphRet, err := a.runtime.GraphRetriever()
	if err != nil {
		return nil, fmt.Errorf("graph retriever unavailable: %w", err)
	}
	return graphfusion.NewGraphFusionRetriever(d, graphRet, graphfusion.GraphFusionConfig{
		DenseWeight: 1.0,
		GraphWeight: 1.0,
	}), nil
}

// probeContent resolves chunk content from the vector store payload
// (probe-only path: the artifact records IDs only; the production ContentStore
// is process-local and empty in a fresh probe process — the payload is the
// durable content source, mirroring the M7 GraphRetriever fix).
func (a *App) probeContent() func(ctx context.Context, ids []string) ([]string, error) {
	return func(ctx context.Context, ids []string) ([]string, error) {
		points, err := a.runtime.vectorStore.ListPoints(ctx, indexingmodel.MetadataFilter{ChunkIDs: ids})
		if err != nil {
			return nil, err
		}
		byID := make(map[string]string, len(points))
		for _, pt := range points {
			byID[pt.Metadata.ChunkID] = pt.ContentMarkdown
		}
		out := make([]string, len(ids))
		for i, id := range ids {
			out[i] = byID[id]
		}
		return out, nil
	}
}

// parseCommand splits a command string into argv (quoted segments supported
// minimally by splitting on spaces).
func parseCommand(s string) []string {
	return strings.Fields(s)
}

// buildProbeGate adapts the M5 EvidenceGate adapter to the probe's gate
// signature: supported -> true, unsupported -> false, failures surface as
// errors. Disabled gates return nil.
func buildProbeGate(enabled bool, cfg Config) func(ctx context.Context, query, content string) (bool, error) {
	if !enabled {
		return nil
	}
	gate := qa.NewLLMEvidenceGate(buildLLMProvider(cfg))
	return func(ctx context.Context, query, content string) (bool, error) {
		decision, err := gate.Evaluate(ctx, query, &qacontext.ContextWindow{Content: content})
		if err != nil {
			return false, err
		}
		if decision == qa.EvidenceGateFailed {
			return false, fmt.Errorf("evidence gate failed for query %q", query)
		}
		return decision == qa.EvidenceSupported, nil
	}
}

func loadGoldSetFile(path string) (*eval.GoldSet, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open gold set: %w", err)
	}
	defer f.Close()
	gs, err := eval.LoadGoldSet(f)
	if err != nil {
		return nil, fmt.Errorf("invalid gold set: %w", err)
	}
	return gs, nil
}

func renderProbeManifest(m ProbeManifest) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "=== ARC M8 RERANK PROBE ===\n")
	fmt.Fprintf(&sb, "fingerprint: %s (gold set %s, commit %s)\n", m.ArtifactFingerprint, m.GoldSetVersion, m.GitCommit)
	fmt.Fprintf(&sb, "budget: p95 rerank <= %d ms, rss <= %d bytes\n", m.Budget.MaxRerankP95Ms, m.Budget.MaxRSSBytes)
	fmt.Fprintf(&sb, "thresholds: MPI nDCG@5 >= +%.1fpp, MAR mrr >= -%.1fpp, verified >= -%.1fpp\n", m.MPI, m.MARMRR, m.MARVerified)
	fmt.Fprintf(&sb, "\nbaseline: recall@5 %.3f, nDCG@5 %.3f, MRR %.3f, verified %.3f\n", m.Report.Baseline.RecallAt5, m.Report.Baseline.NDCGAt5, m.Report.Baseline.MRR, m.Report.Baseline.VerifiedRate)
	sb.WriteString("combinations:\n")
	fmt.Fprintf(&sb, "  %-8s %-5s %-9s %-9s %-8s %-9s %-9s %-6s %-8s\n", "model", "N", "recall@5", "ndcg@5", "mrr", "p95_ms", "rss_mb", "verif", "ci_med")
	for _, c := range m.Report.Combinations {
		ci := ""
		if c.BootstrapCI != nil {
			ci = fmt.Sprintf("%+.2f", c.BootstrapCI.DeltaMedianPp)
		}
		fmt.Fprintf(&sb, "  %-8s %-5d %-9.3f %-9.3f %-8.3f %-9.1f %-8.1f %-6.3f %s\n",
			c.Model, c.CandidateN, c.RecallAt5, c.NDCGAt5, c.MRR, c.P95LatencyMs, float64(c.MaxRSSBytes)/1024/1024, c.VerifiedRate, ci)
	}
	sb.WriteString("\n")
	if m.Outcome.Accepted {
		fmt.Fprintf(&sb, "VERDICT: ACCEPT — %s (model=%s, N=%d)\n", m.Outcome.Reason, m.Outcome.SelectedModel, m.Outcome.SelectedN)
	} else {
		fmt.Fprintf(&sb, "VERDICT: REJECT — %s (production path unchanged)\n", m.Outcome.Reason)
	}
	return sb.String()
}
