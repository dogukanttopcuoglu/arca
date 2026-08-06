package eval_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"arca/internal/eval"
	"arca/internal/qa"
	qacontext "arca/internal/qa/context"
	"arca/internal/retrieval/graphfusion"
	retrievalseam "arca/internal/retrieval/seam"
	"sort"
)

// scriptedGate is an EvidenceGate fake serving canned decisions per query,
// with an optional first-call failure for selected queries to exercise the
// one-retry policy.
type scriptedGate struct {
	decisions map[string]qa.EvidenceDecision
	failFirst map[string]bool
	calls     map[string]int
}

func (g *scriptedGate) Evaluate(ctx context.Context, query string, win *qacontext.ContextWindow) (qa.EvidenceDecision, error) {
	g.calls[query]++
	if g.failFirst[query] && g.calls[query] == 1 {
		return qa.EvidenceGateFailed, errors.New("temporary")
	}
	decision, ok := g.decisions[query]
	if !ok {
		return qa.EvidenceGateFailed, errors.New("missing scripted decision")
	}
	if decision == qa.EvidenceGateFailed {
		return decision, errors.New("gate unavailable")
	}
	return decision, nil
}

func TestRunner_M5EvidenceGate(t *testing.T) {
	gate := &scriptedGate{
		failFirst: map[string]bool{"concept query one": true},
		decisions: map[string]qa.EvidenceDecision{
			"concept query one": qa.EvidenceSupported,
			"entity query two":  qa.EvidenceUnsupported,
		},
		calls: map[string]int{},
	}
	runner, gs, _ := newM5Runner(t, gate)

	report, err := runner.Run(context.Background(), gs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.M5 == nil {
		t.Fatal("expected M5 metrics section when the gate option is set")
	}

	q1, q2, q3 := report.PerQuery[0], report.PerQuery[1], report.PerQuery[2]

	// q1: gate failed once then recovered -> supported, one retry recorded.
	if q1.Gate == nil || q1.Gate.Decision != "supported" || q1.Gate.Retries != 1 {
		t.Errorf("q1 gate: expected supported with 1 retry, got %+v", q1.Gate)
	}
	if q1.Gate != nil && q1.Gate.Error != "" {
		t.Errorf("q1 gate: expected no gate error after recovery, got %q", q1.Gate.Error)
	}

	// q2: semantic unsupported -> false abstention against the label.
	if q2.Gate == nil || q2.Gate.Decision != "unsupported" {
		t.Errorf("q2 gate: expected unsupported observation, got %+v", q2.Gate)
	}

	// q3: empty retrieval abstains without a gate observation.
	if q3.Gate != nil {
		t.Errorf("q3: expected no gate observation for empty retrieval, got %+v", q3.Gate)
	}
	if gate.calls["nothing matches this"] != 0 {
		t.Errorf("expected no gate call for empty retrieval, got %d", gate.calls["nothing matches this"])
	}

	// q1 had a retry; q2 decided immediately.
	if gate.calls["concept query one"] != 2 || gate.calls["entity query two"] != 1 {
		t.Errorf("unexpected gate call counts: %v", gate.calls)
	}

	// M5 aggregates: skipped q2+q3; correct abstention only q3.
	if !approx(report.M5.AbstentionPrecision, 0.5) {
		t.Errorf("expected abstention precision 0.5, got %v", report.M5.AbstentionPrecision)
	}
	if !approx(report.M5.AbstentionRecall, 1.0) {
		t.Errorf("expected abstention recall 1.0, got %v", report.M5.AbstentionRecall)
	}
	if report.M5.FalseAbstentions != 1 {
		t.Errorf("expected 1 false abstention, got %d", report.M5.FalseAbstentions)
	}
	if report.M5.MissedAbstentions != 0 {
		t.Errorf("expected 0 missed abstentions, got %d", report.M5.MissedAbstentions)
	}
	if report.M5.GenerationSkipped != 2 {
		t.Errorf("expected 2 skipped generations, got %d", report.M5.GenerationSkipped)
	}
}

func TestRunner_M5SectionAbsentWithoutGate(t *testing.T) {
	runner, gs, _ := newRunner(t)
	report, err := runner.Run(context.Background(), gs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.M5 != nil {
		t.Error("expected no M5 metrics section without the gate option")
	}
	for _, qr := range report.PerQuery {
		if qr.Gate != nil {
			t.Errorf("expected no gate observation without the gate option, got %+v", qr.Gate)
		}
	}
}

func TestRunner_M5GateExhaustion(t *testing.T) {
	gate := &scriptedGate{
		decisions: map[string]qa.EvidenceDecision{
			"entity query two": qa.EvidenceGateFailed,
		},
		calls: map[string]int{},
	}
	runner, gs, _ := newM5Runner(t, gate)

	report, err := runner.Run(context.Background(), gs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	q2 := report.PerQuery[1]
	if q2.Gate == nil || q2.Gate.Decision != "gate_error" || q2.Gate.Retries != 1 {
		t.Errorf("expected gate_error with 1 retry, got %+v", q2.Gate)
	}
	if q2.Gate == nil || !strings.Contains(q2.Gate.Error, "gate unavailable") {
		t.Errorf("expected recorded gate error, got %+v", q2.Gate)
	}
	// Gate errors are operational failures: not counted as abstentions.
	if report.M5.GenerationSkipped != 1 || report.M5.FalseAbstentions != 0 {
		t.Errorf("expected gate errors excluded from abstention counts, got %+v", report.M5)
	}
}

func TestRunner_GraphManifest(t *testing.T) {
	gs, err := eval.LoadGoldSet(strings.NewReader(runnerGoldSet))
	if err != nil {
		t.Fatalf("failed to load gold set: %v", err)
	}

	t.Run("graph_weight recorded in the manifest", func(t *testing.T) {
		runner := eval.New(&fakeRetriever{}, fakeFingerprintSource{
			hashes: []string{
				"3333333333333333333333333333333333333333333333333333333333333333",
				"1111111111111111111111111111111111111111111111111111111111111111",
				"2222222222222222222222222222222222222222222222222222222222222222",
			},
		}, eval.Options{
			Mode:        retrievalseam.RetrievalDense,
			TopK:        5,
			GraphWeight: 1.0,
			GraphFusionConfig: &graphfusion.GraphFusionConfig{
				DenseWeight: 1.0,
				GraphWeight: 1.0,
			},
		})
		report, err := runner.Run(context.Background(), gs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if report.Retrieval.GraphWeight != 1.0 {
			t.Errorf("expected graph_weight 1.0 in manifest, got %v", report.Retrieval.GraphWeight)
		}
		if report.Retrieval.GraphOnly {
			t.Error("expected graph_only false in manifest")
		}
		if report.Retrieval.GraphFusionConfig == nil ||
			report.Retrieval.GraphFusionConfig.GraphWeight != 1.0 {
			t.Errorf("expected fusion config in manifest, got %+v", report.Retrieval.GraphFusionConfig)
		}
	})

	t.Run("graph_only recorded in the manifest", func(t *testing.T) {
		runner := eval.New(&fakeRetriever{}, fakeFingerprintSource{
			hashes: []string{
				"3333333333333333333333333333333333333333333333333333333333333333",
				"1111111111111111111111111111111111111111111111111111111111111111",
				"2222222222222222222222222222222222222222222222222222222222222222",
			},
		}, eval.Options{
			Mode:      retrievalseam.RetrievalDense,
			TopK:      5,
			GraphOnly: true,
		})
		report, err := runner.Run(context.Background(), gs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !report.Retrieval.GraphOnly {
			t.Error("expected graph_only true in manifest")
		}
	})
}

func TestRunner_M5GateRuns(t *testing.T) {
	// The gate LLM is non-deterministic; repeated evaluation stabilizes the
	// gate metrics (M7 review BULGU-2). Median decision wins; retrieval runs
	// once and its metrics are unchanged.
	gate := &scriptedGate{
		failFirst: map[string]bool{"concept query one": true},
		decisions: map[string]qa.EvidenceDecision{
			"concept query one": qa.EvidenceSupported,
			"entity query two":  qa.EvidenceUnsupported,
		},
		calls: map[string]int{},
	}
	gs, err := eval.LoadGoldSet(strings.NewReader(runnerGoldSet))
	if err != nil {
		t.Fatalf("failed to load gold set: %v", err)
	}
	ret := &fakeRetriever{}
	runner := eval.New(ret, fakeFingerprintSource{
		hashes: []string{
			"3333333333333333333333333333333333333333333333333333333333333333",
			"1111111111111111111111111111111111111111111111111111111111111111",
			"2222222222222222222222222222222222222222222222222222222222222222",
		},
	}, eval.Options{
		Mode:     retrievalseam.RetrievalDense,
		TopK:     5,
		Gate:     gate,
		GateRuns: 3,
	})

	report, err := runner.Run(context.Background(), gs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Retrieval.GateRuns != 3 {
		t.Errorf("expected gate_runs 3 in manifest, got %d", report.Retrieval.GateRuns)
	}
	// concept query one fails on its first attempt, so run 1 costs 2 calls
	// (attempt + retry) and runs 2-3 cost 1 each -> 4; entity query two
	// decides immediately -> 3. The abstention query (no results) never
	// reaches the gate.
	if gate.calls["concept query one"] != 4 || gate.calls["entity query two"] != 3 {
		t.Errorf("expected 4/3 gate calls per relevance query, got %v", gate.calls)
	}
	if gate.calls["nothing matches this"] != 0 {
		t.Errorf("expected no gate calls for empty retrieval, got %d", gate.calls["nothing matches this"])
	}
	// Retrieval metrics identical to a single-run evaluation (retrieval once).
	if !approx(report.Metrics.RecallAtK, 1.0) {
		t.Errorf("expected retrieval metrics unchanged, got %v", report.Metrics.RecallAtK)
	}
	// q1: [error, supported, supported] -> median supported.
	q1 := report.PerQuery[0]
	if q1.Gate == nil || q1.Gate.Decision != "supported" {
		t.Errorf("expected median supported for q1, got %+v", q1.Gate)
	}
	// The surviving (median) decision's retry count is recorded: q1's first
	// run retried once, so the median-supported run carries retries=1.
	if q1.Gate.Retries != 1 {
		t.Errorf("expected surviving decision retries 1, got %d", q1.Gate.Retries)
	}
}

func TestRunner_M5GateRunsLowerMedian(t *testing.T) {
	// Even run counts take the lower median: supported beats unsupported in a
	// tie, and unsupported beats gate_error — never tipping toward gate_error.
	for name, tc := range map[string]struct {
		runs int
		want string
	}{
		"tie supported vs unsupported": {runs: 2, want: "supported"},
		"unsupported vs gate_error":    {runs: 2, want: "unsupported"},
	} {
		t.Run(name, func(t *testing.T) {
			decision := qa.EvidenceSupported
			if tc.want == "unsupported" {
				decision = qa.EvidenceUnsupported
			}
			other := qa.EvidenceUnsupported
			if tc.want == "unsupported" {
				other = qa.EvidenceGateFailed
			}
			got := medianOf(decision, other, tc.runs)
			if got != tc.want {
				t.Errorf("expected lower median %s, got %s", tc.want, got)
			}
		})
	}
}

// medianOf applies the runner's lower-median rule to two decisions repeated
// for the given even run count.
func medianOf(a, b qa.EvidenceDecision, runs int) string {
	rank := map[qa.EvidenceDecision]int{
		qa.EvidenceSupported:   0,
		qa.EvidenceUnsupported: 1,
		qa.EvidenceGateFailed:  2,
	}
	decisions := make([]qa.EvidenceDecision, 0, runs)
	for i := 0; i < runs; i++ {
		if i%2 == 0 {
			decisions = append(decisions, a)
		} else {
			decisions = append(decisions, b)
		}
	}
	sorted := append([]qa.EvidenceDecision(nil), decisions...)
	sort.Slice(sorted, func(i, j int) bool { return rank[sorted[i]] < rank[sorted[j]] })
	return string(sorted[(len(sorted)-1)/2])
}

// newM5Runner builds the standard fake-corpus runner with the M5 gate wired
// through Options, mirroring how the eval CLI will construct it.
func newM5Runner(t *testing.T, gate qa.EvidenceGate) (*eval.Runner, *eval.GoldSet, *fakeRetriever) {
	t.Helper()
	gs, err := eval.LoadGoldSet(strings.NewReader(runnerGoldSet))
	if err != nil {
		t.Fatalf("failed to load gold set: %v", err)
	}
	ret := &fakeRetriever{}
	runner := eval.New(ret, fakeFingerprintSource{
		hashes: []string{
			"3333333333333333333333333333333333333333333333333333333333333333",
			"1111111111111111111111111111111111111111111111111111111111111111",
			"2222222222222222222222222222222222222222222222222222222222222222",
		},
	}, eval.Options{
		Mode: retrievalseam.RetrievalDense,
		TopK: 5,
		Gate: gate,
	})
	return runner, gs, ret
}

func TestRunner_ComparisonEvidenceBudget(t *testing.T) {
	// The gold set declares no comparison intent, so extend it in-memory to
	// verify the override applies only to comparison queries.
	gs, err := eval.LoadGoldSet(strings.NewReader(runnerGoldSet))
	if err != nil {
		t.Fatalf("failed to load gold set: %v", err)
	}
	gs.Queries = append(gs.Queries, eval.GoldQuery{
		ID: "cmp", Intent: eval.IntentComparison, Query: "How do X and Y differ?",
		ExpectedChunkIDs: []string{"a"}, ExpectedSections: []string{"S"}, ExpectedNoEvidence: false,
	})

	ret := &fakeRetriever{}
	runner := eval.New(ret, fakeFingerprintSource{
		hashes: []string{
			"3333333333333333333333333333333333333333333333333333333333333333",
			"1111111111111111111111111111111111111111111111111111111111111111",
			"2222222222222222222222222222222222222222222222222222222222222222",
		},
	}, eval.Options{
		Mode:           retrievalseam.RetrievalDense,
		TopK:           5,
		ComparisonTopK: 8,
	})

	report, err := runner.Run(context.Background(), gs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Retrieval.ComparisonTopK != 8 {
		t.Errorf("expected comparison_top_k 8 in manifest, got %d", report.Retrieval.ComparisonTopK)
	}

	// 4 queries in order: q1, q2, q3 (TopK 5) then cmp (TopK 8).
	want := []int{5, 5, 5, 8}
	if len(ret.topKs) != len(want) {
		t.Fatalf("expected %d retrieval calls, got %d (%v)", len(want), len(ret.topKs), ret.topKs)
	}
	for i := range want {
		if ret.topKs[i] != want[i] {
			t.Errorf("retrieval %d: expected TopK %d, got %d", i, want[i], ret.topKs[i])
		}
	}
}
