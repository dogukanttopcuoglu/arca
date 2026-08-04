package eval

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// ComputeFingerprint derives the corpus fingerprint: SHA-256 over the sorted
// ContentHash values joined with newlines. Deterministic regardless of
// indexing order (ADR-0027).
func ComputeFingerprint(contentHashes []string) string {
	sorted := append([]string(nil), contentHashes...)
	sort.Strings(sorted)
	h := sha256.Sum256([]byte(strings.Join(sorted, "\n")))
	return hex.EncodeToString(h[:])
}

// FingerprintSource provides the content hashes of the indexed corpus so the
// harness can verify the gold set's declared fingerprint. Implementations read
// from the live vector store (ListPoints); tests inject fakes.
type FingerprintSource interface {
	// ContentHashes returns the ContentHash of every indexed chunk for the
	// given document, in any order.
	ContentHashes(documentID string) ([]string, error)
}

// VerifyFingerprint computes the live fingerprint from the source's content
// hashes and compares it against the gold set's declared value. For a
// multi-document gold set every declared document is verified individually.
// It returns the live hashes of the whole corpus (in declaration order, for
// report population) and an error describing any mismatch.
func VerifyFingerprint(source FingerprintSource, gs *GoldSet) ([]string, error) {
	if source == nil {
		return nil, fmt.Errorf("fingerprint source is nil")
	}
	var all []string
	if len(gs.Documents) > 0 {
		for _, doc := range gs.Documents {
			hashes, err := source.ContentHashes(doc.DocumentID)
			if err != nil {
				return nil, fmt.Errorf("failed to read corpus hashes: %w", err)
			}
			live := ComputeFingerprint(hashes)
			if live != doc.CorpusFingerprint {
				return nil, fmt.Errorf(
					"corpus fingerprint mismatch for %q: gold set declares %s, live index is %s (%d chunks)",
					doc.DocumentID, doc.CorpusFingerprint, live, len(hashes),
				)
			}
			all = append(all, hashes...)
		}
		return all, nil
	}
	hashes, err := source.ContentHashes(gs.Corpus.DocumentID)
	if err != nil {
		return nil, fmt.Errorf("failed to read corpus hashes: %w", err)
	}
	live := ComputeFingerprint(hashes)
	if live != gs.Corpus.CorpusFingerprint {
		return nil, fmt.Errorf(
			"corpus fingerprint mismatch: gold set declares %s, live index is %s (%d chunks)",
			gs.Corpus.CorpusFingerprint, live, len(hashes),
		)
	}
	return hashes, nil
}
