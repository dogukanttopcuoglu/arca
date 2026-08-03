package cli

import (
	"context"
	"strings"
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
)

func TestAppRunAsk_RendersAnswer(t *testing.T) {
	ctx := context.Background()

	t.Run("renders query, answer, and Sources section for a verified answer", func(t *testing.T) {
		app := newTestApp(ctx, t, "Verified grounded answer with citation [Ref 1].")
		out, err := app.RunAsk(ctx, "What is creativity?")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(out, "Q: What is creativity?") {
			t.Errorf("expected query line, got:\n%s", out)
		}
		if !strings.Contains(out, "Verified grounded answer with citation [Ref 1].") {
			t.Errorf("expected answer text with inline marker, got:\n%s", out)
		}
		if !strings.Contains(out, "Sources:") {
			t.Errorf("expected Sources section, got:\n%s", out)
		}
		if !strings.Contains(out, "[Ref 1]") || !strings.Contains(out, "Introduction") || !strings.Contains(out, "page(s) 1") {
			t.Errorf("expected source mapping with reference, section, and page, got:\n%s", out)
		}
		if strings.Contains(out, "⚠") {
			t.Errorf("expected no warning for a verified answer, got:\n%s", out)
		}
	})

	t.Run("renders a warning for an unverified answer", func(t *testing.T) {
		app := newTestApp(ctx, t, "Hallucinated claim [Ref 99].")
		out, err := app.RunAsk(ctx, "What is creativity?")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(out, "Hallucinated claim [Ref 99].") {
			t.Errorf("expected answer text preserved, got:\n%s", out)
		}
		if !strings.Contains(out, "⚠") || !strings.Contains(out, "unverified") {
			t.Errorf("expected visible unverified warning, got:\n%s", out)
		}
	})

	t.Run("renders the sources-don't-cover message for no_evidence without a Sources section", func(t *testing.T) {
		embProvider := provider.NewMockEmbeddingProvider("mock-provider", "mock-model-v1", 1536)
		retriever := dense.NewDenseRetriever(embProvider, store.NewInMemoryVectorStore(), store.NewInMemoryContentStore())
		engine := qa.NewAnswerEngine(
			qa.NewRuleBasedAnalyzer(),
			retriever,
			qacontext.NewDefaultContextBuilder(nil, 4000),
			qaprompt.NewRAGPromptBuilder(),
			&cliFakeLLM{content: "Should never be called [Ref 1]."},
			qaverification.NewDefaultVerificationPipeline(),
		)
		app := &App{answerEngine: engine}

		out, err := app.RunAsk(ctx, "Query with no matches")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(out, "sources do not cover") {
			t.Errorf("expected sources-don't-cover message, got:\n%s", out)
		}
		if strings.Contains(out, "Sources:") {
			t.Errorf("expected no Sources section for no_evidence, got:\n%s", out)
		}
		if strings.Contains(out, "⚠") {
			t.Errorf("expected no warning for no_evidence, got:\n%s", out)
		}
	})

	t.Run("rejects an empty query", func(t *testing.T) {
		app := &App{}
		_, err := app.RunAsk(ctx, "   ")
		if err == nil {
			t.Error("expected error for empty query, got nil")
		}
	})

	t.Run("abstains when the configured threshold filters all results", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.RetrievalMinScore = 1.1 // above any cosine similarity
		runtime, err := NewRuntime(cfg)
		if err != nil {
			t.Fatalf("failed to construct runtime: %v", err)
		}
		vecStore := runtime.vectorStore.(*store.InMemoryVectorStore)
		seedTestChunk(ctx, t, runtime.embeddingProvider, vecStore, runtime.contentStore.(*store.InMemoryContentStore))

		app := NewAppWithRuntime(runtime)
		out, err := app.RunAsk(ctx, "What is creativity?")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(out, "sources do not cover") {
			t.Errorf("expected sources-don't-cover message, got:\n%s", out)
		}
		if strings.Contains(out, "Sources:") {
			t.Errorf("expected no Sources section for abstention, got:\n%s", out)
		}
		if strings.Contains(out, "⚠") {
			t.Errorf("expected no warning for abstention, got:\n%s", out)
		}
	})
}

// newTestApp builds an App whose answer engine retrieves from a seeded
// in-memory store and generates with a configurable fake LLM.
func newTestApp(ctx context.Context, t *testing.T, llmContent string) *App {
	t.Helper()
	embProvider := provider.NewMockEmbeddingProvider("mock-provider", "mock-model-v1", 1536)
	vecStore := store.NewInMemoryVectorStore()
	contentStore := store.NewInMemoryContentStore()
	seedTestChunk(ctx, t, embProvider, vecStore, contentStore)

	retriever := dense.NewDenseRetriever(embProvider, vecStore, contentStore)
	engine := qa.NewAnswerEngine(
		qa.NewRuleBasedAnalyzer(),
		retriever,
		qacontext.NewDefaultContextBuilder(nil, 4000),
		qaprompt.NewRAGPromptBuilder(),
		&cliFakeLLM{content: llmContent},
		qaverification.NewDefaultVerificationPipeline(),
	)
	return &App{answerEngine: engine}
}

// seedTestChunk embeds and stores one chunk with content for CLI tests.
func seedTestChunk(ctx context.Context, t *testing.T, embProvider provider.EmbeddingProvider, vecStore *store.InMemoryVectorStore, contentStore *store.InMemoryContentStore) {
	t.Helper()
	vec, err := embProvider.EmbedQuery(ctx, "Creativity is a fundamental human quality.")
	if err != nil || len(vec) == 0 {
		t.Fatalf("failed to embed seed chunk: %v", err)
	}
	if err := vecStore.UpsertPoints(ctx, []store.VectorPoint{{
		ID:     "pt-chk-1",
		Vector: vec,
		Metadata: indexingmodel.VectorMetadata{
			DocumentID:  "doc-1",
			ChunkID:     "chk-1",
			SectionPath: "Introduction",
			PageNumbers: []int{1},
			ContentHash: "hash-1",
		},
	}}); err != nil {
		t.Fatalf("failed to seed vector store: %v", err)
	}
	if err := contentStore.PutContent(ctx, []store.ChunkContent{
		{ChunkID: "chk-1", ContentMarkdown: "Creativity is a fundamental human quality."},
	}); err != nil {
		t.Fatalf("failed to seed content store: %v", err)
	}
}

// cliFakeLLM is a configurable LLMProvider fake for CLI rendering tests.
type cliFakeLLM struct {
	content string
}

func (f *cliFakeLLM) Generate(ctx context.Context, prompt qaprompt.PromptMessage) (*llmprovider.LLMResponse, error) {
	return &llmprovider.LLMResponse{
		Content:    f.content,
		Model:      "fake-model",
		Provider:   "fake-provider",
		TokenUsage: llmprovider.LLMUsage{TotalTokens: 10},
	}, nil
}

func (f *cliFakeLLM) Stream(ctx context.Context, prompt qaprompt.PromptMessage) (<-chan llmprovider.StreamChunk, error) {
	ch := make(chan llmprovider.StreamChunk, 1)
	ch <- llmprovider.StreamChunk{Content: f.content}
	ch <- llmprovider.StreamChunk{Done: true}
	close(ch)
	return ch, nil
}

func (f *cliFakeLLM) Capabilities() llmprovider.ModelCapabilities {
	return llmprovider.ModelCapabilities{SupportsSystemMessage: true, SupportsStreaming: true, ContextWindow: 128000}
}
