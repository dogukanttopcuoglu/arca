// Package eval implements the retrieval evaluation harness: Gold Set loading,
// corpus fingerprint verification, retrieval metric computation, and report
// emission. The harness evaluates retrieval only — generation is never
// involved (ADR-0027).
package eval

import (
	"math"
)

// RecallAtK returns the fraction of relevant chunks retrieved within the top k.
// Queries with no relevant chunks are excluded by the caller (undefined metric).
func RecallAtK(retrieved, relevant []string, k int) float64 {
	if len(relevant) == 0 || k <= 0 {
		return 0
	}
	rel := toSet(relevant)
	hit := 0
	for i, id := range retrieved {
		if i >= k {
			break
		}
		if rel[id] {
			hit++
		}
	}
	return float64(hit) / float64(len(rel))
}

// PrecisionAtK returns the fraction of the top k retrieved chunks that are
// relevant.
func PrecisionAtK(retrieved, relevant []string, k int) float64 {
	if k <= 0 {
		return 0
	}
	rel := toSet(relevant)
	hit := 0
	for i, id := range retrieved {
		if i >= k {
			break
		}
		if rel[id] {
			hit++
		}
	}
	return float64(hit) / float64(k)
}

// MRR returns the mean reciprocal rank of the first relevant chunk. Queries
// with no relevant chunks contribute zero.
func MRR(retrieved, relevant []string) float64 {
	rel := toSet(relevant)
	for i, id := range retrieved {
		if rel[id] {
			return 1.0 / float64(i+1)
		}
	}
	return 0
}

// NDCGAtK returns normalized discounted cumulative gain over the top k with
// binary gains (1 for relevant, 0 otherwise). Returns 0 when no relevant
// chunks exist (IDCG is zero).
func NDCGAtK(retrieved, relevant []string, k int) float64 {
	rel := toSet(relevant)

	dcg := 0.0
	for i, id := range retrieved {
		if i >= k {
			break
		}
		if rel[id] {
			dcg += 1.0 / math.Log2(float64(i+2))
		}
	}

	idcg := 0.0
	for i := 0; i < len(rel) && i < k; i++ {
		idcg += 1.0 / math.Log2(float64(i+2))
	}

	if idcg == 0 {
		return 0
	}
	return dcg / idcg
}

// NoEvidencePrecision returns the fraction of abstention queries (given their
// retrieved result counts) that correctly retrieved zero chunks.
func NoEvidencePrecision(retrievedCounts []int) float64 {
	if len(retrievedCounts) == 0 {
		return 0
	}
	correct := 0
	for _, n := range retrievedCounts {
		if n == 0 {
			correct++
		}
	}
	return float64(correct) / float64(len(retrievedCounts))
}

func toSet(ids []string) map[string]bool {
	m := make(map[string]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}
