package qa_test

import (
	"context"
	"errors"
	"testing"

	"arca/internal/indexing/provider"
	"arca/internal/indexing/store"
	llmprovider "arca/internal/llm/provider"
	"arca/internal/qa"
	qacontext "arca/internal/qa/context"
	qaprompt "arca/internal/qa/prompt"
	qaverification "arca/internal/qa/verification"
	"arca/internal/retrieval/dense"
	"arca/internal/retrieval/seam"
)

func TestAnswerEngine_EvidenceGate(t *testing.T) {
	ctx := context.Background()

	t.Run("unsupported context abstains without generation", func(t *testing.T) {
		gate := &scriptedGate{decisions: []qa.EvidenceDecision{qa.EvidenceUnsupported}}
		llm := &fakeLLM{content: "should not be generated"}
		engine := newGatedTestEngine(t, gate, llm)

		ans, err := engine.Answer(ctx, retrievalQuery("What is unrelated?"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ans.Status != qaverification.StatusNoEvidence {
			t.Fatalf("expected no_evidence, got %q", ans.Status)
		}
		if llm.calls != 0 {
			t.Fatalf("expected generation to be skipped, got %d calls", llm.calls)
		}
	})

	t.Run("gate errors retry once then fail closed with typed error", func(t *testing.T) {
		gate := &scriptedGate{
			decisions: []qa.EvidenceDecision{qa.EvidenceGateFailed, qa.EvidenceGateFailed},
			errs:      []error{errors.New("temporary"), errors.New("still unavailable")},
		}
		llm := &fakeLLM{content: "should not be generated"}
		engine := newGatedTestEngine(t, gate, llm)

		_, err := engine.Answer(ctx, retrievalQuery("What is unrelated?"))
		if err == nil {
			t.Fatal("expected evidence gate error")
		}
		var gateErr *qa.EvidenceGateError
		if !errors.As(err, &gateErr) {
			t.Fatalf("expected *qa.EvidenceGateError, got %T: %v", err, err)
		}
		if gateErr.Attempts != 2 {
			t.Fatalf("expected 2 attempts, got %d", gateErr.Attempts)
		}
		if gate.calls != 2 {
			t.Fatalf("expected one retry (2 calls), got %d", gate.calls)
		}
		if llm.calls != 0 {
			t.Fatalf("expected generation to be skipped, got %d calls", llm.calls)
		}
	})

	t.Run("transient gate failure recovers on retry", func(t *testing.T) {
		gate := &scriptedGate{
			decisions: []qa.EvidenceDecision{qa.EvidenceGateFailed, qa.EvidenceSupported},
			errs:      []error{errors.New("temporary")},
		}
		llm := &fakeLLM{content: "Grounded answer [Ref 1]."}
		engine := newGatedTestEngine(t, gate, llm)

		ans, err := engine.Answer(ctx, retrievalQuery("What is creativity?"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ans.Status != qaverification.StatusVerified {
			t.Fatalf("expected verified answer after retry, got %q", ans.Status)
		}
		if gate.calls != 2 {
			t.Fatalf("expected 2 gate calls, got %d", gate.calls)
		}
		if llm.calls != 1 {
			t.Fatalf("expected generation after recovery, got %d calls", llm.calls)
		}
	})

	t.Run("gate receives the original query and final context window", func(t *testing.T) {
		gate := &scriptedGate{decisions: []qa.EvidenceDecision{qa.EvidenceSupported}}
		engine := newGatedTestEngine(t, gate, &fakeLLM{content: "Grounded answer [Ref 1]."})

		_, err := engine.Answer(ctx, retrievalQuery("What is creativity?"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(gate.queried) != 1 || gate.queried[0] != "What is creativity?" {
			t.Fatalf("expected gate to receive the original query, got %v", gate.queried)
		}
		if len(gate.windows) != 1 || gate.windows[0].Content == "" {
			t.Fatal("expected gate to receive the final assembled context window")
		}
	})

	t.Run("empty retrieval abstains immediately without calling the gate", func(t *testing.T) {
		gate := &scriptedGate{decisions: []qa.EvidenceDecision{qa.EvidenceSupported}}
		embProvider := provider.NewMockEmbeddingProvider("mock-provider", "mock-model-v1", 1536)
		retriever := dense.NewDenseRetriever(embProvider, store.NewInMemoryVectorStore(), store.NewInMemoryContentStore())
		engine := qa.NewAnswerEngine(
			qa.NewRuleBasedAnalyzer(),
			retriever,
			qacontext.NewDefaultContextBuilder(nil, 4000),
			qaprompt.NewRAGPromptBuilder(),
			&fakeLLM{content: "should not be generated"},
			qaverification.NewDefaultVerificationPipeline(),
			gate,
		)

		ans, err := engine.Answer(ctx, retrievalQuery("No matches anywhere"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ans.Status != qaverification.StatusNoEvidence {
			t.Fatalf("expected no_evidence, got %q", ans.Status)
		}
		if gate.calls != 0 {
			t.Fatalf("expected no gate call for empty retrieval, got %d", gate.calls)
		}
	})
}

func TestLLMEvidenceGate(t *testing.T) {
	ctx := context.Background()
	win := &qacontext.ContextWindow{Content: "The source says creativity requires practice."}

	t.Run("accepts only explicit supported and unsupported decisions", func(t *testing.T) {
		provider := &scriptedLLM{contents: []string{
			`{"decision": "supported"}`,
			`{"decision": "unsupported"}`,
		}}
		gate := qa.NewLLMEvidenceGate(provider)

		decision, err := gate.Evaluate(ctx, "What does creativity require?", win)
		if err != nil || decision != qa.EvidenceSupported {
			t.Fatalf("expected supported, got %q, %v", decision, err)
		}
		decision, err = gate.Evaluate(ctx, "What does creativity require?", win)
		if err != nil || decision != qa.EvidenceUnsupported {
			t.Fatalf("expected unsupported, got %q, %v", decision, err)
		}
	})

	t.Run("malformed output is an operational gate error", func(t *testing.T) {
		gate := qa.NewLLMEvidenceGate(&scriptedLLM{contents: []string{`{"decision": "maybe"}`}})

		decision, err := gate.Evaluate(ctx, "What does creativity require?", win)
		if decision != qa.EvidenceGateFailed || err == nil {
			t.Fatalf("expected gate_error with error, got %q, %v", decision, err)
		}
	})

	t.Run("non-JSON output is an operational gate error", func(t *testing.T) {
		gate := qa.NewLLMEvidenceGate(&scriptedLLM{contents: []string{"supported, obviously"}})

		decision, err := gate.Evaluate(ctx, "What does creativity require?", win)
		if decision != qa.EvidenceGateFailed || err == nil {
			t.Fatalf("expected gate_error with error, got %q, %v", decision, err)
		}
	})

	t.Run("empty provider output is an operational gate error", func(t *testing.T) {
		gate := qa.NewLLMEvidenceGate(&scriptedLLM{contents: []string{""}})

		decision, err := gate.Evaluate(ctx, "What does creativity require?", win)
		if decision != qa.EvidenceGateFailed || err == nil {
			t.Fatalf("expected gate_error with error, got %q, %v", decision, err)
		}
	})

	t.Run("provider failure surfaces as an operational gate error", func(t *testing.T) {
		gate := qa.NewLLMEvidenceGate(&scriptedLLM{err: errors.New("provider down")})

		decision, err := gate.Evaluate(ctx, "What does creativity require?", win)
		if decision != qa.EvidenceGateFailed || err == nil {
			t.Fatalf("expected gate_error with error, got %q, %v", decision, err)
		}
	})

	t.Run("empty query or context is an operational gate error", func(t *testing.T) {
		gate := qa.NewLLMEvidenceGate(&scriptedLLM{contents: []string{`{"decision": "supported"}`}})

		if _, err := gate.Evaluate(ctx, "  ", win); err == nil {
			t.Error("expected error for empty query")
		}
		if _, err := gate.Evaluate(ctx, "What does creativity require?", nil); err == nil {
			t.Error("expected error for nil context")
		}
		if _, err := gate.Evaluate(ctx, "What does creativity require?", &qacontext.ContextWindow{}); err == nil {
			t.Error("expected error for empty context")
		}
	})
}

func retrievalQuery(text string) seam.RetrievalQuery {
	return seam.RetrievalQuery{QueryText: text, TopK: 5}
}

// newGatedTestEngine constructs an AnswerEngine with a seeded retriever, a
// configurable gate, and a counting fake LLM.
func newGatedTestEngine(t *testing.T, gate qa.EvidenceGate, llm llmprovider.LLMProvider) *qa.AnswerEngine {
	t.Helper()
	ctx := context.Background()
	embProvider := provider.NewMockEmbeddingProvider("mock-provider", "mock-model-v1", 1536)
	vecStore := store.NewInMemoryVectorStore()
	contentStore := store.NewInMemoryContentStore()
	seedChunk(ctx, t, embProvider, vecStore, contentStore, "chk-1", "Creativity",
		"Creativity is a fundamental human quality that every person can develop.")
	retriever := dense.NewDenseRetriever(embProvider, vecStore, contentStore)
	return qa.NewAnswerEngine(
		qa.NewRuleBasedAnalyzer(),
		retriever,
		qacontext.NewDefaultContextBuilder(nil, 4000),
		qaprompt.NewRAGPromptBuilder(),
		llm,
		qaverification.NewDefaultVerificationPipeline(),
		gate,
	)
}

// scriptedGate is a deterministic EvidenceGate fake recording calls for seam
// assertions; it never consults a provider.
type scriptedGate struct {
	decisions []qa.EvidenceDecision
	errs      []error
	calls     int
	queried   []string
	windows   []*qacontext.ContextWindow
}

func (g *scriptedGate) Evaluate(ctx context.Context, query string, win *qacontext.ContextWindow) (qa.EvidenceDecision, error) {
	idx := g.calls
	g.calls++
	g.queried = append(g.queried, query)
	g.windows = append(g.windows, win)
	if idx >= len(g.decisions) {
		return qa.EvidenceGateFailed, errors.New("missing scripted decision")
	}
	var err error
	if idx < len(g.errs) {
		err = g.errs[idx]
	}
	return g.decisions[idx], err
}

// scriptedLLM returns canned raw responses for EvidenceGate adapter tests.
type scriptedLLM struct {
	contents []string
	err      error
	calls    int
}

func (p *scriptedLLM) Generate(ctx context.Context, prompt qaprompt.PromptMessage) (*llmprovider.LLMResponse, error) {
	if p.err != nil {
		return nil, p.err
	}
	if p.calls >= len(p.contents) {
		return nil, errors.New("missing scripted response")
	}
	content := p.contents[p.calls]
	p.calls++
	return &llmprovider.LLMResponse{Content: content, Provider: "scripted", Model: "scripted"}, nil
}

func (p *scriptedLLM) Stream(ctx context.Context, prompt qaprompt.PromptMessage) (<-chan llmprovider.StreamChunk, error) {
	return nil, errors.New("not implemented")
}

func (p *scriptedLLM) Capabilities() llmprovider.ModelCapabilities {
	return llmprovider.ModelCapabilities{ContextWindow: 128000}
}
