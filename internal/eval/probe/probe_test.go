package probe

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"arca/internal/eval"
	retrievalseam "arca/internal/retrieval/seam"
	"arca/internal/retrieval/rerank"
)

// recordingReranker reverses the candidate list, records how many candidates
// it received per query, and can fail.
type recordingReranker struct {
	calls [][]string
	fail  bool
	delay time.Duration
}

func (f *recordingReranker) Rerank(ctx context.Context, query string, candidates []retrievalseam.SearchResult) ([]rerank.ScoredCandidate, error) {
	ids := make([]string, len(candidates))
	for i, c := range candidates {
		ids[i] = c.ChunkID
	}
	f.calls = append(f.calls, ids)
	if f.fail {
		return nil, errors.New("model down")
	}
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	out := make([]rerank.ScoredCandidate, len(candidates))
	for i, c := range candidates {
		out[len(candidates)-1-i] = rerank.ScoredCandidate{ChunkID: c.ChunkID, Score: float32(i)}
	}
	return out, nil
}

func probeArtifact(t *testing.T) (*eval.CandidateArtifact, *eval.GoldSet) {
	t.Helper()
	gs, err := eval.LoadGoldSet(strings.NewReader(`{
		"schema_version": "1.2",
		"documents": [
			{"document_id": "book-a", "corpus_fingerprint": "fp-a", "chunk_count": 3}
		],
		"queries": [
			{"id": "q1", "intent": "entity", "query": "who founded the company", "expected_chunk_ids": ["c1"]},
			{"id": "q2", "intent": "entity", "query": "second question", "expected_chunk_ids": ["c2"]},
			{"id": "q3", "intent": "abstention", "query": "no such topic here", "expected_no_evidence": true}
		]
	}`))
	if err != nil {
		t.Fatalf("LoadGoldSet: %v", err)
	}
	art := &eval.CandidateArtifact{
		SchemaVersion:        eval.CandidateArtifactSchemaVersion,
		BenchmarkFingerprint: "fp-a",
		GoldSetVersion:       "1.2",
		CandidateTopK:        5,
		RetrievalConfig:      eval.RetrievalConfig{TopK: 5},
		Queries: []eval.ArtifactQuery{
			{QueryID: "q1", Query: "who founded the company", Intent: "entity",
				Candidates: []string{"c1", "c2", "c3"}, CandidateScores: []float32{1, 0.9, 0.8}},
			{QueryID: "q2", Query: "second question", Intent: "entity",
				Candidates: []string{"c2", "c1", "c3"}, CandidateScores: []float32{1, 0.9, 0.8}},
			{QueryID: "q3", Query: "no such topic here", Intent: "abstention", ExpectedNoEvidence: true},
		},
	}
	return art, gs
}

func combos() []Combination {
	return []Combination{
		{Model: "reverse", N: 5},
		{Model: "reverse", N: 2},
	}
}

