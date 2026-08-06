package probe

// Kill gate thresholds (ADR-0045), frozen before the benchmark starts:
// baseline-relative, never absolute metric targets. Named in the ADR so
// "why +1?" is never reopened.
const (
	// MPI is the minimum practical improvement: nDCG@5 delta in percentage
	// points over baseline (a product/engineering threshold, not a
	// statistical construct).
	MPI = 1.0
	// MARMRR is the maximum acceptable regression for MRR (first correct
	// result must not degrade beyond measurement noise).
	MARMRR = 0.5
	// MARVerified is the maximum acceptable regression for the gate
	// verified rate (answer quality must not regress).
	MARVerified = 1.0
	// selectionTolerance is the relative nDCG difference inside which the
	// smallest candidate budget wins (deterministic selection rule).
	selectionTolerance = 0.05
)

// Budget is the operational acceptance ceiling (ADR-0045): values are frozen
// in the benchmark manifest before the probe starts.
type Budget struct {
	MaxRerankP95Ms int64
	MaxRSSBytes    int64
}

// Outcome is the deterministic kill gate verdict (ADR-0045).
type Outcome struct {
	Accepted      bool
	Reason        string
	SelectedModel string
	SelectedN     int
}

// Evaluate applies the frozen acceptance thresholds to the probe report and
// selects the winning combination deterministically: highest quality among
// accepted combinations; within the selection tolerance, the smallest N.
// "None accepted" is a first-class outcome — no forced winner.
func Evaluate(rep *ProbeReport, budget Budget) Outcome {
	best := -1
	bestNDCG := 0.0
	for i := range rep.Combinations {
		c := &rep.Combinations[i]
		if !c.AbstentionAligned {
			continue
		}
		ndcgDeltaPp := (c.NDCGAt5 - rep.Baseline.NDCGAt5) * 100
		if ndcgDeltaPp < MPI {
			continue
		}
		if (c.MRR-rep.Baseline.MRR)*100 < -MARMRR {
			continue
		}
		if c.GateEvaluations > 0 && rep.Baseline.GateEvaluations > 0 {
			if (c.VerifiedRate-rep.Baseline.VerifiedRate)*100 < -MARVerified {
				continue
			}
		}
		if c.P95LatencyMs > float64(budget.MaxRerankP95Ms) || c.MaxRSSBytes > budget.MaxRSSBytes {
			continue
		}
		if best == -1 || c.NDCGAt5 > bestNDCG*(1+selectionTolerance) {
			best = i
			bestNDCG = c.NDCGAt5
		} else if c.NDCGAt5 >= bestNDCG && rep.Combinations[best].CandidateN > c.CandidateN {
			// Within tolerance of the best nDCG: the smallest N wins.
			best = i
			bestNDCG = c.NDCGAt5
		}
	}

	if best == -1 {
		return Outcome{
			Accepted: false,
			Reason:   "no combination passes the frozen thresholds (MPI/MAR/abstention/budget)",
		}
	}
	w := rep.Combinations[best]
	return Outcome{
		Accepted:      true,
		Reason:        "benchmark acceptance (ADR-0045)",
		SelectedModel: w.Model,
		SelectedN:     w.CandidateN,
	}
}
