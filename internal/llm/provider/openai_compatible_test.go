package provider_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	llmprovider "arca/internal/llm/provider"
	qaprompt "arca/internal/qa/prompt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAICompatibleProvider(t *testing.T) {
	t.Run("posts to chat/completions with bearer auth and mapped prompt", func(t *testing.T) {
		var gotBody map[string]interface{}
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "POST", r.Method)
			assert.Equal(t, "/chat/completions", r.URL.Path)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			assert.Equal(t, "Bearer secret-key", r.Header.Get("Authorization"))
			require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"choices": [{"message": {"role": "assistant", "content": "Hello"}}],
				"usage": {"prompt_tokens": 5, "completion_tokens": 7, "total_tokens": 12}
			}`))
		}))
		defer ts.Close()

		p := llmprovider.NewOpenAICompatibleProvider(ts.URL, "secret-key", "gpt-4o-mini", "agentrouter")
		resp, err := p.Generate(context.Background(), qaprompt.PromptMessage{
			System: "You are a helpful assistant.",
			Messages: []qaprompt.Message{
				{Role: "user", Content: "What is creativity?"},
			},
			Options: qaprompt.GenerationOptions{Temperature: 0.2, MaxTokens: 1500},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, "gpt-4o-mini", gotBody["model"], "model must be forwarded verbatim")
		messages, ok := gotBody["messages"].([]interface{})
		require.True(t, ok, "expected messages array")
		require.Len(t, messages, 2)
		system := messages[0].(map[string]interface{})
		user := messages[1].(map[string]interface{})
		assert.Equal(t, "system", system["role"])
		assert.Equal(t, "You are a helpful assistant.", system["content"])
		assert.Equal(t, "user", user["role"])
		assert.Equal(t, "What is creativity?", user["content"])
		assert.Equal(t, 0.2, gotBody["temperature"])
		assert.Equal(t, float64(1500), gotBody["max_tokens"])

		assert.Equal(t, "Hello", resp.Content)
		assert.Equal(t, "agentrouter", resp.Provider)
		assert.Equal(t, "gpt-4o-mini", resp.Model)
		assert.Equal(t, 5, resp.TokenUsage.PromptTokens)
		assert.Equal(t, 7, resp.TokenUsage.CompletionTokens)
		assert.Equal(t, 12, resp.TokenUsage.TotalTokens)
	})

	t.Run("forwards model identifier verbatim with no normalization", func(t *testing.T) {
		var gotBody map[string]interface{}
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"choices": [{"message": {"content": "ok"}}]}`))
		}))
		defer ts.Close()

		model := "My-Custom.Model:case-sensitive-v1 "
		p := llmprovider.NewOpenAICompatibleProvider(ts.URL, "k", model, "any-label")
		_, err := p.Generate(context.Background(), qaprompt.PromptMessage{Messages: []qaprompt.Message{{Role: "user", Content: "hi"}}})
		require.NoError(t, err)
		assert.Equal(t, model, gotBody["model"])
	})

	t.Run("omits authorization header when no key is configured", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Empty(t, r.Header.Get("Authorization"))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"choices": [{"message": {"content": "ok"}}]}`))
		}))
		defer ts.Close()

		p := llmprovider.NewOpenAICompatibleProvider(ts.URL, "", "model", "label")
		_, err := p.Generate(context.Background(), qaprompt.PromptMessage{Messages: []qaprompt.Message{{Role: "user", Content: "hi"}}})
		require.NoError(t, err)
	})

	t.Run("falls back to configured model when response omits model", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"choices": [{"message": {"content": "ok"}}]}`))
		}))
		defer ts.Close()

		p := llmprovider.NewOpenAICompatibleProvider(ts.URL, "k", "fallback-model", "label")
		resp, err := p.Generate(context.Background(), qaprompt.PromptMessage{Messages: []qaprompt.Message{{Role: "user", Content: "hi"}}})
		require.NoError(t, err)
		assert.Equal(t, "fallback-model", resp.Model)
	})

	t.Run("maps non-200 responses to a typed API error with status", func(t *testing.T) {
		for _, tc := range []struct {
			status int
			body   string
		}{
			{http.StatusUnauthorized, `{"error": "invalid api key"}`},
			{http.StatusTooManyRequests, `{"error": "rate limited"}`},
			{http.StatusInternalServerError, `{"error": "boom"}`},
		} {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))

			p := llmprovider.NewOpenAICompatibleProvider(ts.URL, "k", "model", "label")
			_, err := p.Generate(context.Background(), qaprompt.PromptMessage{Messages: []qaprompt.Message{{Role: "user", Content: "hi"}}})
			require.Error(t, err, "expected error for status %d", tc.status)

			var apiErr *llmprovider.OpenAIAPIError
			require.ErrorAs(t, err, &apiErr, "expected typed OpenAIAPIError for status %d", tc.status)
			assert.Equal(t, tc.status, apiErr.StatusCode)
			assert.Contains(t, apiErr.Message, tc.body)
			ts.Close()
		}
	})

	t.Run("returns error on malformed response body", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"choices": [{"message": {`))
		}))
		defer ts.Close()

		p := llmprovider.NewOpenAICompatibleProvider(ts.URL, "k", "model", "label")
		_, err := p.Generate(context.Background(), qaprompt.PromptMessage{Messages: []qaprompt.Message{{Role: "user", Content: "hi"}}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "decode")
	})

	t.Run("returns error when response contains no assistant content", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"choices": [], "usage": {}}`))
		}))
		defer ts.Close()

		p := llmprovider.NewOpenAICompatibleProvider(ts.URL, "k", "model", "label")
		_, err := p.Generate(context.Background(), qaprompt.PromptMessage{Messages: []qaprompt.Message{{Role: "user", Content: "hi"}}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no assistant content")
	})

	t.Run("rejects generation when no model is configured", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("server must not be called without a configured model")
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer ts.Close()

		p := llmprovider.NewOpenAICompatibleProvider(ts.URL, "k", "", "label")
		_, err := p.Generate(context.Background(), qaprompt.PromptMessage{Messages: []qaprompt.Message{{Role: "user", Content: "hi"}}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "model identifier is not configured")
	})

	t.Run("omits zero-valued options from the request", func(t *testing.T) {
		var gotBody map[string]interface{}
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"choices": [{"message": {"content": "ok"}}]}`))
		}))
		defer ts.Close()

		p := llmprovider.NewOpenAICompatibleProvider(ts.URL, "k", "model", "label")
		_, err := p.Generate(context.Background(), qaprompt.PromptMessage{Messages: []qaprompt.Message{{Role: "user", Content: "hi"}}})
		require.NoError(t, err)
		_, hasTemp := gotBody["temperature"]
		_, hasMax := gotBody["max_tokens"]
		assert.False(t, hasTemp, "zero temperature should be omitted")
		assert.False(t, hasMax, "zero max_tokens should be omitted")
	})

	t.Run("reports capabilities without provider branching", func(t *testing.T) {
		p := llmprovider.NewOpenAICompatibleProvider("https://example.com/v1", "k", "model", "whatever-label")
		caps := p.Capabilities()
		assert.True(t, caps.SupportsSystemMessage)
		assert.True(t, caps.SupportsStreaming)
		assert.Equal(t, 128000, caps.ContextWindow)
	})

	t.Run("stream emits the completion as a single chunk then done", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"choices": [{"message": {"content": "streamed text"}}]}`))
		}))
		defer ts.Close()

		p := llmprovider.NewOpenAICompatibleProvider(ts.URL, "k", "model", "label")
		ch, err := p.Stream(context.Background(), qaprompt.PromptMessage{Messages: []qaprompt.Message{{Role: "user", Content: "hi"}}})
		require.NoError(t, err)

		var got strings.Builder
		for chunk := range ch {
			require.NoError(t, chunk.Error)
			got.WriteString(chunk.Content)
			if chunk.Done {
				break
			}
		}
		assert.Equal(t, "streamed text", got.String())
	})
}
