package eval

import (
	"context"
	"errors"
	"strings"
	"testing"

	retrievalseam "arca/internal/retrieval/seam"
)

// recordingRetriever records the TopK it was asked for and returns a fixed
// candidate list for the given query.
type recordingRetriever struct {
	requestedTopKs []int
	candidates     map[string][]retrievalseam.SearchResult
}

func (f *recordingRetriever) Retrieve(ctx context.Context, query retrievalseam.RetrievalQuery) ([]retrievalseam.SearchResult, error) {
	f.requestedTopKs = append(f.requestedTopKs, query.TopK)
	return f.candidates[query.QueryText], nil
}

type failingRetriever struct {
	err error
}

func (f *failingRetriever) Retrieve(ctx context.Context, query retrievalseam.RetrievalQuery) ([]retrievalseam.SearchResult, error) {
	return nil, f.err
}

type fixedSource struct {
	hashes []string
	err    error
}

func (s fixedSource) ContentHashes(documentID string) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.hashes, nil
}

func artifactGoldSet(t *testing.T) *GoldSet {
	t.Helper()
	gs, err := LoadGoldSet(strings.NewReader(`{
		"schema_version": "1.2",
		"documents": [
			{"document_id": "book-a", "corpus_fingerprint": "` + ComputeFingerprint([]string{"h1", "h2"}) + `", "chunk_count": 2}
		],
		"queries": [
			{"id": "q1", "intent": "entity", "query": "who founded the company", "expected_chunk_ids": ["c1"]},
			{"id": "q2", "intent": "abstention", "query": "quantum cryptography details", "expected_no_evidence": true}
		]
	}`))
	if err != nil {
		t.Fatalf("LoadGoldSet: %v", err)
	}
	return gs
}

func artifactOpts() Options {
	return Options{
		Mode:              retrievalseam.RetrievalHybrid,
		TopK:              5,
		MinScore:          0.6,
		EmbeddingProvider: "test",
		EmbeddingModel:    "test-model",
		GraphWeight:       1.0,
	}
}

func TestCollectCandidateArtifactRecordsCandidates(t *testing.T) {
	gs := artifactGoldSet(t)
	rt := &recordingRetriever{candidates: map[string][]retrievalseam.SearchResult{
		"who founded the company": {
			{ChunkID: "c1", Score: 0.95, ContentMarkdown: "x"},
			{ChunkID: "c2", Score: 0.90, ContentMarkdown: "y"},
			{ChunkID: "c3", Score: 0.80, ContentMarkdown: "z"},
		},
	}}

	art, err := CollectCandidateArtifact(context.Background(), rt, fixedSource{hashes: []string{"h1", "h2"}}, gs, artifactOpts(), 100)
	if err != nil {
		t.Fatalf("CollectCandidateArtifact: %v", err)
	}

	if art.BenchmarkFingerprint != ComputeFingerprint([]string{"h1", "h2"}) {
		t.Fatalf("fingerprint = %q, want live corpus fingerprint", art.BenchmarkFingerprint)
	}
	if art.SchemaVersion != 1 {
		t.Fatalf("schema version = %d, want 1", art.SchemaVersion)
	}
	if art.CandidateTopK != 100 {
		t.Fatalf("candidate top N = %d, want 100", art.CandidateTopK)
	}
	if len(art.Queries) != 2 {
		t.Fatalf("queries = %d, want 2", len(art.Queries))
	}
	q1 := art.Queries[0]
	if q1.QueryID != "q1" || len(q1.Candidates) != 3 {
		t.Fatalf("q1 candidates = %v, want 3 recorded", q1.Candidates)
	}
	if len(q1.CandidateScores) != 3 || q1.CandidateScores[0] != 0.95 {
		t.Fatalf("q1 scores = %v, want informational scores", q1.CandidateScores)
	}
}

func TestCollectCandidateArtifactRequestsCandidateTopK(t *testing.T) {
	gs := artifactGoldSet(t)
	rt := &recordingRetriever{candidates: map[string][]retrievalseam.SearchResult{
		"who founded the company":     {{ChunkID: "c1", Score: 1.0}},
		"quantum cryptography details": nil,
	}}

	if _, err := CollectCandidateArtifact(context.Background(), rt, fixedSource{hashes: []string{"h1", "h2"}}, gs, artifactOpts(), 50); err != nil {
		t.Fatalf("CollectCandidateArtifact: %v", err)
	}
	for _, got := range rt.requestedTopKs {
		if got != 50 {
			t.Fatalf("retriever asked TopK = %d, want candidate top N 50 for every query", got)
		}
	}
}

func TestCollectCandidateArtifactAbstentionQueriesEmpty(t *testing.T) {
	gs := artifactGoldSet(t)
	rt := &recordingRetriever{candidates: map[string][]retrievalseam.SearchResult{
		"who founded the company":      {{ChunkID: "c1", Score: 1.0}},
		"quantum cryptography details": nil,
	}}

	art, err := CollectCandidateArtifact(context.Background(), rt, fixedSource{hashes: []string{"h1", "h2"}}, gs, artifactOpts(), 100)
	if err != nil {
		t.Fatalf("CollectCandidateArtifact: %v", err)
	}
	q2 := art.Queries[1]
	if q2.QueryID != "q2" || !q2.ExpectedNoEvidence {
		t.Fatalf("second query = %+v, want abstention q2", q2)
	}
	if len(q2.Candidates) != 0 {
		t.Fatalf("abstention query candidates = %v, want empty", q2.Candidates)
	}
}

func TestCollectCandidateArtifactFingerprintMismatch(t *testing.T) {
	gs := artifactGoldSet(t)
	rt := &recordingRetriever{candidates: map[string][]retrievalseam.SearchResult{
		"who founded the company": {{ChunkID: "c1", Score: 1.0}},
	}}

	_, err := CollectCandidateArtifact(context.Background(), rt, fixedSource{hashes: []string{"h1", "h3"}}, gs, artifactOpts(), 100)
	if err == nil {
		t.Fatal("expected fingerprint mismatch error, got nil")
	}
}

func TestCollectCandidateArtifactRetrievalFailure(t *testing.T) {
	gs := artifactGoldSet(t)

	_, err := CollectCandidateArtifact(context.Background(), &failingRetriever{err: errors.New("store down")}, fixedSource{hashes: []string{"h1", "h2"}}, gs, artifactOpts(), 100)
	if err == nil {
		t.Fatal("expected retrieval error propagation, got nil")
	}
}
