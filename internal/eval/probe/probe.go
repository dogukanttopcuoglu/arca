// Package probe implements the M8 reranking probe (ADR-0043...0046): it
// re-runs rerankers over the recorded candidate artifact, computes ranking
// metrics per (model, candidate-N) combination, and verifies abstention
// alignment. It is benchmark tooling only — production code never imports it.
package probe

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
	"time"

	"arca/internal/eval"
	retrievalseam "arca/internal/retrieval/seam"
	"arca/internal/retrieval/rerank"
)

// Combination is one probe measurement: a reranker model applied to the
// candidate top N (the first N candidates of each recorded artifact list).
type Combination struct {
	Model string
	N     int
}

// Options wires optional end-to-end measurement: content resolution for the
// gate and rerankers (from the vector store) and the gate itself. Both or
// neither must be set; without them the probe measures ranking only.
type Options struct {
	// Content resolves one content string per chunk ID (probe-only path:
	// the artifact records IDs, not content).
	Content func(ctx context.Context, ids []string) ([]string, error)
	// Gate evaluates the M5 semantic evidence gate over reranked content.
	Gate func(ctx context.Context, query, content string) (bool, error)
}

// Runner executes the probe over a recorded candidate artifact.
type Runner struct {
	rerankers map[string]rerank.Reranker
	options   Options
}

// NewRunner constructs the probe runner. rerankers maps model names (as
// recorded in the artifact config) to their adapters.
func NewRunner(rerankers map[string]rerank.Reranker, opts Options) *Runner {
	return &Runner{rerankers: rerankers, options: opts}
}

// BaselineResult is the production baseline (candidate list top-K, no
// rerank) — the delta reference for every combination (ADR-0045). The gate
// fields measure the baseline's answer quality when a gate is wired.
type BaselineResult struct {
	NDCGAt5         float64 `json:"ndcg_at_5"`
	MRR             float64 `json:"mrr"`
	GateEvaluations int     `json:"gate_evaluations,omitempty"`
	VerifiedRate    float64 `json:"verified_rate,omitempty"`
}

// CIRange is a report-only bootstrap confidence interval over the per-query
// nDCG delta (percentage points) of one combination (ADR-0045: reported,
// never a decision criterion).
type CIRange struct {
	DeltaMedianPp float64 `json:"delta_median_pp"`
	LowerPp       float64 `json:"lower_pp"`
	UpperPp       float64 `json:"upper_pp"`
}

// CombinationResult is one (model, N) measurement (ADR-0045).
type CombinationResult struct {
	Model             string              `json:"model"`
	CandidateN        int                 `json:"candidate_n"`
	NDCGAt5           float64             `json:"ndcg_at_5"`
	MRR               float64             `json:"mrr"`
	RerankedQueries   int                 `json:"reranked_queries"`
	AbstentionAligned bool                `json:"abstention_aligned"`
	P50LatencyMs      float64             `json:"p50_latency_ms"`
	P95LatencyMs      float64             `json:"p95_latency_ms"`
	ColdLoadMs        int64               `json:"cold_load_ms,omitempty"`
	MaxRSSBytes       int64               `json:"max_rss_bytes,omitempty"`
	BootstrapCI       *CIRange            `json:"bootstrap_ci,omitempty"`
	RerankerOrdering  map[string][]string `json:"reranker_ordering,omitempty"`
	GateEvaluations   int                 `json:"gate_evaluations,omitempty"`
	VerifiedRate      float64             `json:"verified_rate,omitempty"`
}

// ProbeReport is the full probe outcome (ADR-0045): baseline plus every
// combination, carrying the artifact fingerprint for traceability.
type ProbeReport struct {
	ArtifactFingerprint string              `json:"artifact_fingerprint"`
	GoldSetVersion      string              `json:"goldset_version"`
	CandidateTopK       int                 `json:"candidate_top_n"`
	TopK                int                 `json:"top_k"`
	Baseline            BaselineResult      `json:"baseline"`
	Combinations        []CombinationResult `json:"combinations"`
}

