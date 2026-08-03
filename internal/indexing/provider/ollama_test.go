package provider_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"arca/internal/indexing/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// genVec builds a deterministic 768-dimension vector for test payloads.
func genVec(seed float32) []float32 {
	v := make([]float32, 768)
	for i := range v {
		v[i] = seed + float32(i%10)
	}
	return v
}

func TestOllamaEmbeddingProvider(t *testing.T) {
	t.Run("health check hits /api/tags", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "GET", r.Method)
			assert.Equal(t, "/api/tags", r.URL.Path)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"models":[]}`))
		}))
		defer ts.Close()

		p := provider.NewOllamaEmbeddingProvider(ts.URL, "nomic-embed-text:latest")
		require.NoError(t, p.Health(context.Background()))
	})

	t.Run("health check fails on non-200", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer ts.Close()

		p := provider.NewOllamaEmbeddingProvider(ts.URL, "nomic-embed-text:latest")
		require.Error(t, p.Health(context.Background()))
	})

	t.Run("embed query posts prompt with search_query prefix and returns 768-dim vector", func(t *testing.T) {
		var gotBody map[string]interface{}
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "POST", r.Method)
			assert.Equal(t, "/api/embeddings", r.URL.Path)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			resp, _ := json.Marshal(map[string]interface{}{"embedding": genVec(0.5)})
			_, _ = w.Write(resp)
		}))
		defer ts.Close()

		p := provider.NewOllamaEmbeddingProvider(ts.URL, "nomic-embed-text:latest")
		vec, err := p.EmbedQuery(context.Background(), "Rick Rubin creativity")
		require.NoError(t, err)
		require.Len(t, vec, 768)

		assert.Equal(t, "nomic-embed-text:latest", gotBody["model"])
		assert.Equal(t, "search_query: Rick Rubin creativity", gotBody["prompt"])
	})

	t.Run("embed documents posts batch with search_document prefix", func(t *testing.T) {
		var gotBody map[string]interface{}
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "POST", r.Method)
			assert.Equal(t, "/api/embed", r.URL.Path)
			require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			resp, _ := json.Marshal(map[string]interface{}{
				"embeddings": [][]float32{genVec(0.1), genVec(0.2)},
			})
			_, _ = w.Write(resp)
		}))
		defer ts.Close()

		p := provider.NewOllamaEmbeddingProvider(ts.URL, "nomic-embed-text:latest")
		res, err := p.EmbedDocuments(context.Background(), []string{"first chunk", "second chunk"})
		require.NoError(t, err)
		require.Len(t, res.Vectors, 2)
		require.Len(t, res.Vectors[0], 768)

		inputs, ok := gotBody["input"].([]interface{})
		require.True(t, ok, "expected input to be an array")
		require.Len(t, inputs, 2)
		assert.Equal(t, "search_document: first chunk", inputs[0])
		assert.Equal(t, "search_document: second chunk", inputs[1])
		assert.Equal(t, "Ollama", res.Provider)
		assert.Equal(t, "nomic-embed-text:latest", res.Model)
	})

	t.Run("embed documents validates response dimensions", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			resp, _ := json.Marshal(map[string]interface{}{
				"embeddings": [][]float32{genVec(0.1), make([]float32, 4)},
			})
			_, _ = w.Write(resp)
		}))
		defer ts.Close()

		p := provider.NewOllamaEmbeddingProvider(ts.URL, "nomic-embed-text:latest")
		_, err := p.EmbedDocuments(context.Background(), []string{"a", "b"})
		require.Error(t, err)
	})

	t.Run("embed query fails on non-200 response", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"unknown model"}`))
		}))
		defer ts.Close()

		p := provider.NewOllamaEmbeddingProvider(ts.URL, "nomic-embed-text:latest")
		_, err := p.EmbedQuery(context.Background(), "query")
		require.Error(t, err)
	})

	t.Run("capabilities report 768 dimension and batch support", func(t *testing.T) {
		p := provider.NewOllamaEmbeddingProvider("http://localhost:11434", "nomic-embed-text:latest")
		caps := p.Capabilities()
		assert.Equal(t, 768, caps.Dimension)
		assert.True(t, caps.SupportsBatch)
		assert.Equal(t, "Ollama", p.Provider())
		assert.Equal(t, "nomic-embed-text:latest", p.Model())
	})
}
