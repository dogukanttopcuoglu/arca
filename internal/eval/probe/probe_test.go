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
