package hybrid

import (
	"arca/internal/retrieval/seam"
)

// ReciprocalRankFusion combines multiple ranked SearchResult lists using the Reciprocal Rank Fusion (RRF) formula:
// RRF_score(d) = \sum_{m \in M} \frac{1}{k + r_m(d)} where k is standard constant (default 60).
func ReciprocalRankFusion(streams [][]seam.SearchResult, k float64) []seam.SearchResult {
	if k <= 0 {
		k = 60.0
	}

	scoreMap := make(map[string]float64)
	resultMap := make(map[string]seam.SearchResult)

	for _, stream := range streams {
		for rank, res := range stream {
			r := rank + 1 // 1-indexed rank
			rrfScore := 1.0 / (k + float64(r))

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
