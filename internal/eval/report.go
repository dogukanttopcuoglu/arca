package eval

import (
	"encoding/json"
	"time"

	"arca/internal/retrieval/hybrid"
	retrievalseam "arca/internal/retrieval/seam"
)

// Report is the fully reproducible benchmark output (ADR-0027). It carries
// every configuration input plus the metric results.
type Report struct {
	GitCommit string       `json:"git_commit"`
	Timestamp time.Time    `json:"timestamp"`
	Corpus    CorpusResult `json:"corpus"`
	// Documents records per-document identity for multi-document corpora
	// (gold set schema 1.2); absent for single-document runs.
	Documents  []CorpusResult  `json:"documents,omitempty"`
	Retrieval  RetrievalConfig `json:"retrieval"`
	DurationMs int64           `json:"duration_ms"`
	Metrics    Metrics         `json:"metrics"`
	PerQuery   []QueryResult   `json:"per_query"`
	// M5 records orchestration and semantic-gate measurements. It is absent
	// from reports produced without the M5 Gate option, keeping M3/M4 report
	// output unchanged.
	M5 *M5Metrics `json:"m5,omitempty"`
}

// CorpusResult records the verified corpus identity of the run.
type CorpusResult struct {
	Fingerprint string `json:"corpus_fingerprint"`
	DocumentID  string `json:"document_id"`
	ChunkCount  int    `json:"chunk_count"`
}

// RetrievalConfig records the retrieval configuration of the run.
type RetrievalConfig struct {
	Mode              string               `json:"mode"`
	EmbeddingProvider string               `json:"embedding_provider"`
	EmbeddingModel    string               `json:"embedding_model"`
	TopK              int                  `json:"top_k"`
	MinScore          float32              `json:"min_score"`
	FusionPolicy      *hybrid.FusionPolicy `json:"fusion_policy,omitempty"`
	Reranker          string               `json:"reranker,omitempty"`
	Collection        string               `json:"collection"`
	ComparisonTopK    int                  `json:"comparison_top_k,omitempty"`
}

// Metrics aggregates retrieval quality over the gold set. Recall, precision,
// MRR, and nDCG are averaged over non-abstention queries only; abstention
// queries are measured by NoEvidencePrecision.
type Metrics struct {
	RecallAtK           float64 `json:"recall_at_k"`
	PrecisionAtK        float64 `json:"precision_at_k"`
	MRR                 float64 `json:"mrr"`
	NDCGAtK             float64 `json:"ndcg_at_k"`
	NoEvidencePrecision float64 `json:"no_evidence_precision"`
	Queries             int     `json:"queries"`
}

// QueryResult records one query's retrieved set and per-query metrics.
type QueryResult struct {
	Signals            *AbstentionSignals            `json:"abstention_signals,omitempty"`
	ID                 string                        `json:"id"`
	Intent             string                        `json:"intent"`
	RetrievedChunkIDs  []string                      `json:"retrieved_chunk_ids"`
	RetrievedScores    []float32                     `json:"retrieved_scores"`
	ExpectedChunkIDs   []string                      `json:"expected_chunk_ids"`
	ExpectedNoEvidence bool                          `json:"expected_no_evidence"`
	RecallAtK          float64                       `json:"recall_at_k,omitempty"`
	PrecisionAtK       float64                       `json:"precision_at_k,omitempty"`
	MRR                float64                       `json:"mrr,omitempty"`
	NDCGAtK            float64                       `json:"ndcg_at_k,omitempty"`
	Stats              *retrievalseam.RetrievalStats `json:"stats,omitempty"`
	// M5 orchestration and evidence-gate observations (ADR-0034). Recorded
	// only when the gate actually evaluated the query; empty retrieval
	// abstains without an observation, mirroring the AnswerEngine flow.
	Decomposed bool             `json:"decomposed,omitempty"`
	Gate       *GateObservation `json:"gate,omitempty"`
}

// GateObservation records one evidence-gate evaluation (ADR-0034): the typed
// decision, any operational failure, latency, and retries.
type GateObservation struct {
	Decision  string `json:"decision"`
	Error     string `json:"error,omitempty"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
	Retries   int    `json:"retries,omitempty"`
}

// M5Metrics aggregates the M5 semantic-abstention measurements against the
// Gold Set labels. Ground truth is query-level expected_no_evidence; the
// reports must document that this is not context-window-level sufficiency.
// Gate-error queries are operational failures, excluded from both abstention
// and missed-abstention counts.
type M5Metrics struct {
	AbstentionPrecision float64 `json:"abstention_precision"`
	AbstentionRecall    float64 `json:"abstention_recall"`
	FalseAbstentions    int     `json:"false_abstentions"`
	MissedAbstentions   int     `json:"missed_abstentions"`
	GenerationSkipped   int     `json:"generation_skipped"`
	GateProvider        string  `json:"gate_provider,omitempty"`
	GateModel           string  `json:"gate_model,omitempty"`
}

// JSON renders the report as indented JSON.
func (r *Report) JSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}
