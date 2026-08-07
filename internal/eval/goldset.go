package eval

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// allowedIntentCategories are the gold set query intents (ADR-0027).
// heading was added for the ADR-0047 heading-aware embedding benchmark:
// section-title-driven queries that require heading context in the
// embedding input. document_overview was added for the M9 document-level
// retrieval benchmark (ADR-0048): document identity/overview questions.
var allowedIntentCategories = [...]string{
	"single_fact", "concept", "procedural", "comparison", "entity", "abstention", "heading", "document_overview",
}

// IntentComparison is the gold set intent key for comparison queries; the
// M6 evidence budget is keyed off it in the runner (ADR-0037).
const IntentComparison = "comparison"

// IntentHeading is the gold set intent key for heading-aware queries
// (ADR-0047 benchmark slice).
const IntentHeading = "heading"

// AllowedIntentCategories returns the six gold set query intents.
func AllowedIntentCategories() []string {
	return append([]string(nil), allowedIntentCategories[:]...)
}

// GoldSet is the versioned, human-curated chunk-level evaluation dataset
// (ADR-0027). Queries are built exclusively from the real indexed corpus.
// The corpus is either a single document (legacy `corpus` field) or a list
// of documents (`documents`, schema 1.2) when queries span multiple books.
type GoldSet struct {
	SchemaVersion string       `json:"schema_version"`
	Corpus        CorpusInfo   `json:"corpus,omitempty"`
	Documents     []CorpusInfo `json:"documents,omitempty"`
	Queries       []GoldQuery  `json:"queries"`
}

// CorpusInfo identifies one indexed document of the corpus.
type CorpusInfo struct {
	DocumentID        string `json:"document_id"`
	CorpusFingerprint string `json:"corpus_fingerprint"`
	ChunkCount        int    `json:"chunk_count"`
}

// GoldQuery is a single benchmark query with its declared expectations.
// At most one of ExpectedChunkIDs / ExpectedRetrievalPoints is declared
// (ADR-0027/0048): ExpectedChunkIDs for chunk-level benchmarks,
// ExpectedRetrievalPoints for benchmarks whose targets are retrieval
// artifacts that are not chunks (e.g. document profile points).
type GoldQuery struct {
	ID                    string   `json:"id"`
	Intent                string   `json:"intent"`
	Query                 string   `json:"query"`
	// DocumentID scopes the query to one declared document (M9 document-
	// overview slice: the retrieval filter mirrors the user's -Doc selection).
	// Empty means whole-corpus retrieval.
	DocumentID            string   `json:"document_id,omitempty"`
	ExpectedChunkIDs      []string `json:"expected_chunk_ids"`
	ExpectedRetrievalPoints []string `json:"expected_retrieval_points"`
	ExpectedSections      []string `json:"expected_sections"`
	ExpectedNoEvidence    bool     `json:"expected_no_evidence"`
}

// ExpectedIDs returns the query's declared expectation set — the retrieval
// artifacts that must be surfaced: chunks, document profile points, or both.
func (q GoldQuery) ExpectedIDs() []string {
	if len(q.ExpectedRetrievalPoints) > 0 {
		return q.ExpectedRetrievalPoints
	}
	return q.ExpectedChunkIDs
}

// LoadGoldSet parses and validates a gold set document. Validation enforces
// the schema contract: known intents, non-empty queries, abstention queries
// declaring no expected chunks, unique ids, and a declared corpus fingerprint.
func LoadGoldSet(r io.Reader) (*GoldSet, error) {
	var gs GoldSet
	if err := json.NewDecoder(r).Decode(&gs); err != nil {
		return nil, fmt.Errorf("invalid gold set json: %w", err)
	}
	if err := gs.Validate(); err != nil {
		return nil, err
	}
	return &gs, nil
}

// declaresDocument reports whether the gold set declares the document ID
// (single-document corpus or multi-document documents array).
func (g *GoldSet) declaresDocument(docID string) bool {
	if len(g.Documents) > 0 {
		for _, d := range g.Documents {
			if d.DocumentID == docID {
				return true
			}
		}
		return false
	}
	return g.Corpus.DocumentID == docID
}

// Validate checks the structural invariants of the gold set.
func (g *GoldSet) Validate() error {
	if len(g.Queries) == 0 {
		return fmt.Errorf("gold set contains no queries")
	}
	if len(g.Documents) > 0 {
		for _, d := range g.Documents {
			if d.DocumentID == "" {
				return fmt.Errorf("gold set document has empty document_id")
			}
			if d.CorpusFingerprint == "" {
				return fmt.Errorf("gold set document %q has empty fingerprint", d.DocumentID)
			}
		}
	} else {
		if g.Corpus.DocumentID == "" {
			return fmt.Errorf("gold set corpus document_id is empty")
		}
		if g.Corpus.CorpusFingerprint == "" {
			return fmt.Errorf("gold set corpus fingerprint is empty")
		}
	}

	allowed := map[string]bool{}
	for _, c := range allowedIntentCategories {
		allowed[c] = true
	}

	seen := map[string]bool{}
	for _, q := range g.Queries {
		if q.ID == "" {
			return fmt.Errorf("query has empty id")
		}
		if !allowed[q.Intent] {
			return fmt.Errorf("query %q has unknown intent %q", q.ID, q.Intent)
		}
		if strings.TrimSpace(q.Query) == "" {
			return fmt.Errorf("query %q has empty query text", q.ID)
		}
		if seen[q.ID] {
			return fmt.Errorf("duplicate query id %q", q.ID)
		}
		seen[q.ID] = true
		if q.DocumentID != "" && !g.declaresDocument(q.DocumentID) {
			return fmt.Errorf("query %q scopes an undeclared document %q", q.ID, q.DocumentID)
		}
		if len(q.ExpectedChunkIDs) > 0 && len(q.ExpectedRetrievalPoints) > 0 {
			return fmt.Errorf("query %q declares both expected_chunk_ids and expected_retrieval_points (at most one, ADR-0048)", q.ID)
		}
		if q.ExpectedNoEvidence && len(q.ExpectedIDs()) > 0 {
			return fmt.Errorf("abstention query %q declares expected artifacts", q.ID)
		}
		if !q.ExpectedNoEvidence && len(q.ExpectedIDs()) == 0 {
			return fmt.Errorf("query %q declares no expected artifacts", q.ID)
		}
		for _, s := range q.ExpectedSections {
			if strings.TrimSpace(s) == "" {
				return fmt.Errorf("query %q has an empty expected section", q.ID)
			}
		}
	}
	return nil
}
