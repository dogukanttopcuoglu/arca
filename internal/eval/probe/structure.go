package probe

import (
	"context"
	"sort"
	"strings"

	retrievalseam "arca/internal/retrieval/seam"
	"arca/internal/retrieval/rerank"
)

// StructureReranker is a deterministic, model-free reranker (research E2,
// docs/research/STRUCTURE_AWARE_RERANKING_RESEARCH.md): candidates are
// re-ranked by content score multiplied by a heading-overlap bonus —
// the fraction of query tokens present in the candidate's section path.
// It never touches embedding geometry and carries no model, GPU, or
// inference cost. Probe-side only until a benchmark accepts it.
type StructureReranker struct {
	// BonusAlpha is the overlap bonus weight: final = content * (1 + α·overlap).
	// Zero yields the baseline ordering (identity).
	BonusAlpha float64
}

// structureStopwords mirrors the GraphRetriever tokenizer's stopword set so
// the overlap signal keys on content words, not connectors.
var structureStopwords = map[string]bool{
	"what": true, "does": true, "the": true, "book": true, "say": true, "about": true,
	"how": true, "is": true, "are": true, "do": true, "a": true, "an": true, "of": true,
	"in": true, "on": true, "with": true, "and": true, "to": true, "for": true,
	"me": true, "mean": true, "by": true, "it": true, "its": true, "according": true,
}

// Rerank re-orders the candidates: score = contentScore * (1 + α·overlap).
// The returned ordering is deterministic (ties broken by the wrapper's
// ChunkID ASC stabilization).
func (r *StructureReranker) Rerank(ctx context.Context, query string, candidates []retrievalseam.SearchResult) ([]rerank.ScoredCandidate, error) {
	qTokens := structureTokens(query)
	scored := make([]rerank.ScoredCandidate, 0, len(candidates))
	for _, c := range candidates {
		overlap := structureOverlap(qTokens, sectionTokens(c))
		bonus := 1.0
		if r.BonusAlpha > 0 {
			bonus = 1 + r.BonusAlpha*overlap
		}
		scored = append(scored, rerank.ScoredCandidate{ChunkID: c.ChunkID, Score: float32(bonus) * c.Score})
	}
	sort.SliceStable(scored, func(i, j int) bool { return scored[i].Score > scored[j].Score })
	return scored, nil
}

// sectionTokens extracts the candidate's most specific heading: the last
// segment of Metadata.SectionPath when populated, otherwise the chunk ID's
// section slug (docID/<section-slug>/NNN) — the artifact records IDs only,
// so the slug is the fallback. Upper path segments ("Thinking in Systems >")
// are deliberately excluded: they repeat across every chunk of a book and
// would erase the discrimination the bonus is meant to add.
func sectionTokens(c retrievalseam.SearchResult) []string {
	sp := c.Metadata.SectionPath
	if sp == "" {
		segs := strings.Split(c.ChunkID, "/")
		if len(segs) >= 2 {
			sp = strings.ReplaceAll(segs[len(segs)-2], "-", " ")
		}
	}
	if sp == "" {
		return nil
	}
	if i := strings.LastIndex(sp, " > "); i >= 0 {
		sp = sp[i+3:]
	}
	return structureTokens(sp)
}

// structureTokens lowercases, splits hyphens, splits on non-alphanumerics
// and drops stopwords and single-character tokens.
func structureTokens(s string) []string {
	s = strings.ReplaceAll(strings.ToLower(s), "-", " ")
	var out []string
	for _, raw := range strings.Fields(s) {
		trimmed := strings.Trim(raw, "?!.,'\"():;-")
		if len(trimmed) < 2 || structureStopwords[trimmed] {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

// structureOverlap returns |qTokens ∩ sTokens| / |qTokens| (0..1).
func structureOverlap(qTokens, sTokens []string) float64 {
	if len(qTokens) == 0 {
		return 0
	}
	have := make(map[string]bool, len(sTokens))
	for _, t := range sTokens {
		have[t] = true
	}
	hits := 0
	for _, t := range qTokens {
		if have[t] {
			hits++
		}
	}
	return float64(hits) / float64(len(qTokens))
}
