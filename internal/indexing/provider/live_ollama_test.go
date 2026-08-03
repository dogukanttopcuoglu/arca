package provider_test

import (
	"context"
	"testing"
	"time"

	"arca/internal/indexing/provider"
)

func TestLiveOllamaEmbeddingProvider(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live Ollama smoke test in short mode")
	}

	p := provider.NewOllamaEmbeddingProvider("http://localhost:11434", "nomic-embed-text:latest")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := p.Health(ctx); err != nil {
		t.Skipf("Ollama not reachable at localhost:11434, skipping: %v", err)
	}

	vec, err := p.EmbedQuery(ctx, "Rick Rubin creativity")
	if err != nil {
		t.Fatalf("EmbedQuery against live Ollama failed: %v", err)
	}
	if len(vec) != 768 {
		t.Fatalf("expected 768-dim query vector, got %d", len(vec))
	}

	res, err := p.EmbedDocuments(ctx, []string{"Rick Rubin is a producer.", "The Creative Act explores creativity."})
	if err != nil {
		t.Fatalf("EmbedDocuments against live Ollama failed: %v", err)
	}
	if len(res.Vectors) != 2 {
		t.Fatalf("expected 2 document vectors, got %d", len(res.Vectors))
	}
	for i, v := range res.Vectors {
		if len(v) != 768 {
			t.Fatalf("document vector %d expected 768-dim, got %d", i, len(v))
		}
	}
}
