package retriever

import (
	"context"
	"fmt"
	"sort"
	"strings"

	graphmodel "arca/internal/graph/model"
	graphstore "arca/internal/graph/store"
	indexingmodel "arca/internal/indexing/model"
	"arca/internal/indexing/store"
	retrievalseam "arca/internal/retrieval/seam"
)

// GraphRetriever adapts the entity-only knowledge graph to the standard
// Retriever seam (ADR-0039): deterministic lexical entity-overlap scoring.
// Its job is not to answer queries — it deterministically produces chunk
// candidates carrying entity evidence for downstream fusion.
type GraphRetriever struct {
	store        graphstore.GraphStore
	contentStore store.ContentStore
	vectorStore  store.VectorStore
}

// GraphRetrieverOption configures a GraphRetriever instance.
type GraphRetrieverOption func(*GraphRetriever)

// WithVectorStore attaches the vector store so chunk content is resolved
// from the point payload — the same source of truth as the dense retriever
// (production: indexing and querying run in different processes, so the
// process-local ContentStore is empty). Without it, the ContentStore remains
// the only content source (legacy/test composition).
func WithVectorStore(vs store.VectorStore) GraphRetrieverOption {
	return func(g *GraphRetriever) {
		g.vectorStore = vs
	}
}

// NewGraphRetriever constructs a GraphRetriever over the graph store and the
// content store used for chunk markdown resolution.
func NewGraphRetriever(store graphstore.GraphStore, contentStore store.ContentStore, opts ...GraphRetrieverOption) *GraphRetriever {
	g := &GraphRetriever{
		store:        store,
		contentStore: contentStore,
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// stopwords are stripped from queries before entity matching. The list is
// identical to the kill-gate prototype's set so benchmark numbers stay
// comparable (ADR-0039); it is query-language scaffolding, not an entity
// signal.
var stopwords = map[string]bool{
	"what": true, "does": true, "the": true, "book": true, "say": true, "about": true,
	"how": true, "is": true, "are": true, "do": true, "a": true, "an": true, "of": true,
	"in": true, "on": true, "with": true, "and": true, "to": true, "for": true,
}

// queryTokens normalizes the query deterministically: lowercase, possessive
// apostrophes removed, punctuation trimmed, stopwords removed, single
// characters dropped.
func queryTokens(query string) []string {
	var out []string
	for _, w := range strings.Fields(strings.ToLower(query)) {
		w = strings.ReplaceAll(w, "'", "")
		w = strings.Trim(w, "?,.'\"!():;-")
		if w != "" && len(w) > 1 && !stopwords[w] {
			out = append(out, w)
		}
	}
	return out
}

// Retrieve scores chunks by entity evidence: for every entity node whose
// normalized name contains at least one query token, each evidenced chunk
// gains entity.Score x (matchedTokens / entityTokenCount). Results are fully
// ordered (score desc, chunk ID asc), filtered by query.MinScore, resolved
// through the content store, and truncated to query.TopK.
func (g *GraphRetriever) Retrieve(ctx context.Context, query retrievalseam.RetrievalQuery) ([]retrievalseam.SearchResult, error) {
	if query.QueryText == "" {
		return nil, fmt.Errorf("query text cannot be empty")
	}
	query.Normalize()

	tokens := queryTokens(query.QueryText)
	if len(tokens) == 0 {
		return []retrievalseam.SearchResult{}, nil
	}
	// Distinct tokens only: coverage counts each matched token once, so a
	// repeated query token must not inflate token_coverage.
	tokens = unique(tokens)

	nodes, err := g.store.ListEntityNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("graph entity listing failed: %w", err)
	}

	chunkScore := make(map[string]float64)
	for _, node := range nodes {
		score, matched := scoreEntity(node, tokens)
		if matched == 0 {
			continue
		}
		for _, cid := range node.ChunkIDs() {
			chunkScore[cid] += score
		}
	}

	chunkIDs := make([]string, 0, len(chunkScore))
	for cid := range chunkScore {
		chunkIDs = append(chunkIDs, cid)
	}
	sort.Slice(chunkIDs, func(i, j int) bool {
		if chunkScore[chunkIDs[i]] != chunkScore[chunkIDs[j]] {
			return chunkScore[chunkIDs[i]] > chunkScore[chunkIDs[j]]
		}
		return chunkIDs[i] < chunkIDs[j]
	})

	results := make([]retrievalseam.SearchResult, 0, len(chunkIDs))
	resolveContent := make([]string, 0, len(chunkIDs))
	for _, cid := range chunkIDs {
		score := chunkScore[cid]
		if score < float64(query.MinScore) {
			continue
		}
		results = append(results, retrievalseam.SearchResult{
			ChunkID: cid,
			Score:   float32(score),
			Metadata: indexingmodel.VectorMetadata{
				ChunkID: cid,
			},
		})
		resolveContent = append(resolveContent, cid)
		if len(results) >= query.TopK {
			break
		}
	}

	if len(resolveContent) > 0 {
		contents, err := g.resolveContent(ctx, resolveContent)
		if err != nil {
			return nil, err
		}
		for i := range results {
			results[i].ContentMarkdown = contents[results[i].ChunkID]
		}
	}

	return results, nil
}

// resolveContent fills chunk markdown from the vector store point payload
// first (the production source of truth, shared with the dense retriever),
// falling back to the ContentStore for anything the payload lacks. Content
// resolution never changes result ordering.
func (g *GraphRetriever) resolveContent(ctx context.Context, chunkIDs []string) (map[string]string, error) {
	contents := make(map[string]string, len(chunkIDs))
	var missing []string

	if g.vectorStore != nil {
		points, err := g.vectorStore.ListPoints(ctx, indexingmodel.MetadataFilter{ChunkIDs: chunkIDs})
		if err != nil {
			return nil, fmt.Errorf("failed to resolve chunk content: %w", err)
		}
		for _, pt := range points {
			if pt.ContentMarkdown != "" {
				contents[pt.Metadata.ChunkID] = pt.ContentMarkdown
			}
		}
		for _, cid := range chunkIDs {
			if contents[cid] == "" {
				missing = append(missing, cid)
			}
		}
	} else {
		missing = chunkIDs
	}

	if len(missing) > 0 {
		fallback, err := g.contentStore.GetContent(ctx, missing)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve chunk content: %w", err)
		}
		for cid, text := range fallback {
			contents[cid] = text
		}
	}

	return contents, nil
}

// unique returns the input tokens without duplicates, preserving order.
func unique(tokens []string) []string {
	seen := make(map[string]bool, len(tokens))
	out := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

// scoreEntity returns (contribution, matchedTokenCount) for one entity node:
// the entity score scaled by token coverage (matched/entityTokens) when at
// least one distinct query token is contained in the normalized name.
// Coverage never exceeds 1.0 because matched tokens are counted distinctly.
func scoreEntity(node graphmodel.Node, tokens []string) (float64, int) {
	name := node.Name()
	if name == "" {
		return 0, 0
	}
	matched := 0
	for _, t := range tokens {
		if strings.Contains(name, t) {
			matched++
		}
	}
	if matched == 0 {
		return 0, 0
	}
	entityTokens := len(strings.Fields(name))
	if entityTokens == 0 {
		entityTokens = 1
	}
	if matched > entityTokens {
		matched = entityTokens
	}
	return node.Score() * float64(matched) / float64(entityTokens), matched
}
