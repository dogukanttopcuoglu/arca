// Package rerank implements the M8 second-stage reranking layer
// (ADR-0043...0046): a Reranker seam with an ordering-only contract, and a
// RerankedRetriever execution component wrapping any seam.Retriever.
package rerank

import (
	"context"
	"sort"

	retrievalseam "arca/internal/retrieval/seam"
)

// ScoredCandidate is one reranked candidate. The Score is informational
// only — the ordering contract (ADR-0044) forbids interpreting its absolute
// value or scale; the only guarantee is the ordering of the returned list,
// with ChunkID ASC tie-break enforced by the wrapper.
type ScoredCandidate struct {
	ChunkID string
	Score   float32
}

// Reranker abstracts model behavior only: it reorders a candidate list for a
// query. Adapters (cross-encoder, late-interaction) may use arbitrary score
// scales; they are never shared across adapters. The wrapper enforces
// deterministic ordering.
type Reranker interface {
	// Rerank returns the candidates in reranked order. All input ChunkIDs
	// must be present exactly once; any missing or extra IDs are ignored.
	Rerank(ctx context.Context, query string, candidates []retrievalseam.SearchResult) ([]ScoredCandidate, error)
}

// Config is the wrapper-internal configuration: the candidate budget N the
// wrapper requests from the inner retriever, and the Reranker to apply.
type Config struct {
	// CandidateBudget is the internal candidate budget N. Zero or negative
	// disables reranking: the wrapper passes the query through unchanged.
	CandidateBudget int
	// Reranker is the model adapter. Nil disables reranking.
	Reranker Reranker
}

// RerankedRetriever wraps any seam.Retriever with a second-stage rerank
// (ADR-0044). From the outside it behaves like a standard retriever — the
// caller asks TopK=K and receives K results; the candidate budget N is
// internal behavior only.
type RerankedRetriever struct {
	inner  retrievalseam.Retriever
	config Config
}

// NewRerankedRetriever constructs the wrapper around the given inner
// retriever.
func NewRerankedRetriever(inner retrievalseam.Retriever, config Config) *RerankedRetriever {
	return &RerankedRetriever{inner: inner, config: config}
}

// Retrieve requests the candidate budget N from the inner retriever, reranks
// the candidates, and returns the caller's TopK. Reranker failures degrade
// gracefully to the inner retriever's ordering (Graceful Degradation).
// Empty candidate lists stay empty — abstention behavior is preserved.
func (r *RerankedRetriever) Retrieve(ctx context.Context, query retrievalseam.RetrievalQuery) ([]retrievalseam.SearchResult, error) {
	query.Normalize()

	if r.config.Reranker == nil || r.config.CandidateBudget <= 0 {
		return r.inner.Retrieve(ctx, query)
	}

	innerQuery := query
	innerQuery.TopK = r.config.CandidateBudget
	innerResults, err := r.inner.Retrieve(ctx, innerQuery)
	if err != nil {
		return nil, err
	}
	if len(innerResults) == 0 {
		return nil, nil
	}

	ordered, err := r.config.Reranker.Rerank(ctx, query.QueryText, innerResults)
	if err != nil {
		if query.Stats != nil {
			query.Stats.RerankerFailed = true
		}
		return truncate(innerResults, query.TopK), nil
	}
	if query.Stats != nil {
		query.Stats.Reranked = true
	}

	return applyOrdering(innerResults, ordered, query.TopK), nil
}

// applyOrdering maps the reranker's ordered candidates back to SearchResults
// and stabilizes equal-score groups deterministically by ChunkID ASC. Scores
// in the output are the reranker's informational scores.
func applyOrdering(inner []retrievalseam.SearchResult, ordered []ScoredCandidate, topK int) []retrievalseam.SearchResult {
	byID := make(map[string]retrievalseam.SearchResult, len(inner))
	for _, res := range inner {
		byID[res.ChunkID] = res
	}

	out := make([]retrievalseam.SearchResult, 0, len(ordered))
	for i := 0; i < len(ordered); {
		score := ordered[i].Score
		group := make([]retrievalseam.SearchResult, 0, 4)
		for i < len(ordered) && ordered[i].Score == score {
			if res, ok := byID[ordered[i].ChunkID]; ok {
				res.Score = score
				group = append(group, res)
			}
			i++
		}
		sort.Slice(group, func(a, b int) bool { return group[a].ChunkID < group[b].ChunkID })
		out = append(out, group...)
	}

	return truncate(out, topK)
}

func truncate(results []retrievalseam.SearchResult, topK int) []retrievalseam.SearchResult {
	if topK <= 0 {
		topK = 10
	}
	if len(results) > topK {
		return results[:topK]
	}
	return results
}
