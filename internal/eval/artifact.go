package eval

import (
	"context"
	"fmt"

	retrievalseam "arca/internal/retrieval/seam"
)

// CandidateArtifactSchemaVersion is the version of the M8 candidate artifact
// format (ADR-0045). Bump it when the format changes; the probe hard-fails on
// version mismatch so stale artifacts never feed a benchmark.
const CandidateArtifactSchemaVersion = 1

// CandidateArtifact is the M8 probe artifact (ADR-0045): per gold set query,
// the ordered candidate list produced by the unchanged production baseline
// (GraphFusionRetriever, w=1.0) at the candidate top N. Rerankers are
// re-run over these identical candidates, so retrieval variance is fully
// eliminated from reranker comparisons.
type CandidateArtifact struct {
	SchemaVersion        int               `json:"schema_version"`
	BenchmarkFingerprint string            `json:"benchmark_fingerprint"`
	GoldSetVersion       string            `json:"goldset_version"`
	GitCommit            string            `json:"git_commit"`
	CandidateTopK        int               `json:"candidate_top_n"`
	RetrievalConfig      RetrievalConfig   `json:"retrieval_config"`
	Queries              []ArtifactQuery   `json:"queries"`
}

// ArtifactQuery is one gold set query's recorded candidate list.
// CandidateScores are informational only (ADR-0044 ordering contract —
// scales are never comparable across rerankers). Reranker output orderings
// are recorded per combination in the probe report, not here: six
// combinations cannot share one field, and the artifact must stay a
// re-runnable, immutable baseline record (ADR-0045).
type ArtifactQuery struct {
	QueryID            string           `json:"query_id"`
	Query              string           `json:"query"`
	Intent             string           `json:"intent"`
	ExpectedNoEvidence bool             `json:"expected_no_evidence"`
	Candidates         []string         `json:"candidates"`
	CandidateScores    []float32        `json:"candidate_scores"`
	BaselineStats      *retrievalseam.RetrievalStats `json:"baseline_stats,omitempty"`
}

// CollectCandidateArtifact runs the gold set through the given retriever at
// the candidate top N and records the candidate lists. It verifies the
// corpus fingerprint first (ADR-0027): on mismatch nothing is produced.
// Decomposition follows the runner's path (MergeRankedLists) when configured.
func CollectCandidateArtifact(
	ctx context.Context,
	retriever retrievalseam.Retriever,
	source FingerprintSource,
	gs *GoldSet,
	opts Options,
	candidateTopK int,
) (*CandidateArtifact, error) {
	if retriever == nil {
		return nil, fmt.Errorf("artifact retriever is nil")
	}
	if candidateTopK <= 0 {
		return nil, fmt.Errorf("candidate top N must be positive, got %d", candidateTopK)
	}

	liveHashes, err := VerifyFingerprint(source, gs)
	if err != nil {
		return nil, err
	}

	art := &CandidateArtifact{
		SchemaVersion:        CandidateArtifactSchemaVersion,
		BenchmarkFingerprint: ComputeFingerprint(liveHashes),
		GoldSetVersion:       gs.SchemaVersion,
		GitCommit:            opts.GitCommit,
		CandidateTopK:        candidateTopK,
		RetrievalConfig: RetrievalConfig{
			Mode:              opts.Mode.String(),
			EmbeddingProvider: opts.EmbeddingProvider,
			EmbeddingModel:    opts.EmbeddingModel,
			TopK:              opts.TopK,
			MinScore:          opts.MinScore,
			FusionPolicy:      opts.FusionPolicy,
			Reranker:          opts.Reranker,
			Collection:        opts.Collection,
			ComparisonTopK:    opts.ComparisonTopK,
			GraphWeight:       opts.GraphWeight,
			GraphOnly:         opts.GraphOnly,
			GraphFusionConfig: opts.GraphFusionConfig,
			GateRuns:          effectiveGateRuns(opts.GateRuns),
		},
	}

	for _, q := range gs.Queries {
		query := retrievalseam.RetrievalQuery{
			QueryText: q.Query,
			TopK:      candidateTopK,
			Mode:      opts.Mode,
			MinScore:  opts.MinScore,
			Stats:     &retrievalseam.RetrievalStats{},
		}

		var results []retrievalseam.SearchResult
		if opts.Decompose != nil {
			subs := opts.Decompose(q.Query)
			if len(subs) > 0 {
				var lists [][]retrievalseam.SearchResult
				for _, sub := range subs {
					subQuery := query
					subQuery.QueryText = sub
					subResults, err := retriever.Retrieve(ctx, subQuery)
					if err != nil {
						return nil, fmt.Errorf("query %q sub-retrieval failed: %w", q.ID, err)
					}
					if len(subResults) > 0 {
						lists = append(lists, subResults)
					}
				}
				results = retrievalseam.MergeRankedLists(lists, candidateTopK)
			} else {
				results, err = retriever.Retrieve(ctx, query)
				if err != nil {
					return nil, fmt.Errorf("query %q retrieval failed: %w", q.ID, err)
				}
			}
		} else {
			results, err = retriever.Retrieve(ctx, query)
			if err != nil {
				return nil, fmt.Errorf("query %q retrieval failed: %w", q.ID, err)
			}
		}

		aq := ArtifactQuery{
			QueryID:            q.ID,
			Query:              q.Query,
			Intent:             q.Intent,
			ExpectedNoEvidence: q.ExpectedNoEvidence,
			BaselineStats:      query.Stats,
		}
		for _, res := range results {
			aq.Candidates = append(aq.Candidates, res.ChunkID)
			aq.CandidateScores = append(aq.CandidateScores, res.Score)
		}
		art.Queries = append(art.Queries, aq)
	}

	return art, nil
}