// Run verifies the artifact against the gold set, evaluates the baseline
// from the recorded candidates, and evaluates every combination. It fails
// before any measurement when the artifact and gold set disagree (query
// sets, fingerprint) — a stale artifact never feeds a benchmark.
func (r *Runner) Run(ctx context.Context, art *eval.CandidateArtifact, gs *eval.GoldSet, combos []Combination) (*ProbeReport, error) {
	if err := r.validate(art, gs); err != nil {
		return nil, err
	}
	topK := art.RetrievalConfig.TopK
	if topK <= 0 {
		topK = 5
	}

	rep := &ProbeReport{
		ArtifactFingerprint: art.BenchmarkFingerprint,
		GoldSetVersion:      art.GoldSetVersion,
		CandidateTopK:       art.CandidateTopK,
		TopK:                topK,
	}

	byID := make(map[string]*eval.ArtifactQuery, len(art.Queries))
	for i := range art.Queries {
		byID[art.Queries[i].QueryID] = &art.Queries[i]
	}

	// Baseline: the recorded candidate list top-K, no rerank. Per-query
	// nDCG values feed the bootstrap delta of every combination.
	var baselineNDCG []float64
	var baselineN, baselineM, verified float64
	baseCount := 0
	for _, q := range gs.Queries {
		aq := byID[q.ID]
		if aq.ExpectedNoEvidence {
			continue
		}
		top := sliceString(aq.Candidates, topK)
		ndcg := eval.NDCGAtK(top, q.ExpectedChunkIDs, topK)
		baselineN += ndcg
		baselineNDCG = append(baselineNDCG, ndcg)
		baselineM += eval.MRR(top, q.ExpectedChunkIDs)
		baseCount++

		if r.options.Gate != nil && r.options.Content != nil {
			ok, err := r.evaluateGate(ctx, q.Query, aq, top)
			if err != nil {
				return nil, err
			}
			rep.Baseline.GateEvaluations++
			if ok {
				verified++
			}
		}
	}
	if baseCount > 0 {
		rep.Baseline.NDCGAt5 = baselineN / float64(baseCount)
		rep.Baseline.MRR = baselineM / float64(baseCount)
	}
	if rep.Baseline.GateEvaluations > 0 {
		rep.Baseline.VerifiedRate = verified / float64(rep.Baseline.GateEvaluations)
	}

	for _, combo := range combos {
		if combo.N > art.CandidateTopK {
			return nil, fmt.Errorf("combination %s N=%d exceeds the artifact's candidate top N %d — re-collect the artifact (probe review C2)", combo.Model, combo.N, art.CandidateTopK)
		}
		rr, ok := r.rerankers[combo.Model]
		if !ok {
			return nil, fmt.Errorf("reranker %q is not registered", combo.Model)
		}
		res, err := r.evaluateCombination(ctx, rr, byID, gs, combo, topK, baselineNDCG)
		if err != nil {
			return nil, fmt.Errorf("combination %s N=%d failed: %w", combo.Model, combo.N, err)
		}
		rep.Combinations = append(rep.Combinations, res)
	}

	return rep, nil
}

// evaluateCombination reranks each query's first-N candidates, computes
// ranking metrics, the report-only bootstrap CI, and optionally the gate
// over the reranked content. Abstention queries never reach the reranker;
// the artifact's empty candidate list must stay empty (hard invariant,
// ADR-0045).
func (r *Runner) evaluateCombination(
	ctx context.Context,
	rr rerank.Reranker,
	byID map[string]*eval.ArtifactQuery,
	gs *eval.GoldSet,
	combo Combination,
	topK int,
	baselineNDCG []float64,
) (CombinationResult, error) {
	res := CombinationResult{Model: combo.Model, CandidateN: combo.N, AbstentionAligned: true, RerankerOrdering: map[string][]string{}}
	var nSum, mSum, verified float64
	count := 0
	var latencies []int64
	var deltas []float64

	for _, q := range gs.Queries {
		aq := byID[q.ID]

		if q.ExpectedNoEvidence {
			if len(aq.Candidates) != 0 {
				res.AbstentionAligned = false
			}
			continue
		}

		candidates := make([]retrievalseam.SearchResult, 0, combo.N)
		for _, id := range sliceString(aq.Candidates, combo.N) {
			candidates = append(candidates, retrievalseam.SearchResult{ChunkID: id, Score: 1.0})
		}

		var rerankedIDs []string
		if len(candidates) > 0 {
			// Rerankers score (query, document) pairs, so candidates carry
			// their content when a content provider is wired; ranking-only
			// probes run with empty content.
			if r.options.Content != nil {
				ids := make([]string, len(candidates))
				for i, c := range candidates {
					ids[i] = c.ChunkID
				}
				contents, err := r.options.Content(ctx, ids)
				if err != nil {
					return res, fmt.Errorf("content resolution failed: %w", err)
				}
				if len(contents) != len(candidates) {
					return res, fmt.Errorf("content provider returned %d entries for %d ids", len(contents), len(candidates))
				}
				for i := range candidates {
					candidates[i].ContentMarkdown = contents[i]
				}
			}

			start := time.Now()
			ordered, err := rr.Rerank(ctx, q.Query, candidates)
			latencies = append(latencies, time.Since(start).Milliseconds())
			if err != nil {
				return res, err
			}

			// Single enforcement point of the ADR-0044 tie-break contract,
			// shared with the production wrapper.
			stabilized := rerank.StabilizeOrdering(candidates, ordered, combo.N)
			rerankedIDs = make([]string, 0, len(stabilized))
			for _, c := range stabilized {
				rerankedIDs = append(rerankedIDs, c.ChunkID)
			}
		}
		res.RerankerOrdering[q.ID] = rerankedIDs

		top := sliceString(rerankedIDs, topK)
		ndcg := eval.NDCGAtK(top, q.ExpectedChunkIDs, topK)
		nSum += ndcg
		mSum += eval.MRR(top, q.ExpectedChunkIDs)
		count++
		res.RerankedQueries++
		// Pair the per-query delta against the same query's baseline nDCG:
		// both loops iterate the same non-abstention queries in gold set
		// order, so count-1 stays aligned even for empty-candidate queries
		// (which contribute a zero nDCG).
		if count-1 < len(baselineNDCG) {
			deltas = append(deltas, (ndcg-baselineNDCG[count-1])*100)
		}

		if r.options.Gate != nil && r.options.Content != nil {
			ok, err := r.evaluateGate(ctx, q.Query, aq, top)
			if err != nil {
				return res, err
			}
			res.GateEvaluations++
			if ok {
				verified++
			}
		}
	}

	if count > 0 {
		res.NDCGAt5 = nSum / float64(count)
		res.MRR = mSum / float64(count)
	}
	if res.GateEvaluations > 0 {
		res.VerifiedRate = verified / float64(res.GateEvaluations)
	}
	if len(latencies) > 0 {
		sorted := append([]int64(nil), latencies...)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
		res.P50LatencyMs = float64(percentile(sorted, 50))
		res.P95LatencyMs = float64(percentile(sorted, 95))
	}
	if len(deltas) > 0 {
		res.BootstrapCI = bootstrapCI(deltas)
	}
	if rep, ok := rr.(loadReporter); ok {
		res.MaxRSSBytes = rep.RSSBytes()
		res.ColdLoadMs = rep.LoadTimeMs()
	}

	return res, nil
}

