package probe

import (
	"testing"
)

// gateFixture builds a report whose reranked combinations are strictly
// better than the baseline (nDCG 0.879 -> 0.91 = +3.1 pp), MRR preserved,
// abstention aligned.
func gateFixture() *ProbeReport {
	return &ProbeReport{
		ArtifactFingerprint: "fp-a",
		TopK:                5,
		Baseline:            BaselineResult{NDCGAt5: 0.879, MRR: 0.938, VerifiedRate: 0.90, GateEvaluations: 20},
		Combinations: []CombinationResult{
			{
				Model: "bge", CandidateN: 20,
				NDCGAt5: 0.91, MRR: 0.94, AbstentionAligned: true,
				P95LatencyMs: 120, MaxRSSBytes: 600 * 1024 * 1024,
				VerifiedRate: 0.92, GateEvaluations: 20,
			},
			{
				Model: "bge", CandidateN: 50,
				NDCGAt5: 0.912, MRR: 0.94, AbstentionAligned: true,
				P95LatencyMs: 200, MaxRSSBytes: 600 * 1024 * 1024,
				VerifiedRate: 0.91, GateEvaluations: 20,
			},
		},
	}
}

func TestGateAcceptsBestCombination(t *testing.T) {
	rep := gateFixture()
	out := Evaluate(rep, Budget{MaxRerankP95Ms: 500, MaxRSSBytes: 1 << 30})

	if !out.Accepted {
		t.Fatalf("expected acceptance, got reject: %s", out.Reason)
	}
	if out.SelectedModel != "bge" || out.SelectedN != 20 {
		t.Fatalf("selection = %s N=%d, want smallest N within 5%% tolerance of the best nDCG", out.SelectedModel, out.SelectedN)
	}
}

func TestGateRejectsBelowMPI(t *testing.T) {
	rep := gateFixture()
	// nDCG 0.879 -> 0.885 = +0.6 pp, below the +1 pp MPI.
	rep.Combinations[0].NDCGAt5 = 0.885
	rep.Combinations[1].NDCGAt5 = 0.885

	out := Evaluate(rep, Budget{MaxRerankP95Ms: 500, MaxRSSBytes: 1 << 30})
	if out.Accepted {
		t.Fatalf("expected rejection below MPI, got acceptance: %s", out.Reason)
	}
}

func TestGateRejectsMRRRegression(t *testing.T) {
	rep := gateFixture()
	// MRR 0.938 -> 0.90 = -3.8 pp, beyond the -0.5 pp MAR.
	rep.Combinations[0].MRR = 0.90
	rep.Combinations[1].MRR = 0.90

	out := Evaluate(rep, Budget{MaxRerankP95Ms: 500, MaxRSSBytes: 1 << 30})
	if out.Accepted {
		t.Fatalf("expected rejection on MRR regression, got acceptance: %s", out.Reason)
	}
}

func TestGateRejectsVerifiedRegression(t *testing.T) {
	rep := gateFixture()
	// Verified 0.90 -> 0.86 = -4 pp, beyond the -1 pp MAR.
	rep.Combinations[0].VerifiedRate = 0.86
	rep.Combinations[1].VerifiedRate = 0.86

	out := Evaluate(rep, Budget{MaxRerankP95Ms: 500, MaxRSSBytes: 1 << 30})
	if out.Accepted {
		t.Fatalf("expected rejection on verified regression, got acceptance: %s", out.Reason)
	}
}

func TestGateRejectsAbstentionViolation(t *testing.T) {
	rep := gateFixture()
	rep.Combinations[0].AbstentionAligned = false
	rep.Combinations[1].AbstentionAligned = false

	out := Evaluate(rep, Budget{MaxRerankP95Ms: 500, MaxRSSBytes: 1 << 30})
	if out.Accepted {
		t.Fatalf("abstention is a hard invariant: expected rejection, got acceptance: %s", out.Reason)
	}
}

func TestGateSkipsOnlyViolatingCombination(t *testing.T) {
	rep := gateFixture()
	// Only the N=50 combination violates abstention; N=20 still accepts.
	rep.Combinations[1].AbstentionAligned = false

	out := Evaluate(rep, Budget{MaxRerankP95Ms: 500, MaxRSSBytes: 1 << 30})
	if !out.Accepted {
		t.Fatalf("expected N=20 acceptance, got reject: %s", out.Reason)
	}
	if out.SelectedN != 20 {
		t.Fatalf("selection = N=%d, want 20", out.SelectedN)
	}
}

func TestGateRejectsOperationalBudget(t *testing.T) {
	rep := gateFixture()
	out := Evaluate(rep, Budget{MaxRerankP95Ms: 100, MaxRSSBytes: 1 << 30})

	if out.Accepted {
		t.Fatalf("expected rejection on latency budget, got acceptance: %s", out.Reason)
	}
}

func TestGateNoneAccepted(t *testing.T) {
	rep := gateFixture()
	rep.Combinations[0].NDCGAt5 = 0.885
	rep.Combinations[1].NDCGAt5 = 0.885
	rep.Combinations[0].MRR = 0.90
	rep.Combinations[1].MRR = 0.90

	out := Evaluate(rep, Budget{MaxRerankP95Ms: 500, MaxRSSBytes: 1 << 30})
	if out.Accepted {
		t.Fatalf("expected none-accepted, got acceptance: %s", out.Reason)
	}
	if out.SelectedModel != "" || out.SelectedN != 0 {
		t.Fatalf("selection = %s N=%d, want empty on rejection", out.SelectedModel, out.SelectedN)
	}
}
