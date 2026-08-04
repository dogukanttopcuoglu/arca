package eval

import (
	"context"
	"fmt"
	"strings"
	"time"

	"arca/internal/indexing/sparse"
	"arca/internal/qa"
	qacontext "arca/internal/qa/context"
	"arca/internal/retrieval/hybrid"
	retrievalseam "arca/internal/retrieval/seam"
)

// maxGateAttempts mirrors the AnswerEngine bounded retry budget (one initial
// attempt plus one retry) by referencing the engine's single source of truth,
// so the harness policy cannot drift from production.
const maxGateAttempts = qa.MaxGateAttempts

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
	// Decompose, when non-nil, splits each gold-set query into deterministic
	// sub-queries before retrieval; sub-results are merged via the shared
	// MergeRankedLists seam — the same path the AnswerEngine uses.
	Decompose func(query string) []string
	// CorpusTexts, when non-nil, provides the indexed corpus content so the
	// runner can compute lexical abstention signals (distinctive-term DF).
	CorpusTexts func() ([]string, error)
	// DistinctiveMaxDF is the maximum document frequency for a query term to
	// count as distinctive in the lexical coverage signal (default 3).
	DistinctiveMaxDF int
	// Gate, when non-nil, runs the M5 semantic evidence gate over each
	// query's assembled retrieval content, mirroring the AnswerEngine flow
	// (retrieve -> context -> gate). It enables the M5 report section; the
	// harness itself never invokes generation. Retries follow the same
	// one-retry policy as the engine. GateProvider and GateModel identify
	// the gate adapter in the report.
	Gate         qa.EvidenceGate
	GateProvider string
	GateModel    string
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

	// Corpus term frequencies for the lexical coverage signal.
	var corpusDF *sparse.CorpusStats
	if r.opts.CorpusTexts != nil {
		texts, err := r.opts.CorpusTexts()
		if err != nil {
			return nil, fmt.Errorf("failed to read corpus texts: %w", err)
		}
		corpusDF, err = sparse.BuildCorpusStats(texts)
		if err != nil {
			return nil, fmt.Errorf("failed to build corpus statistics: %w", err)
		}
	}
	maxDF := r.opts.DistinctiveMaxDF
	if maxDF <= 0 {
		maxDF = 3
	}

	for _, q := range gs.Queries {
		query := retrievalseam.RetrievalQuery{
			QueryText: q.Query,
			TopK:      r.opts.TopK,
			Mode:      r.opts.Mode,
			MinScore:  r.opts.MinScore,
			Stats:     &retrievalseam.RetrievalStats{},
		}
		var results []retrievalseam.SearchResult
		qres := QueryResult{ID: q.ID, Intent: q.Intent}
		if r.opts.Decompose != nil {
			subs := r.opts.Decompose(q.Query)
			if len(subs) > 0 {
				qres.Decomposed = true
				var lists [][]retrievalseam.SearchResult
				for _, sub := range subs {
					subQuery := query
					subQuery.QueryText = sub
					subResults, err := r.retriever.Retrieve(ctx, subQuery)
					if err != nil {
						return nil, fmt.Errorf("query %q sub-retrieval failed: %w", q.ID, err)
					}
					if len(subResults) > 0 {
						lists = append(lists, subResults)
					}
				}
				results = retrievalseam.MergeRankedLists(lists, query.TopK)
			} else {
				results, err = r.retriever.Retrieve(ctx, query)
				if err != nil {
					return nil, fmt.Errorf("query %q retrieval failed: %w", q.ID, err)
				}
			}
		} else {
			results, err = r.retriever.Retrieve(ctx, query)
			if err != nil {
				return nil, fmt.Errorf("query %q retrieval failed: %w", q.ID, err)
			}
		}

		retrieved := make([]string, len(results))
		scores := make([]float32, len(results))
		var content strings.Builder
		for i, res := range results {
			retrieved[i] = res.ChunkID
			scores[i] = res.Score
			content.WriteString(res.ContentMarkdown)
			content.WriteString(" ")
		}

		qres.RetrievedChunkIDs = retrieved
		qres.RetrievedScores = scores
		qres.ExpectedChunkIDs = q.ExpectedChunkIDs
		qres.ExpectedNoEvidence = q.ExpectedNoEvidence
		qres.Stats = query.Stats

		// M5 evidence gate (ADR-0034): empty retrieval abstains immediately
		// without a gate call, mirroring the AnswerEngine flow. Only real
		// gate evaluations are recorded as observations.
		if r.opts.Gate != nil && len(results) > 0 {
			win := &qacontext.ContextWindow{Content: content.String()}
			decision, retries, latency, gateErr := r.evaluateGate(ctx, q.Query, win)
			obs := &GateObservation{
				Decision:  string(decision),
				LatencyMs: latency,
				Retries:   retries,
			}
			if gateErr != nil {
				obs.Error = gateErr.Error()
			}
			qres.Gate = obs
		}

		if corpusDF != nil {
			qres.Signals = abstentionSignals(q.Query, content.String(), scores, corpusDF, maxDF)
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

	if r.opts.Gate != nil {
		report.M5 = computeM5Metrics(report.PerQuery)
		if r.opts.GateProvider != "" {
			report.M5.GateProvider = r.opts.GateProvider
		}
		if r.opts.GateModel != "" {
			report.M5.GateModel = r.opts.GateModel
		}
	}

	report.DurationMs = time.Since(start).Milliseconds()

	return report, nil
}

// evaluateGate runs the M5 gate with the same one-retry policy as the
// AnswerEngine: supported and unsupported decisions return immediately;
// operational failures retry once and surface as GateError.
func (r *Runner) evaluateGate(ctx context.Context, query string, win *qacontext.ContextWindow) (qa.EvidenceDecision, int, int64, error) {
	start := time.Now()
	var lastErr error
	retries := 0
	for attempt := 1; attempt <= maxGateAttempts; attempt++ {
		decision, err := r.opts.Gate.Evaluate(ctx, query, win)
		if err != nil || decision == qa.EvidenceGateFailed {
			lastErr = err
			if attempt < maxGateAttempts {
				retries++
			}
			continue
		}
		return decision, retries, time.Since(start).Milliseconds(), nil
	}
	return qa.EvidenceGateFailed, retries, time.Since(start).Milliseconds(), lastErr
}

// computeM5Metrics derives semantic-abstention measurements from the recorded
// per-query observations. Empty retrieval abstains by construction. Gate-error
// queries are operational failures and are excluded from both abstention and
// missed-abstention counts.
func computeM5Metrics(queries []QueryResult) *M5Metrics {
	m := &M5Metrics{}
	abstain := 0
	expected := 0
	for _, qr := range queries {
		abstained := len(qr.RetrievedChunkIDs) == 0 || gateDecisionIs(qr, "unsupported")
		if abstained {
			m.GenerationSkipped++
			if qr.ExpectedNoEvidence {
				abstain++
			} else {
				m.FalseAbstentions++
			}
		}
		if qr.ExpectedNoEvidence {
			expected++
			if gateDecisionIs(qr, "supported") {
				m.MissedAbstentions++
			}
		}
	}
	m.AbstentionPrecision = frac(abstain, m.GenerationSkipped)
	m.AbstentionRecall = frac(abstain, expected)
	return m
}

// gateDecisionIs reports whether a per-query gate observation carries the
// given decision; queries without an observation never match.
func gateDecisionIs(qr QueryResult, decision string) bool {
	return qr.Gate != nil && qr.Gate.Decision == decision
}

func frac(num, den int) float64 {
	if den == 0 {
		return 0
	}
	return float64(num) / float64(den)
}

func avg(sum float64, n int) float64 {
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}
