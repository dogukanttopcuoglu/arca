// Package documentoverview implements the M9 document-level retrieval
// component (ADR-0048): a deterministic adapter on the Retriever seam that
// serves the document_overview intent. It never runs embedding search,
// graph, decomposition, or hybrid paths — it reads the vector store and
// selects documents' profile points plus their first real content chunks.
package documentoverview

import (
	"context"
	"fmt"
	"sort"

	indexingmodel "arca/internal/indexing/model"
	"arca/internal/indexing/store"
	pdfmodel "arca/internal/pdfinspector/model"
	retrievalseam "arca/internal/retrieval/seam"
)

// IntroChunks is how many real content chunks follow the profile point for
// a document-filtered overview (M9 spec: small, deterministic context).
const IntroChunks = 4

// Retriever serves document-level overview queries deterministically from
// the vector store: with a document filter the document's profile point
// plus its first real content chunks (chunk_order > 1 — known front-matter
// noise is excluded at retrieval time); without a filter, all profile
// points (library-level overview). Ordering is deterministic: profiles by
// document ID, then chunks by (document ID, chunk_order).
type Retriever struct {
	store store.VectorStore
}

// NewRetriever constructs the overview retriever over the vector store.
func NewRetriever(s store.VectorStore) *Retriever {
	return &Retriever{store: s}
}

// Retrieve implements the Retriever seam (ADR-0048).
func (r *Retriever) Retrieve(ctx context.Context, query retrievalseam.RetrievalQuery) ([]retrievalseam.SearchResult, error) {
	if query.QueryText == "" {
		return nil, fmt.Errorf("query text cannot be empty")
	}
	query.Normalize()

	points, err := r.store.ListPoints(ctx, indexingmodel.MetadataFilter{DocumentIDs: query.Filter.DocumentIDs})
	if err != nil {
		return nil, fmt.Errorf("overview retrieval failed: %w", err)
	}

	var profiles []store.VectorPoint
	var chunks []store.VectorPoint
	for _, p := range points {
		if p.Metadata.ContentType == pdfmodel.ContentTypeDocumentProfile {
			profiles = append(profiles, p)
			continue
		}
		// Front matter (order 1, e.g. the copyright page) is excluded at
		// retrieval time — the gate is never asked to clean the input.
		if p.Metadata.ChunkOrder > 1 {
			chunks = append(chunks, p)
		}
	}

	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Metadata.DocumentID < profiles[j].Metadata.DocumentID })
	sort.Slice(chunks, func(i, j int) bool {
		if chunks[i].Metadata.DocumentID != chunks[j].Metadata.DocumentID {
			return chunks[i].Metadata.DocumentID < chunks[j].Metadata.DocumentID
		}
		return chunks[i].Metadata.ChunkOrder < chunks[j].Metadata.ChunkOrder
	})

	results := make([]retrievalseam.SearchResult, 0, query.TopK)
	// Profiles first (deterministic, no embedding similarity involved);
	// scores are ordering-only — the overview path never fuses.
	for _, p := range profiles {
		results = append(results, toResult(p, 1.0))
		if len(results) >= query.TopK {
			return results, nil
		}
	}
	// Real content chunks join the context only for a document-filtered
	// overview (M9 decision): without a filter the library-level overview
	// surfaces profile points exclusively — no chunk retrieval fallback.
	if len(query.Filter.DocumentIDs) > 0 {
		for _, c := range chunks {
			results = append(results, toResult(c, 0.5))
			if len(results) >= query.TopK {
				break
			}
		}
	}
	return results, nil
}

// toResult carries the full payload metadata (document/section/pages) so
// the context builder and citation rendering see a real source.
func toResult(p store.VectorPoint, score float32) retrievalseam.SearchResult {
	return retrievalseam.SearchResult{
		ChunkID:         p.Metadata.ChunkID,
		Score:           score,
		ContentMarkdown: p.ContentMarkdown,
		Metadata:        p.Metadata,
	}
}
