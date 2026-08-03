package seam

// MergeRankedLists deterministically merges ranked retrieval streams for
// decomposed queries: results are interleaved by rank (round-robin across
// streams), deduplicated by ChunkID keeping the first occurrence, and
// truncated to topK. No scoring is involved — per-stream ranking is preserved
// by the interleave order.
func MergeRankedLists(lists [][]SearchResult, topK int) []SearchResult {
	if topK <= 0 {
		topK = 10
	}
	seen := make(map[string]bool)
	var merged []SearchResult

	maxLen := 0
	for _, l := range lists {
		if len(l) > maxLen {
			maxLen = len(l)
		}
	}

	for rank := 0; rank < maxLen && len(merged) < topK; rank++ {
		for _, l := range lists {
			if rank >= len(l) {
				continue
			}
			res := l[rank]
			if seen[res.ChunkID] {
				continue
			}
			seen[res.ChunkID] = true
			merged = append(merged, res)
			if len(merged) >= topK {
				break
			}
		}
	}
	return merged
}
