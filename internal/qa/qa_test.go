package qa_test

import (
	"context"
	"testing"

	indexingmodel "arca/internal/indexing/model"
	"arca/internal/indexing/provider"
	"arca/internal/indexing/store"
	"arca/internal/qa"
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

func TestAnswerEngine_SyncOrchestration(t *testing.T) {
	ctx := context.Background()

	mockProvider := provider.NewMockEmbeddingProvider("mock-provider", "mock-model-v1", 1536)
	storeImpl := store.NewInMemoryVectorStore()

	// Seed vector store point
	_ = storeImpl.UpsertPoints(ctx, []store.VectorPoint{
		{
			ID:     "pt-1",
			Vector: mockProviderGenerateVector(mockProvider, "Creativity is a fundamental human quality."),
			Metadata: indexingmodel.VectorMetadata{
				DocumentID:  "doc-1",
				ChunkID:     "chk-1",
				SectionPath: "Introduction",
				ContentHash: "hash-1",
			},
		},
	})

	denseRetriever := dense.NewDenseRetriever(mockProvider, storeImpl, store.NewInMemoryContentStore())
	analyzer := qa.NewRuleBasedAnalyzer()

	engine := qa.NewAnswerEngine(analyzer, denseRetriever, nil, nil, nil)

	t.Run("successfully orchestrates QA pipeline end to end", func(t *testing.T) {
		draft, err := engine.Answer(ctx, seam.RetrievalQuery{
			QueryText: "What is creativity?",
			TopK:      5,
		})

		if err != nil {
			t.Fatalf("unexpected error during Answer execution: %v", err)
		}

		if draft == nil {
			t.Fatal("expected non-nil AnswerDraft")
		}
		if len(draft.SearchResults) == 0 {
			t.Error("expected non-empty SearchResults in draft")
		}
	})
}

func mockProviderGenerateVector(p provider.EmbeddingProvider, text string) []float32 {
	vec, err := p.EmbedQuery(context.Background(), text)
	if err != nil || len(vec) == 0 {
		return make([]float32, 1536)
	}
	return vec
}
