// Package sparse implements the SparseEncoder seam (ADR-0028): the
// indexing-stage counterpart of the dense embedding provider. M3 ships one
// implementation — BM25 term weights with corpus-wide IDF, persisted as
// Qdrant-native sparse vectors by the store layer.
package sparse

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

// SparseVector is a sparse term-weight vector: parallel term ids (indices)
// and BM25 weights (values), sorted by index for determinism.
type SparseVector struct {
	Indices []uint32  `json:"indices"`
	Values  []float32 `json:"values"`
}

// SparseEncoder converts text (chunk markdown) into a sparse vector
// representation. It is independent of the dense embedding provider and
// swappable (BM25 in M3; SPLADE/learned sparse later). Corpus context (IDF,
// vocabulary) is bound at construction — never process-global state.
type SparseEncoder interface {
	// Encode converts a chunk's text into its sparse vector.
	Encode(ctx context.Context, text string) (SparseVector, error)
} // tokenPattern matches lowercase alphanumeric tokens.
var tokenPattern = regexp.MustCompile(`[a-z0-9]+`)

// tokenize normalizes text (lowercase) and splits it into alphanumeric
// tokens. Deterministic: same text always yields the same token sequence.
func tokenize(text string) []string {
	return tokenPattern.FindAllString(strings.ToLower(text), -1)
}

// CorpusStats holds the corpus-wide statistics required for BM25: document
// frequency per term, document count, average document length, and the
// deterministic term-id vocabulary (sorted terms).
type CorpusStats struct {
	TotalDocs int
	AvgDocLen float64
	docFreq   map[string]int
	termIDs   map[string]uint32
}

// DocFreq returns the number of corpus documents containing the term.
func (s *CorpusStats) DocFreq(term string) int { return s.docFreq[term] }

// TermID returns the deterministic vocabulary id for the term.
func (s *CorpusStats) TermID(term string) uint32 { return s.termIDs[term] }

// BuildCorpusStats computes corpus-wide IDF statistics from document texts.
// Term ids are assigned over the sorted vocabulary, so results are
// deterministic regardless of input order.
func BuildCorpusStats(texts []string) (*CorpusStats, error) {
	docFreq := make(map[string]int)
	totalLen := 0
	tokenSets := make([]map[string]bool, 0, len(texts))

	for _, text := range texts {
		tokens := tokenize(text)
		if len(tokens) == 0 {
			continue
		}
		totalLen += len(tokens)
		seen := make(map[string]bool)
		for _, tok := range tokens {
			if !seen[tok] {
				seen[tok] = true
				docFreq[tok]++
			}
		}
		tokenSets = append(tokenSets, seen)
	}

	if len(tokenSets) == 0 {
		return nil, fmt.Errorf("corpus contains no tokenizable documents")
	}

	vocab := make([]string, 0, len(docFreq))
	for term := range docFreq {
		vocab = append(vocab, term)
	}
	sort.Strings(vocab)

	termIDs := make(map[string]uint32, len(vocab))
	for i, term := range vocab {
		termIDs[term] = uint32(i)
	}

	return &CorpusStats{
		TotalDocs: len(tokenSets),
		AvgDocLen: float64(totalLen) / float64(len(tokenSets)),
		docFreq:   docFreq,
		termIDs:   termIDs,
	}, nil
}

// BM25Encoder implements SparseEncoder with BM25 term weighting
// (k1 = 1.5, b = 0.75 by default) over corpus statistics bound at
// construction.
type BM25Encoder struct {
	stats *CorpusStats
	k1    float64
	b     float64
}

// BM25Option configures a BM25Encoder instance.
type BM25Option func(*BM25Encoder)

// WithBM25K1 overrides the BM25 term-frequency saturation parameter (default 1.5).
func WithBM25K1(k1 float64) BM25Option {
	return func(e *BM25Encoder) {
		if k1 > 0 {
			e.k1 = k1
		}
	}
}

// WithBM25B overrides the BM25 length-normalization parameter (default 0.75).
func WithBM25B(b float64) BM25Option {
	return func(e *BM25Encoder) {
		if b >= 0 {
			e.b = b
		}
	}
}

// NewBM25Encoder constructs a BM25 encoder bound to the given corpus
// statistics. stats must come from BuildCorpusStats — a zero-value
// CorpusStats would produce undefined weights.
func NewBM25Encoder(stats *CorpusStats, opts ...BM25Option) *BM25Encoder {
	e := &BM25Encoder{
		stats: stats,
		k1:    1.5,
		b:     0.75,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Encode converts text into its BM25-weighted sparse vector. Terms absent
// from the corpus vocabulary get no weight. Indices are sorted ascending.
func (e *BM25Encoder) Encode(ctx context.Context, text string) (SparseVector, error) {
	if err := ctx.Err(); err != nil {
		return SparseVector{}, err
	}

	tokens := tokenize(text)
	if len(tokens) == 0 {
		return SparseVector{}, nil
	}

	tf := make(map[string]int)
	for _, tok := range tokens {
		tf[tok]++
	}

	docLen := float64(len(tokens))
	denomFactor := 1 - e.b + e.b*docLen/e.stats.AvgDocLen

	indices := make([]uint32, 0, len(tf))
	values := make([]float32, 0, len(tf))
	for term, count := range tf {
		idf := e.idf(term)
		if idf <= 0 {
			continue
		}
		tfNorm := float64(count) * (e.k1 + 1) / (float64(count) + e.k1*denomFactor)
		indices = append(indices, e.stats.TermID(term))
		values = append(values, float32(idf*tfNorm))
	}

	// Deterministic output: sort index/value pairs by index.
	order := make([]int, len(indices))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool { return indices[order[i]] < indices[order[j]] })
	sortedIdx := make([]uint32, len(indices))
	sortedVal := make([]float32, len(values))
	for i, o := range order {
		sortedIdx[i] = indices[o]
		sortedVal[i] = values[o]
	}

	return SparseVector{Indices: sortedIdx, Values: sortedVal}, nil
}

// idf returns the BM25 inverse document frequency for the term
// (ln(1 + (N - df + 0.5) / (df + 0.5))). Terms never seen in the corpus get
// idf 0 and are excluded from the vector.
func (e *BM25Encoder) idf(term string) float64 {
	df := e.stats.DocFreq(term)
	if df == 0 {
		return 0
	}
	return math.Log(1 + (float64(e.stats.TotalDocs)-float64(df)+0.5)/(float64(df)+0.5))
}
