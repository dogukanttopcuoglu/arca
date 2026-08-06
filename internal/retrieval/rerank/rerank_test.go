package rerank

import (
	"context"
	"errors"
	"testing"

	retrievalseam "arca/internal/retrieval/seam"
)

// fakeInner records the TopK it was asked for and returns a fixed candidate
// list, simulating the inner retriever of the wrapper.
type fakeInner struct {
	requestedTopK int
	results       []retrievalseam.SearchResult
	err           error
}

func (f *fakeInner) Retrieve(ctx context.Context, query retrievalseam.RetrievalQuery) ([]retrievalseam.SearchResult, error) {
	f.requestedTopK = query.TopK
	if f.err != nil {
		return nil, f.err
	}
	if len(f.results) > query.TopK {
		return f.results[:query.TopK], nil
	}
	return f.results, nil
}

// fakeReranker reorders candidates according to a configured preference
// ordering of ChunkIDs, optionally failing. With constantScore all
// candidates receive the same score, exercising the wrapper's tie-break.
// Scores are otherwise arbitrary — the ordering contract forbids the
// wrapper from interpreting their scale.
type fakeReranker struct {
	preference    []string
	constantScore bool
	err           error
}

func (f *fakeReranker) Rerank(ctx context.Context, query string, candidates []retrievalseam.SearchResult) ([]ScoredCandidate, error) {
	if f.err != nil {
		return nil, f.err
	}
	byID := make(map[string]float32, len(candidates))
	for i, c := range candidates {
		if f.constantScore {
			byID[c.ChunkID] = 1.0
		} else {
			byID[c.ChunkID] = float32(i)
		}
	}
	out := make([]ScoredCandidate, 0, len(candidates))
	for _, id := range f.preference {
		if score, ok := byID[id]; ok {
			out = append(out, ScoredCandidate{ChunkID: id, Score: score})
		}
	}
	return out, nil
}

func results(ids ...string) []retrievalseam.SearchResult {
	out := make([]retrievalseam.SearchResult, len(ids))
	for i, id := range ids {
		out[i] = retrievalseam.SearchResult{ChunkID: id, Score: float32(100 - i)}
	}
	return out
}

func ids(r []retrievalseam.SearchResult) []string {
	out := make([]string, len(r))
	for i, res := range r {
		out[i] = res.ChunkID
	}
	return out
}

func TestRerankedRetrieverAppliesRerankerOrdering(t *testing.T) {
	inner := &fakeInner{results: results("a", "b", "c")}
	reranker := &fakeReranker{preference: []string{"c", "a", "b"}}
	r := NewRerankedRetriever(inner, Config{CandidateBudget: 10, Reranker: reranker})

	got, err := r.Retrieve(context.Background(), retrievalseam.RetrievalQuery{QueryText: "q", TopK: 2})
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}
	want := []string{"c", "a"}
	if !equal(ids(got), want) {
		t.Fatalf("ordering = %v, want %v", ids(got), want)
	}
}

func TestRerankedRetrieverRequestsCandidateBudget(t *testing.T) {
	inner := &fakeInner{results: results("a", "b", "c", "d", "e")}
	reranker := &fakeReranker{preference: []string{"e", "d", "c", "b", "a"}}
	r := NewRerankedRetriever(inner, Config{CandidateBudget: 50, Reranker: reranker})

	got, err := r.Retrieve(context.Background(), retrievalseam.RetrievalQuery{QueryText: "q", TopK: 3})
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}
	if inner.requestedTopK != 50 {
		t.Fatalf("inner TopK = %d, want candidate budget 50", inner.requestedTopK)
	}
	if len(got) != 3 {
		t.Fatalf("returned %d results, want caller TopK 3", len(got))
	}
}

func TestRerankedRetrieverTieBreakChunkIDAscending(t *testing.T) {
	inner := &fakeInner{results: results("b", "a", "c")}
	// All candidates receive the same score; the wrapper must stabilize the
	// tie deterministically by ChunkID ASC.
	reranker := &fakeReranker{preference: []string{"c", "a", "b"}, constantScore: true}
	r := NewRerankedRetriever(inner, Config{CandidateBudget: 10, Reranker: reranker})

	got, err := r.Retrieve(context.Background(), retrievalseam.RetrievalQuery{QueryText: "q", TopK: 10})
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}
	want := []string{"a", "b", "c"}
	if !equal(ids(got), want) {
		t.Fatalf("tie-break ordering = %v, want %v", ids(got), want)
	}
}

func TestRerankedRetrieverPreservesEmptyCandidates(t *testing.T) {
	reranker := &fakeReranker{preference: []string{"a"}}
	r := NewRerankedRetriever(&fakeInner{results: nil}, Config{CandidateBudget: 10, Reranker: reranker})

	got, err := r.Retrieve(context.Background(), retrievalseam.RetrievalQuery{QueryText: "q", TopK: 5})
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("abstention violated: got %d results for empty candidates", len(got))
	}
}

