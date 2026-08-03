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
	GitCommit  string          `json:"git_commit"`
	Timestamp  time.Time       `json:"timestamp"`
	Corpus     CorpusResult    `json:"corpus"`
	Retrieval  RetrievalConfig `json:"retrieval"`
	DurationMs int64           `json:"duration_ms"`
	Metrics    Metrics         `json:"metrics"`
	PerQuery   []QueryResult   `json:"per_query"`
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
}

// JSON renders the report as indented JSON.
func (r *Report) JSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}
