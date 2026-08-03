package eval

import (
	"arca/internal/indexing/sparse"
)

// AbstentionSignals are deterministic, cheap retrieval-tier signals for
// abstention decisions (M4 measurement): distinctive-term lexical coverage of
// the retrieved context and the top-1/top-2 score separation.
type AbstentionSignals struct {
	LexicalCoverage float64 `json:"lexical_coverage"`
	ScoreGap        float64 `json:"score_gap"`
}

// abstentionSignals computes the two signals for a query against its
// retrieved context. Corpus DF statistics gate which query terms count as
// distinctive (df <= maxDF, or absent from the corpus).
func abstentionSignals(query, retrievedContent string, scores []float32, corpus *sparse.CorpusStats, maxDF int) *AbstentionSignals {
	queryTokens := sparse.Tokenize(query)
	distinctive := make([]string, 0, len(queryTokens))
	for _, tok := range queryTokens {
		if corpus.DocFreq(tok) <= maxDF {
			distinctive = append(distinctive, tok)
		}
	}

	coverage := 1.0
	if len(distinctive) > 0 {
		contentSet := map[string]bool{}
		for _, tok := range sparse.Tokenize(retrievedContent) {
			contentSet[tok] = true
		}
		present := 0
		for _, tok := range distinctive {
			if contentSet[tok] {
				present++
			}
		}
		coverage = float64(present) / float64(len(distinctive))
	}

	gap := 0.0
	switch {
	case len(scores) >= 2 && scores[0] > 0:
		gap = float64(scores[0]) / float64(scores[1])
	case len(scores) == 1:
		gap = 1.0
	}

	return &AbstentionSignals{LexicalCoverage: coverage, ScoreGap: gap}
}
