package qa_test

import (
	"context"
	"testing"

	indexingmodel "arca/internal/indexing/model"
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

func TestRuleBasedQueryAnalyzer(t *testing.T) {
	ctx := context.Background()
	analyzer := qa.NewRuleBasedAnalyzer()

	t.Run("analyzes query text and extracts basic intent", func(t *testing.T) {
		analyzed, err := analyzer.Analyze(ctx, "What is creativity according to Rick Rubin?")
		if err != nil {
			t.Fatalf("unexpected error analyzing query: %v", err)
		}

		if analyzed.RawQuery != "What is creativity according to Rick Rubin?" {
			t.Errorf("expected RawQuery to match input, got %q", analyzed.RawQuery)
		}
		if analyzed.Intent == "" {
			t.Error("expected non-empty intent")
		}
	})

	t.Run("empty query string returns validation error", func(t *testing.T) {
		_, err := analyzer.Analyze(ctx, "")
		if err == nil {
			t.Error("expected error for empty query string, got nil")
		}
	})
}

func TestAnswerEngine_RealPipeline(t *testing.T) {
	ctx := context.Background()
	mockProvider := provider.NewMockEmbeddingProvider("mock-provider", "mock-model-v1", 1536)

	t.Run("returns a verified Answer with citations and metadata", func(t *testing.T) {
		vecStore := store.NewInMemoryVectorStore()
		contentStore := store.NewInMemoryContentStore()
		seedChunk(ctx, t, mockProvider, vecStore, contentStore, "chk-1", "Introduction",
			"Creativity is a fundamental human quality.")

		retriever := dense.NewDenseRetriever(mockProvider, vecStore, contentStore)
		llm := &fakeLLM{content: "Creativity is a discipline [Ref 1]."}
		engine := newTestEngine(retriever, llm, 4000)

		ans, err := engine.Answer(ctx, seam.RetrievalQuery{QueryText: "What is creativity?", TopK: 5})
		if err != nil {
			t.Fatalf("unexpected error during Answer execution: %v", err)
		}
		if ans == nil {
			t.Fatal("expected non-nil Answer")
		}
		if ans.Status != qaverification.StatusVerified {
			t.Errorf("expected Status %q, got %q", qaverification.StatusVerified, ans.Status)
		}
		if len(ans.Citations) != 1 {
			t.Fatalf("expected 1 citation, got %d", len(ans.Citations))
		}
		if ans.Citations[0].ChunkID != "chk-1" {
			t.Errorf("expected citation for chunk chk-1, got %q", ans.Citations[0].ChunkID)
		}
		if ans.Text != "Creativity is a discipline [Ref 1]." {
			t.Errorf("expected LLM text preserved, got %q", ans.Text)
		}
		if ans.Metadata.Provider != "fake-provider" || ans.Metadata.Model != "fake-model" {
			t.Errorf("expected fake provider metadata, got %s/%s", ans.Metadata.Provider, ans.Metadata.Model)
		}
		if ans.Metadata.Usage == nil || ans.Metadata.Usage.TotalTokens != 42 {
			t.Errorf("expected token usage from LLM, got %+v", ans.Metadata.Usage)
		}
		if llm.calls != 1 {
			t.Errorf("expected exactly 1 LLM call, got %d", llm.calls)
		}
	})

	t.Run("returns no_evidence without invoking the LLM when nothing is retrieved", func(t *testing.T) {
		retriever := dense.NewDenseRetriever(mockProvider, store.NewInMemoryVectorStore(), store.NewInMemoryContentStore())
		llm := &fakeLLM{content: "Should never be generated [Ref 1]."}
		engine := newTestEngine(retriever, llm, 4000)

		ans, err := engine.Answer(ctx, seam.RetrievalQuery{QueryText: "Nothing here matches", TopK: 5})
		if err != nil {
			t.Fatalf("unexpected error during Answer execution: %v", err)
		}
		if ans.Status != qaverification.StatusNoEvidence {
			t.Errorf("expected Status %q, got %q", qaverification.StatusNoEvidence, ans.Status)
		}
		if len(ans.Citations) != 0 {
			t.Errorf("expected no citations, got %d", len(ans.Citations))
		}
		if ans.Text == "" {
			t.Error("expected a non-empty sources-don't-cover message")
		}
		if llm.calls != 0 {
			t.Errorf("expected LLM to never be called, got %d calls", llm.calls)
		}
	})

	t.Run("abstains when every retrieved result falls below the score threshold", func(t *testing.T) {
		vecStore := store.NewInMemoryVectorStore()
		contentStore := store.NewInMemoryContentStore()
		seedChunk(ctx, t, mockProvider, vecStore, contentStore, "chk-1", "Introduction",
			"Creativity is a fundamental human quality.")

		retriever := dense.NewDenseRetriever(mockProvider, vecStore, contentStore)
		llm := &fakeLLM{content: "Should never be generated [Ref 1]."}
		engine := newTestEngine(retriever, llm, 4000)

		// Cosine similarity can never exceed 1.0, so a threshold above it
		// filters every result while the store is not empty.
		ans, err := engine.Answer(ctx, seam.RetrievalQuery{
			QueryText: "What is creativity?",
			TopK:      5,
			MinScore:  1.1,
		})
		if err != nil {
			t.Fatalf("unexpected error during Answer execution: %v", err)
		}
		if ans.Status != qaverification.StatusNoEvidence {
			t.Errorf("expected Status %q with threshold filtering all results, got %q", qaverification.StatusNoEvidence, ans.Status)
		}
		if llm.calls != 0 {
			t.Errorf("expected LLM to never be called when all results are filtered, got %d calls", llm.calls)
		}
	})

	t.Run("marks answer unverified when the LLM cites an invalid reference", func(t *testing.T) {
		vecStore := store.NewInMemoryVectorStore()
		contentStore := store.NewInMemoryContentStore()
		seedChunk(ctx, t, mockProvider, vecStore, contentStore, "chk-1", "Introduction",
			"Creativity is a fundamental human quality.")

		retriever := dense.NewDenseRetriever(mockProvider, vecStore, contentStore)
		llm := &fakeLLM{content: "A hallucinated claim [Ref 999]."}
		engine := newTestEngine(retriever, llm, 4000)

		ans, err := engine.Answer(ctx, seam.RetrievalQuery{QueryText: "What is creativity?", TopK: 5})
		if err != nil {
			t.Fatalf("unexpected error during Answer execution: %v", err)
		}
		if ans.Status != qaverification.StatusUnverified {
			t.Errorf("expected Status %q, got %q", qaverification.StatusUnverified, ans.Status)
		}
		if ans.Verification.InvalidReferences == 0 {
			t.Error("expected InvalidReferences > 0 in verification report")
		}
	})

	t.Run("marks answer unverified when the LLM cites no sources at all", func(t *testing.T) {
		vecStore := store.NewInMemoryVectorStore()
		contentStore := store.NewInMemoryContentStore()
		seedChunk(ctx, t, mockProvider, vecStore, contentStore, "chk-1", "Introduction",
			"Creativity is a fundamental human quality.")

		retriever := dense.NewDenseRetriever(mockProvider, vecStore, contentStore)
		llm := &fakeLLM{content: "An answer with no citation markers whatsoever."}
		engine := newTestEngine(retriever, llm, 4000)

		ans, err := engine.Answer(ctx, seam.RetrievalQuery{QueryText: "What is creativity?", TopK: 5})
		if err != nil {
			t.Fatalf("unexpected error during Answer execution: %v", err)
		}
		if ans.Status != qaverification.StatusUnverified {
			t.Errorf("expected Status %q, got %q", qaverification.StatusUnverified, ans.Status)
		}
	})

	t.Run("verifies answers citing combined reference markers", func(t *testing.T) {
		vecStore := store.NewInMemoryVectorStore()
		contentStore := store.NewInMemoryContentStore()
		seedChunk(ctx, t, mockProvider, vecStore, contentStore, "chk-1", "Introduction",
			"Creativity is a fundamental human quality.")
		seedChunk(ctx, t, mockProvider, vecStore, contentStore, "chk-2", "Practice",
			"Discipline and daily practice turn creative impulses into finished works.")

		retriever := dense.NewDenseRetriever(mockProvider, vecStore, contentStore)
		llm := &fakeLLM{content: "Creativity requires practice [Ref 1, 2]."}
		engine := newTestEngine(retriever, llm, 4000)

		ans, err := engine.Answer(ctx, seam.RetrievalQuery{QueryText: "What is creativity?", TopK: 5})
		if err != nil {
			t.Fatalf("unexpected error during Answer execution: %v", err)
		}
		if ans.Status != qaverification.StatusVerified {
			t.Errorf("expected Status %q for combined markers, got %q", qaverification.StatusVerified, ans.Status)
		}
		if len(ans.Citations) != 2 {
			t.Errorf("expected 2 citations from combined markers, got %d", len(ans.Citations))
		}
	})

	t.Run("respects the configured context budget at the engine seam", func(t *testing.T) {
		vecStore := store.NewInMemoryVectorStore()
		contentStore := store.NewInMemoryContentStore()
		seedChunk(ctx, t, mockProvider, vecStore, contentStore, "chk-1", "Introduction",
			"Creativity is a fundamental human quality that every person can develop.")
		seedChunk(ctx, t, mockProvider, vecStore, contentStore, "chk-2", "Practice",
			"Discipline and daily practice turn creative impulses into finished works.")

		retriever := dense.NewDenseRetriever(mockProvider, vecStore, contentStore)
		llm := &fakeLLM{content: "Creativity requires practice [Ref 1] and discipline [Ref 2]."}

		tiny := newTestEngine(retriever, llm, 10)
		ans, err := tiny.Answer(ctx, seam.RetrievalQuery{QueryText: "What is creativity?", TopK: 5})
		if err != nil {
			t.Fatalf("unexpected error during Answer execution: %v", err)
		}
		if len(ans.Citations) != 1 {
			t.Errorf("expected the tiny budget to admit exactly 1 source, got %d citations", len(ans.Citations))
		}
		if ans.Status != qaverification.StatusUnverified {
			t.Errorf("expected the truncated window to leave [Ref 2] unverifiable, got %q", ans.Status)
		}

		generous := newTestEngine(retriever, &fakeLLM{content: llm.content}, 4000)
		ans, err = generous.Answer(ctx, seam.RetrievalQuery{QueryText: "What is creativity?", TopK: 5})
		if err != nil {
			t.Fatalf("unexpected error during Answer execution: %v", err)
		}
		if len(ans.Citations) != 2 {
			t.Errorf("expected the generous budget to admit both sources, got %d citations", len(ans.Citations))
		}
		if ans.Status != qaverification.StatusVerified {
			t.Errorf("expected both references verifiable with full window, got %q", ans.Status)
		}
	})
}

// newTestEngine constructs an AnswerEngine with real seams for engine tests.
func newTestEngine(retriever seam.Retriever, llm llmprovider.LLMProvider, budget int) *qa.AnswerEngine {
	return qa.NewAnswerEngine(
		qa.NewRuleBasedAnalyzer(),
		retriever,
		qacontext.NewDefaultContextBuilder(nil, budget),
		qaprompt.NewRAGPromptBuilder(),
		llm,
		qaverification.NewDefaultVerificationPipeline(),
	)
}

// seedChunk embeds and stores one chunk with content for retrieval tests.
func seedChunk(ctx context.Context, t *testing.T, p provider.EmbeddingProvider, vecStore *store.InMemoryVectorStore, contentStore *store.InMemoryContentStore, chunkID, section, markdown string) {
	t.Helper()
	vec, err := p.EmbedQuery(ctx, markdown)
	if err != nil || len(vec) == 0 {
		t.Fatalf("failed to embed seed chunk: %v", err)
	}
	if err := vecStore.UpsertPoints(ctx, []store.VectorPoint{{
		ID:     "pt-" + chunkID,
		Vector: vec,
		Metadata: indexingmodel.VectorMetadata{
			DocumentID:  "doc-1",
			ChunkID:     chunkID,
			ChunkOrder:  0,
			SectionPath: section,
			ContentHash: "hash-" + chunkID,
		},
	}}); err != nil {
		t.Fatalf("failed to seed vector store: %v", err)
	}
	if err := contentStore.PutContent(ctx, []store.ChunkContent{{ChunkID: chunkID, ContentMarkdown: markdown}}); err != nil {
		t.Fatalf("failed to seed content store: %v", err)
	}
}

// fakeLLM is a configurable LLMProvider fake for engine tests; it records calls so
// tests can assert the engine's generation behavior at the seam.
type fakeLLM struct {
	content string
	calls   int
	err     error
}

func (f *fakeLLM) Generate(ctx context.Context, prompt qaprompt.PromptMessage) (*llmprovider.LLMResponse, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return &llmprovider.LLMResponse{
		Content:  f.content,
		Model:    "fake-model",
		Provider: "fake-provider",
		TokenUsage: llmprovider.LLMUsage{
			PromptTokens:     10,
			CompletionTokens: 32,
			TotalTokens:      42,
		},
	}, nil
}

func (f *fakeLLM) Stream(ctx context.Context, prompt qaprompt.PromptMessage) (<-chan llmprovider.StreamChunk, error) {
	ch := make(chan llmprovider.StreamChunk, 1)
	ch <- llmprovider.StreamChunk{Content: f.content}
	ch <- llmprovider.StreamChunk{Done: true}
	close(ch)
	return ch, nil
}

func (f *fakeLLM) Capabilities() llmprovider.ModelCapabilities {
	return llmprovider.ModelCapabilities{SupportsSystemMessage: true, SupportsStreaming: true, ContextWindow: 128000}
}
