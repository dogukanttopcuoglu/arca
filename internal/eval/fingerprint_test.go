package eval_test

import (
	"testing"

	"arca/internal/eval"
)

func TestComputeFingerprint(t *testing.T) {
	// Known input: three distinct hashes. Expected digest computed
	// independently with sha256sum over the sorted+joined bytes.
	hashes := []string{
		"1111111111111111111111111111111111111111111111111111111111111111",
		"3333333333333333333333333333333333333333333333333333333333333333",
		"2222222222222222222222222222222222222222222222222222222222222222",
	}
	// Sorted: 1111..., 2222..., 3333... joined with "\n" (no trailing newline).
	// sha256 computed independently with sha256sum.
	const want = "bc1b8e4a7c0ce3149fb12980544f5bb2118685632b7139bc95edb218f0704a5e"

	if got := eval.ComputeFingerprint(hashes); got != want {
		t.Errorf("expected fingerprint %s, got %s", want, got)
	}
}

func TestComputeFingerprintDeterministic(t *testing.T) {
	hashes := []string{"b", "a", "c"}
	if got := eval.ComputeFingerprint(hashes); got != eval.ComputeFingerprint([]string{"c", "a", "b"}) {
		t.Error("fingerprint must be independent of input order")
	}
	if got := eval.ComputeFingerprint(hashes); got == eval.ComputeFingerprint([]string{"b", "a", "d"}) {
		t.Error("fingerprint must change when content changes")
	}
}
