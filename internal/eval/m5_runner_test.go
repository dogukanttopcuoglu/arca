package eval_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"arca/internal/eval"
	"arca/internal/qa"
	qacontext "arca/internal/qa/context"
	retrievalseam "arca/internal/retrieval/seam"
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