func TestRerankedRetrieverRerankerErrorFallsBack(t *testing.T) {
	inner := &fakeInner{results: results("a", "b", "c", "d")}
	r := NewRerankedRetriever(inner, Config{
		CandidateBudget: 10,
		Reranker:        &fakeReranker{err: errors.New("model down")},
	})

	stats := &retrievalseam.RetrievalStats{}
	got, err := r.Retrieve(context.Background(), retrievalseam.RetrievalQuery{QueryText: "q", TopK: 2, Stats: stats})
	if err != nil {
		t.Fatalf("reranker failure must degrade gracefully, got error: %v", err)
	}
	if !equal(ids(got), []string{"a", "b"}) {
		t.Fatalf("fallback = %v, want inner top-2", ids(got))
	}
	if !stats.RerankerFailed {
		t.Fatal("Stats.RerankerFailed not set on fallback")
	}
}

func TestRerankedRetrieverDisabledConfigPassesThrough(t *testing.T) {
	inner := &fakeInner{results: results("a", "b", "c")}

	for _, cfg := range []Config{{CandidateBudget: 0, Reranker: &fakeReranker{preference: []string{"c"}}}, {CandidateBudget: 10, Reranker: nil}} {
		r := NewRerankedRetriever(inner, cfg)
		got, err := r.Retrieve(context.Background(), retrievalseam.RetrievalQuery{QueryText: "q", TopK: 2})
		if err != nil {
			t.Fatalf("Retrieve failed: %v", err)
		}
		if !equal(ids(got), []string{"a", "b"}) {
			t.Fatalf("disabled config must pass through unchanged, got %v", ids(got))
		}
		if inner.requestedTopK != 2 {
			t.Fatalf("disabled config changed TopK: inner got %d, want caller TopK", inner.requestedTopK)
		}
	}
}

func TestRerankedRetrieverScaleIndependentOrdering(t *testing.T) {
	// Two adapters with different score scales but the same preference order
	// must produce the same final ordering: the wrapper never interprets
	// absolute scores (ADR-0044 ordering contract).
	inner := &fakeInner{results: results("a", "b", "c")}
	run := func(r *RerankedRetriever) []string {
		got, err := r.Retrieve(context.Background(), retrievalseam.RetrievalQuery{QueryText: "q", TopK: 10})
		if err != nil {
			t.Fatalf("Retrieve failed: %v", err)
		}
		return ids(got)
	}

	bigScale := &scoredReranker{preference: []string{"b", "a", "c"}, scale: func(i int) float32 { return float32(i) * 1000 }}
	smallScale := &scoredReranker{preference: []string{"b", "a", "c"}, scale: func(i int) float32 { return float32(i) / 1000 }}
	first := run(NewRerankedRetriever(inner, Config{CandidateBudget: 10, Reranker: bigScale}))
	second := run(NewRerankedRetriever(inner, Config{CandidateBudget: 10, Reranker: smallScale}))
	if !equal(first, second) {
		t.Fatalf("scale dependence: %v vs %v", first, second)
	}
}

// scoredReranker assigns each candidate a score from the given scale
// function, ordered by preference; it exists to prove the wrapper never
// interprets absolute scores (ADR-0044 ordering contract).
type scoredReranker struct {
	preference []string
	scale      func(index int) float32
}

func (f *scoredReranker) Rerank(ctx context.Context, query string, candidates []retrievalseam.SearchResult) ([]ScoredCandidate, error) {
	byID := make(map[string]float32, len(candidates))
	for i, c := range candidates {
		byID[c.ChunkID] = f.scale(i)
	}
	out := make([]ScoredCandidate, 0, len(candidates))
	for _, id := range f.preference {
		if score, ok := byID[id]; ok {
			out = append(out, ScoredCandidate{ChunkID: id, Score: score})
		}
	}
	return out, nil
}

func TestRerankedRetrieverDeterministicAcrossRuns(t *testing.T) {
	inner := &fakeInner{results: results("a", "b", "c", "d")}
	r := NewRerankedRetriever(inner, Config{CandidateBudget: 10, Reranker: &fakeReranker{preference: []string{"d", "b", "c", "a"}}})
	ctx := context.Background()

	first, err := r.Retrieve(ctx, retrievalseam.RetrievalQuery{QueryText: "q", TopK: 4})
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}
	second, err := r.Retrieve(ctx, retrievalseam.RetrievalQuery{QueryText: "q", TopK: 4})
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}
	if !equal(ids(first), ids(second)) {
		t.Fatalf("non-deterministic ordering: %v vs %v", ids(first), ids(second))
	}
}

func equal(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