func TestProbeComputesBaselineAndRerankedMetrics(t *testing.T) {
	art, gs := probeArtifact(t)
	r := NewRunner(map[string]rerank.Reranker{"reverse": &recordingReranker{}}, Options{})

	rep, err := r.Run(context.Background(), art, gs, []Combination{{Model: "reverse", N: 5}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Baseline: q1 [c1 c2 c3] -> nDCG 1.0; q2 [c2 c1 c3] -> nDCG 1.0.
	if math.Abs(rep.Baseline.NDCGAt5-1.0) > 1e-9 || math.Abs(rep.Baseline.MRR-1.0) > 1e-9 {
		t.Fatalf("baseline = %+v, want nDCG 1.0 / MRR 1.0", rep.Baseline)
	}

	// Reranked (reversed): q1 [c3 c2 c1] -> nDCG 1/log2(4) = 0.5, MRR 1/3;
	// q2 [c3 c1 c2] -> nDCG 1/log2(4) = 0.5, MRR 1/3.
	c := rep.Combinations[0]
	wantNDCG := 0.5
	wantMRR := 1.0 / 3
	if math.Abs(c.NDCGAt5-wantNDCG) > 1e-3 || math.Abs(c.MRR-wantMRR) > 1e-3 {
		t.Fatalf("reranked = nDCG %.4f / MRR %.4f, want %.4f / %.4f", c.NDCGAt5, c.MRR, wantNDCG, wantMRR)
	}
	if c.RerankedQueries != 2 {
		t.Fatalf("reranked queries = %d, want 2 (abstention excluded)", c.RerankedQueries)
	}
	if !c.AbstentionAligned {
		t.Fatal("abstention alignment must hold: q3 has empty candidates")
	}
}

func TestProbeSlicesCandidatesByN(t *testing.T) {
	art, gs := probeArtifact(t)
	fake := &recordingReranker{}
	r := NewRunner(map[string]rerank.Reranker{"reverse": fake}, Options{})

	if _, err := r.Run(context.Background(), art, gs, []Combination{{Model: "reverse", N: 2}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, call := range fake.calls {
		if len(call) != 2 {
			t.Fatalf("reranker received %d candidates, want N=2 slice", len(call))
		}
		if call[0] != "c1" && call[0] != "c2" {
			t.Fatalf("N=2 slice must keep the top candidates, got %v", call)
		}
	}
}

func TestProbeSkipsRerankForAbstentionQueries(t *testing.T) {
	art, gs := probeArtifact(t)
	fake := &recordingReranker{}
	r := NewRunner(map[string]rerank.Reranker{"reverse": fake}, Options{})

	if _, err := r.Run(context.Background(), art, gs, []Combination{{Model: "reverse", N: 5}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// q3 (abstention) must never reach the reranker.
	if len(fake.calls) != 2 {
		t.Fatalf("reranker called %d times, want 2 (no abstention query)", len(fake.calls))
	}
}

func TestProbeReportsLatencyPercentiles(t *testing.T) {
	art, gs := probeArtifact(t)
	r := NewRunner(map[string]rerank.Reranker{"reverse": &recordingReranker{delay: 3 * time.Millisecond}}, Options{})

	rep, err := r.Run(context.Background(), art, gs, []Combination{{Model: "reverse", N: 5}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	c := rep.Combinations[0]
	if c.P50LatencyMs < 1 || c.P95LatencyMs < 1 {
		t.Fatalf("latency percentiles = p50 %.2f / p95 %.2f, want > 0", c.P50LatencyMs, c.P95LatencyMs)
	}
}

func TestProbeFailsOnArtifactGoldSetMismatch(t *testing.T) {
	art, gs := probeArtifact(t)
	art.Queries = art.Queries[:2]
	r := NewRunner(map[string]rerank.Reranker{"reverse": &recordingReranker{}}, Options{})

	_, err := r.Run(context.Background(), art, gs, combos())
	if err == nil {
		t.Fatal("expected query-set mismatch error, got nil")
	}
}

func TestProbeFailsOnArtifactWithoutFingerprint(t *testing.T) {
	art, gs := probeArtifact(t)
	art.BenchmarkFingerprint = ""
	r := NewRunner(map[string]rerank.Reranker{"reverse": &recordingReranker{}}, Options{})

	_, err := r.Run(context.Background(), art, gs, combos())
	if err == nil {
		t.Fatal("expected missing-fingerprint error, got nil")
	}
}

func TestProbeRerankerFailureFailsRun(t *testing.T) {
	art, gs := probeArtifact(t)
	r := NewRunner(map[string]rerank.Reranker{"reverse": &recordingReranker{fail: true}}, Options{})

	_, err := r.Run(context.Background(), art, gs, combos())
	if err == nil {
		t.Fatal("expected reranker failure to fail the run, got nil")
	}
}

func TestProbeGateEvaluation(t *testing.T) {
	art, gs := probeArtifact(t)
	gated := 0
	r := NewRunner(map[string]rerank.Reranker{"reverse": &recordingReranker{}}, Options{
		Content: func(ctx context.Context, ids []string) ([]string, error) {
			out := make([]string, len(ids))
			for i, id := range ids {
				out[i] = id
			}
			return out, nil
		},
		Gate: func(ctx context.Context, query, content string) (bool, error) {
			gated++
			return strings.Contains(content, "c1"), nil
		},
	})

	rep, err := r.Run(context.Background(), art, gs, []Combination{{Model: "reverse", N: 5}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	c := rep.Combinations[0]
	// Baseline gate (2) + reranked gate (2) — abstention excluded from both.
	if gated != 4 {
		t.Fatalf("gate evaluated %d times, want 4 (2 baseline + 2 reranked)", gated)
	}
	if c.GateEvaluations != 2 || c.VerifiedRate != 1.0 {
		t.Fatalf("gate = %d evaluations, verified %.2f, want 2 / 1.0", c.GateEvaluations, c.VerifiedRate)
	}
	if rep.Baseline.GateEvaluations != 2 || rep.Baseline.VerifiedRate != 1.0 {
		t.Fatalf("baseline gate = %d evaluations, verified %.2f, want 2 / 1.0", rep.Baseline.GateEvaluations, rep.Baseline.VerifiedRate)
	}
}

func TestProbeReportsPerIntentSlicesAndGating(t *testing.T) {
	gs, err := eval.LoadGoldSet(strings.NewReader(`{
		"schema_version": "1.2",
		"documents": [
			{"document_id": "book-a", "corpus_fingerprint": "fp-a", "chunk_count": 2}
		],
		"queries": [
			{"id": "q1", "intent": "entity", "query": "who founded the company", "expected_chunk_ids": ["c1"]},
			{"id": "q2", "intent": "concept", "query": "what is a system", "expected_chunk_ids": ["c2"]}
		]
	}`))
	if err != nil {
		t.Fatalf("LoadGoldSet: %v", err)
	}
	art := &eval.CandidateArtifact{
		SchemaVersion:        eval.CandidateArtifactSchemaVersion,
		BenchmarkFingerprint: "fp-a",
		GoldSetVersion:       "1.2",
		CandidateTopK:        5,
		RetrievalConfig:      eval.RetrievalConfig{TopK: 5},
		Queries: []eval.ArtifactQuery{
			{QueryID: "q1", Query: "who founded the company", Intent: "entity",
				Candidates: []string{"c1", "c2"}, CandidateScores: []float32{1, 0.8}},
			{QueryID: "q2", Query: "what is a system", Intent: "concept",
				Candidates: []string{"c2", "c1"}, CandidateScores: []float32{1, 0.8}},
		},
	}
	fake := &recordingReranker{}
	r := NewRunner(map[string]rerank.Reranker{"reverse": fake}, Options{})

	rep, err := r.Run(context.Background(), art, gs, []Combination{{Model: "reverse", N: 5, Intents: []string{"entity"}}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Baseline slices: each intent's first candidate is its expected chunk.
	if s := rep.Baseline.Slices["entity"]; s.Queries != 1 || math.Abs(s.NDCGAt5-1.0) > 1e-9 || math.Abs(s.MRR-1.0) > 1e-9 {
		t.Fatalf("baseline entity slice = %+v, want 1 query / nDCG 1.0 / MRR 1.0", s)
	}
	if s := rep.Baseline.Slices["concept"]; s.Queries != 1 || math.Abs(s.NDCGAt5-1.0) > 1e-9 {
		t.Fatalf("baseline concept slice = %+v, want 1 query / nDCG 1.0", s)
	}

	// Entity-gated combination: q1 reranked (reversed -> [c2 c1]: nDCG
	// 1/log2(3), MRR 1/2); q2 gated out and measured at baseline (nDCG 1.0).
	// The reranker must never see the gated-out query.
	c := rep.Combinations[0]
	if len(fake.calls) != 1 {
		t.Fatalf("reranker called %d times, want 1 (only the entity query)", len(fake.calls))
	}
	wantNDCG := 1.0 / math.Log2(3)
	if s := c.Slices["entity"]; s.Queries != 1 || math.Abs(s.NDCGAt5-wantNDCG) > 1e-3 || math.Abs(s.MRR-0.5) > 1e-3 {
		t.Fatalf("entity slice = %+v, want 1 query / nDCG %.4f / MRR 0.5", s, wantNDCG)
	}
	if s := c.Slices["concept"]; s.Queries != 1 || math.Abs(s.NDCGAt5-1.0) > 1e-9 || math.Abs(s.MRR-1.0) > 1e-9 {
		t.Fatalf("concept slice = %+v, want 1 query / nDCG 1.0 / MRR 1.0 (gated out keeps baseline)", s)
	}
	if orderings := c.RerankerOrdering["q2"]; len(orderings) != 2 || orderings[0] != "c2" {
		t.Fatalf("gated-out ordering = %v, want baseline [c2 c1]", orderings)
	}
}

func TestProbeGateRunsMedianStabilizesVerdicts(t *testing.T) {
	art, gs := probeArtifact(t)
	// A flaky gate: q1's content flips supported/unsupported across runs;
	// with GateRuns=3 the lower median (2/3 supported) wins. q2 stays
	// consistently unsupported.
	calls := 0
	r := NewRunner(map[string]rerank.Reranker{"reverse": &recordingReranker{}}, Options{
		Content: func(ctx context.Context, ids []string) ([]string, error) {
			out := make([]string, len(ids))
			for i, id := range ids {
				out[i] = id
			}
			return out, nil
		},
		Gate: func(ctx context.Context, query, content string) (bool, error) {
			calls++
			if strings.Contains(content, "c1") {
				return calls%2 == 1, nil // supported, unsupported, supported
			}
			return false, nil
		},
		GateRuns: 3,
	})

	rep, err := r.Run(context.Background(), art, gs, []Combination{{Model: "reverse", N: 5}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// 2 non-abstention queries x 3 gate runs, baseline + combination.
	if calls != 12 {
		t.Fatalf("gate evaluated %d times, want 12 (2 queries x 3 runs x 2 configurations)", calls)
	}
	// q1's median (supported, unsupported, supported) is supported; q2 is
	// unsupported on both configurations -> verified rate 0.5.
	if rep.Baseline.VerifiedRate != 0.5 || rep.Combinations[0].VerifiedRate != 0.5 {
		t.Fatalf("verified rates = baseline %.2f / combination %.2f, want 0.5 / 0.5", rep.Baseline.VerifiedRate, rep.Combinations[0].VerifiedRate)
	}
}

func TestProbeBootstrapCIDeterministic(t *testing.T) {
	art, gs := probeArtifact(t)
	build := func() *ProbeReport {
		r := NewRunner(map[string]rerank.Reranker{"reverse": &recordingReranker{}}, Options{})
		rep, err := r.Run(context.Background(), art, gs, []Combination{{Model: "reverse", N: 5}})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		return rep
	}

	first := build()
	second := build()
	if first.Combinations[0].BootstrapCI == nil || second.Combinations[0].BootstrapCI == nil {
		t.Fatal("bootstrap CI must be reported on every combination")
	}
	a, b := first.Combinations[0].BootstrapCI, second.Combinations[0].BootstrapCI
	if a.DeltaMedianPp != b.DeltaMedianPp || a.LowerPp != b.LowerPp || a.UpperPp != b.UpperPp {
		t.Fatalf("CI not deterministic: %+v vs %+v", a, b)
	}
	// Reverse reranking loses position on every query: the median delta must
	// be negative.
	if a.DeltaMedianPp >= 0 {
		t.Fatalf("median delta = %.3f pp, want negative (reverse reranker degrades)", a.DeltaMedianPp)
	}
}

func TestProbePairsDeltasWithEmptyCandidateQueries(t *testing.T) {
	art, gs := probeArtifact(t)
	// q1 has empty candidates (artifact anomaly): the reranked result is
	// empty, and its delta must pair with q1's own baseline (0), not shift
	// q2's pairing (probe review finding C1).
	art.Queries[0].Candidates = nil
	art.Queries[0].CandidateScores = nil
	r := NewRunner(map[string]rerank.Reranker{"reverse": &recordingReranker{}}, Options{})

	rep, err := r.Run(context.Background(), art, gs, []Combination{{Model: "reverse", N: 5}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	c := rep.Combinations[0]
	if c.BootstrapCI == nil {
		t.Fatal("bootstrap CI must still be computed with an empty-candidate query")
	}
	// q1: 0-0 = 0 pp; q2: 0.5-1.0 = -50 pp; median strictly negative.
	if c.BootstrapCI.DeltaMedianPp >= 0 {
		t.Fatalf("median delta = %.3f pp, want negative (q1 pairing preserved)", c.BootstrapCI.DeltaMedianPp)
	}
	// Every non-abstention query is recorded in the ordering map, including
	// the empty one.
	if orderings, ok := c.RerankerOrdering["q1"]; !ok || len(orderings) != 0 {
		t.Fatalf("q1 reranker ordering = %v (present=%v), want empty entry", orderings, ok)
	}
}

// reportingReranker records operational metrics like the exec adapter.
type reportingReranker struct {
	recordingReranker
}

func (r *reportingReranker) RSSBytes() int64   { return 2048 }
func (r *reportingReranker) LoadTimeMs() int64 { return 42 }

func TestProbeReportsColdLoadAndRSS(t *testing.T) {
	art, gs := probeArtifact(t)
	r := NewRunner(map[string]rerank.Reranker{"reverse": &reportingReranker{recordingReranker: recordingReranker{}}}, Options{})

	rep, err := r.Run(context.Background(), art, gs, []Combination{{Model: "reverse", N: 5}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	c := rep.Combinations[0]
	if c.ColdLoadMs != 42 {
		t.Fatalf("cold load = %d, want 42 from loadReporter", c.ColdLoadMs)
	}
	if c.MaxRSSBytes != 2048 {
		t.Fatalf("rss = %d, want 2048 from loadReporter", c.MaxRSSBytes)
	}
}

func TestPercentile(t *testing.T) {
	values := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	if got := percentile(values, 50); got != 5 {
		t.Fatalf("p50 = %d, want 5", got)
	}
	if got := percentile(values, 95); got != 10 {
		t.Fatalf("p95 = %d, want 10", got)
	}
	if got := percentile([]int64{3}, 95); got != 3 {
		t.Fatalf("single-value p95 = %d, want 3", got)
	}
}
