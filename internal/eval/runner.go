package eval

import (
	"context"
	"fmt"
	"time"

	"arca/internal/retrieval/hybrid"
	retrievalseam "arca/internal/retrieval/seam"
)

// Options configures a benchmark run. Everything needed to reproduce the run
// months later is recorded in the report (ADR-0027).
type Options struct {
	Mode              retrievalseam.RetrievalMode
	TopK              int
	MinScore          float32
	EmbeddingProvider string
	EmbeddingModel    string
	FusionPolicy      *hybrid.FusionPolicy
	Reranker          string
	Collection        string
	GitCommit         string
}

// Runner executes a gold set against a Retriever and produces a Report.
// The runner evaluates retrieval only — generation is never involved.
type Runner struct {
	retriever retrievalseam.Retriever
	source    FingerprintSource
	opts      Options
}

// New constructs a benchmark Runner. The source provides the live corpus
// content hashes for fingerprint verification.
func New(retriever retrievalseam.Retriever, source FingerprintSource, opts Options) *Runner {
	return &Runner{retriever: retriever, source: source, opts: opts}
}

// Run verifies the corpus fingerprint, executes every query through the
// retriever, computes metrics, and returns the report. It returns an error —
// without executing any query — when the fingerprint does not match.
func (r *Runner) Run(ctx context.Context, gs *GoldSet) (*Report, error) {
	start := time.Now()

	if r.retriever == nil {
		return nil, fmt.Errorf("runner retriever is nil")
	}
	if r.opts.TopK <= 0 {
		r.opts.TopK = 10
	}

	liveHashes, err := VerifyFingerprint(r.source, gs)
	if err != nil {
		return nil, err
	}

	report := &Report{
		GitCommit: r.opts.GitCommit,
		Timestamp: time.Now().UTC(),
		Corpus: CorpusResult{
			Fingerprint: ComputeFingerprint(liveHashes),
			DocumentID:  gs.Corpus.DocumentID,
			ChunkCount:  len(liveHashes),
		},
		Retrieval: RetrievalConfig{
			Mode:              r.opts.Mode.String(),
			EmbeddingProvider: r.opts.EmbeddingProvider,
			EmbeddingModel:    r.opts.EmbeddingModel,
			TopK:              r.opts.TopK,
			MinScore:          r.opts.MinScore,
			FusionPolicy:      r.opts.FusionPolicy,
			Reranker:          r.opts.Reranker,
			Collection:        r.opts.Collection,
		},
	}

	var abstentionCounts []int
	relRecallSum, relPrecisionSum, relMRRSum, relNDCGSum := 0.0, 0.0, 0.0, 0.0
	relCount := 0

	for _, q := range gs.Queries {
		query := retrievalseam.RetrievalQuery{
			QueryText: q.Query,
			TopK:      r.opts.TopK,
			Mode:      r.opts.Mode,
			MinScore:  r.opts.MinScore,
			Stats:     &retrievalseam.RetrievalStats{},
		}
		results, err := r.retriever.Retrieve(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("query %q retrieval failed: %w", q.ID, err)
		}

		retrieved := make([]string, len(results))
		scores := make([]float32, len(results))
		for i, res := range results {
			retrieved[i] = res.ChunkID
			scores[i] = res.Score
		}

		qres := QueryResult{
			ID:                 q.ID,
			Intent:             q.Intent,
			RetrievedChunkIDs:  retrieved,
			RetrievedScores:    scores,
			ExpectedChunkIDs:   q.ExpectedChunkIDs,
			ExpectedNoEvidence: q.ExpectedNoEvidence,
			Stats:              query.Stats,
		}

		if q.ExpectedNoEvidence {
			abstentionCounts = append(abstentionCounts, len(retrieved))
		} else {
			qres.RecallAtK = RecallAtK(retrieved, q.ExpectedChunkIDs, r.opts.TopK)
			qres.PrecisionAtK = PrecisionAtK(retrieved, q.ExpectedChunkIDs, r.opts.TopK)
			qres.MRR = MRR(retrieved, q.ExpectedChunkIDs)
			qres.NDCGAtK = NDCGAtK(retrieved, q.ExpectedChunkIDs, r.opts.TopK)
			relRecallSum += qres.RecallAtK
			relPrecisionSum += qres.PrecisionAtK
			relMRRSum += qres.MRR
			relNDCGSum += qres.NDCGAtK
			relCount++
		}

		report.PerQuery = append(report.PerQuery, qres)
	}

	report.Metrics = Metrics{
		RecallAtK:           avg(relRecallSum, relCount),
		PrecisionAtK:        avg(relPrecisionSum, relCount),
		MRR:                 avg(relMRRSum, relCount),
		NDCGAtK:             avg(relNDCGSum, relCount),
		NoEvidencePrecision: NoEvidencePrecision(abstentionCounts),
		Queries:             len(gs.Queries),
	}
	report.DurationMs = time.Since(start).Milliseconds()

	return report, nil
}

func avg(sum float64, n int) float64 {
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}
