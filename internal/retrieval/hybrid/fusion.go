package hybrid

import (
	"arca/internal/retrieval/seam"
)

// ReciprocalRankFusion combines multiple ranked SearchResult lists using the
// weighted Reciprocal Rank Fusion (RRF) formula:
// score(d) = sum over streams m of weight_m / (k + rank_m(d)).
// weights is optional and aligned with streams; missing or non-positive
// weights default to 1.0 (balanced RRF). k defaults to 60.
func ReciprocalRankFusion(streams [][]seam.SearchResult, k float64, weights ...float64) []seam.SearchResult {
	if k <= 0 {
		k = 60.0
	}

	scoreMap := make(map[string]float64)
	resultMap := make(map[string]seam.SearchResult)

	for si, stream := range streams {
		w := 1.0
		if si < len(weights) && weights[si] > 0 {
			w = weights[si]
		}
		for rank, res := range stream {
			r := rank + 1 // 1-indexed rank
			rrfScore := w / (k + float64(r))

			scoreMap[res.ChunkID] += rrfScore
			if existing, exists := resultMap[res.ChunkID]; !exists || res.ContentMarkdown != "" {
				resultMap[res.ChunkID] = res
			} else {
				_ = existing
			}
		}
	}

	fusedResults := make([]seam.SearchResult, 0, len(resultMap))
	for chunkID, res := range resultMap {
		res.Score = float32(scoreMap[chunkID])
		fusedResults = append(fusedResults, res)
	}

	seam.SortResultsByScore(fusedResults)

	return fusedResults
}