// loadReporter is the minimal interface the probe uses to surface model
// operational data from exec adapters (memory footprint, cold load time).
type loadReporter interface {
	RSSBytes() int64
	LoadTimeMs() int64
}

// evaluateGate runs the gate over the given top-K chunk IDs' content,
// resolving content through the candidate map of the artifact query.
func (r *Runner) evaluateGate(ctx context.Context, query string, aq *eval.ArtifactQuery, top []string) (bool, error) {
	contents, err := r.options.Content(ctx, top)
	if err != nil {
		return false, fmt.Errorf("content resolution failed: %w", err)
	}
	if len(contents) != len(top) {
		return false, fmt.Errorf("content provider returned %d entries for %d ids", len(contents), len(top))
	}
	var sb strings.Builder
	for _, c := range contents {
		sb.WriteString(c)
		sb.WriteString(" ")
	}
	supported, err := r.options.Gate(ctx, query, sb.String())
	if err != nil {
		return false, fmt.Errorf("gate evaluation failed: %w", err)
	}
	return supported, nil
}

// bootstrapCI computes a report-only 95% CI over per-query nDCG deltas
// (percentage points) with a fixed seed: 10,000 resamples, median and 2.5/97.5
// percentiles. Deterministic across runs (ADR-0045: CI is never a decision
// criterion).
func bootstrapCI(deltas []float64) *CIRange {
	n := len(deltas)
	rng := rand.New(rand.NewSource(42))
	means := make([]float64, 10000)
	for i := range means {
		var sum float64
		for j := 0; j < n; j++ {
			sum += deltas[rng.Intn(n)]
		}
		means[i] = sum / float64(n)
	}
	sort.Float64s(means)
	at := func(p float64) float64 { return means[int(math.Ceil(p/100*float64(len(means))))-1] }
	return &CIRange{
		DeltaMedianPp: at(50),
		LowerPp:       at(2.5),
		UpperPp:       at(97.5),
	}
}

// validate checks that the artifact corresponds to the gold set: fingerprint
// matches a declared corpus fingerprint, query IDs match exactly, and the
// schema version is supported.
func (r *Runner) validate(art *eval.CandidateArtifact, gs *eval.GoldSet) error {
	if art.SchemaVersion != eval.CandidateArtifactSchemaVersion {
		return fmt.Errorf("artifact schema version %d, want %d", art.SchemaVersion, eval.CandidateArtifactSchemaVersion)
	}
	if !declaresFingerprint(gs, art.BenchmarkFingerprint) {
		return fmt.Errorf("artifact fingerprint %q does not match the gold set", art.BenchmarkFingerprint)
	}
	if len(art.Queries) != len(gs.Queries) {
		return fmt.Errorf("artifact has %d queries, gold set has %d", len(art.Queries), len(gs.Queries))
	}
	byID := make(map[string]bool, len(gs.Queries))
	for _, q := range gs.Queries {
		byID[q.ID] = true
	}
	for _, aq := range art.Queries {
		if !byID[aq.QueryID] {
			return fmt.Errorf("artifact query %q is not in the gold set", aq.QueryID)
		}
	}
	return nil
}

// declaresFingerprint reports whether the gold set declares the given
// fingerprint for its corpus (single- or multi-document).
func declaresFingerprint(gs *eval.GoldSet, fp string) bool {
	if len(gs.Documents) > 0 {
		for _, d := range gs.Documents {
			if d.CorpusFingerprint == fp {
				return true
			}
		}
		return false
	}
	return gs.Corpus.CorpusFingerprint == fp
}

// sliceString returns the first n elements of a string slice, or the whole
// slice when shorter.
func sliceString(s []string, n int) []string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n]
}

// percentile returns the p-th percentile of a sorted duration slice, using
// the ceil index (deterministic; single-value input returns that value).
func percentile(sorted []int64, p int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(float64(p)/100*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
