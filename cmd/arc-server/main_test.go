package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"arca/cmd/arc/cli"
	indexingmodel "arca/internal/indexing/model"
	"arca/internal/indexing/provider"
	"arca/internal/indexing/store"
	llmprovider "arca/internal/llm/provider"
	pdfmodel "arca/internal/pdfinspector/model"
	"arca/internal/qa"
	qacontext "arca/internal/qa/context"
	qaprompt "arca/internal/qa/prompt"
	qaverification "arca/internal/qa/verification"
	"arca/internal/retrieval/dense"

	"github.com/gofiber/fiber/v2"
)

// seedTestStore plants the default-space corpus the listing tests assert
// against: one profile point and two chunk points for creative-act, one
// chunk for thinking-in-systems. Vectors come from the mock provider so the
// store stays fully offline.
func seedTestStore(t *testing.T, vecStore *store.InMemoryVectorStore) {
	t.Helper()
	ctx := context.Background()
	emb := provider.NewMockEmbeddingProvider("mock-provider", "mock-model-v1", 1536)
	points := []store.VectorPoint{
		{
			ID:              "pt-creative-profile",
			ContentMarkdown: "Title: The Creative Act\nAuthor: Rick Rubin\nSummary: Ways of looking.\n",
			Metadata: indexingmodel.VectorMetadata{
				DocumentID:  "creative-act",
				ChunkID:     "creative-act/document-profile/001",
				ChunkOrder:  0,
				SectionPath: "Document Profile",
				ContentType: pdfmodel.ContentTypeDocumentProfile,
			},
		},
		{
			ID:              "pt-creative-001",
			ContentMarkdown: "Creativity is a fundamental human quality [Ref 1].",
			Metadata: indexingmodel.VectorMetadata{
				DocumentID:  "creative-act",
				ChunkID:     "creative-act/introduction/001",
				ChunkOrder:  1,
				SectionPath: "Foreword",
				PageNumbers: []int{7},
				ContentType: pdfmodel.ContentTypeParagraph,
			},
		},
		{
			ID:              "pt-creative-002",
			ContentMarkdown: "Discipline turns impulses into works.",
			Metadata: indexingmodel.VectorMetadata{
				DocumentID:  "creative-act",
				ChunkID:     "creative-act/discipline/001",
				ChunkOrder:  2,
				SectionPath: "Discipline",
				PageNumbers: []int{7, 8},
				ContentType: pdfmodel.ContentTypeParagraph,
			},
		},
		{
			ID:              "pt-systems-001",
			ContentMarkdown: "A system is more than the sum of its parts.",
			Metadata: indexingmodel.VectorMetadata{
				DocumentID:  "thinking-in-systems",
				ChunkID:     "thinking-in-systems/introduction/001",
				ChunkOrder:  1,
				SectionPath: "Introduction",
				PageNumbers: []int{1},
				ContentType: pdfmodel.ContentTypeParagraph,
			},
		},
	}
	for i := range points {
		v, err := emb.EmbedQuery(ctx, points[i].ContentMarkdown)
		if err != nil || len(v) == 0 {
			t.Fatalf("embed seed point %d: %v", i, err)
		}
		points[i].Vector = v
	}
	if err := vecStore.UpsertPoints(ctx, points); err != nil {
		t.Fatalf("seed vector store: %v", err)
	}
}

// newTestRuntime builds the production composition root over the in-memory
// store and seeds it, mirroring the CLI test convention. The default LLM
// config stays untouched: the listing assertions never reach the engine.
func newTestRuntime(t *testing.T) (*cli.Runtime, *store.InMemoryVectorStore) {
	t.Helper()
	runtime, err := cli.NewRuntime(cli.DefaultConfig())
	if err != nil {
		t.Fatalf("failed to construct runtime: %v", err)
	}
	vecStore, ok := runtime.VectorStore().(*store.InMemoryVectorStore)
	if !ok {
		t.Fatalf("expected in-memory vector store, got %T", runtime.VectorStore())
	}
	seedTestStore(t, vecStore)
	return runtime, vecStore
}

// newTestApp wires the full app over a freshly seeded runtime.
func newTestApp(t *testing.T, engine *qa.AnswerEngine) *fiber.App {
	t.Helper()
	rt, _ := newTestRuntime(t)
	return newApp(rt, engine, "http://localhost:3000")
}

func TestListSpaces(t *testing.T) {
	app := newTestApp(t, abstainEngine(t))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/spaces", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("spaces request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var spaces []SpaceSummary
	if err := json.NewDecoder(resp.Body).Decode(&spaces); err != nil {
		t.Fatalf("decode spaces: %v", err)
	}
	want := []SpaceSummary{{ID: "default", Name: "Default Space", DocumentCount: 2}}
	if !reflect.DeepEqual(spaces, want) {
		t.Errorf("spaces = %+v, want %+v", spaces, want)
	}
}

func TestListDocuments(t *testing.T) {
	app := newTestApp(t, abstainEngine(t))

	t.Run("default space lists both documents with profile-derived fields", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/spaces/default/documents", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("documents request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		var docs []DocumentSummary
		if err := json.NewDecoder(resp.Body).Decode(&docs); err != nil {
			t.Fatalf("decode documents: %v", err)
		}
		want := []DocumentSummary{
			{DocumentID: "creative-act", ChunkCount: 3, Title: "The Creative Act", Author: "Rick Rubin"},
			{DocumentID: "thinking-in-systems", ChunkCount: 1},
		}
		if !reflect.DeepEqual(docs, want) {
			t.Errorf("documents = %+v, want %+v", docs, want)
		}
	})

	t.Run("unknown space yields an empty list", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/spaces/nope/documents", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("documents request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		var docs []DocumentSummary
		if err := json.NewDecoder(resp.Body).Decode(&docs); err != nil {
			t.Fatalf("decode documents: %v", err)
		}
		if len(docs) != 0 {
			t.Errorf("documents = %+v, want empty", docs)
		}
	})
}

func TestGetChunk(t *testing.T) {
	app := newTestApp(t, abstainEngine(t))

	t.Run("chunk id with embedded slashes resolves to the full JSON shape", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/chunks/creative-act/introduction/001", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("chunk request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		var chunk KnowledgeChunk
		if err := json.NewDecoder(resp.Body).Decode(&chunk); err != nil {
			t.Fatalf("decode chunk: %v", err)
		}
		want := KnowledgeChunk{
			ChunkID:         "creative-act/introduction/001",
			DocumentID:      "creative-act",
			SectionPath:     "Foreword",
			PageNumbers:     []int{7},
			ContentMarkdown: "Creativity is a fundamental human quality [Ref 1].",
			ContentType:     pdfmodel.ContentTypeParagraph,
		}
		if !reflect.DeepEqual(chunk, want) {
			t.Errorf("chunk = %+v, want %+v", chunk, want)
		}
	})

	t.Run("unknown chunk id is a 404 JSON error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/chunks/no-such-chunk", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("chunk request failed: %v", err)
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", resp.StatusCode)
		}
		var body map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode error body: %v", err)
		}
		if body["error"] == "" {
			t.Error("expected a non-empty error message")
		}
	})
}

// abstainEngine is an engine with no retriever: every stream emits the
// no_evidence verification followed by done.
func abstainEngine(t *testing.T) *qa.AnswerEngine {
	t.Helper()
	return qa.NewAnswerEngine(nil, nil, nil, nil, nil, nil, nil)
}

// fakeLLMEngine builds an answer engine over the seeded store with the
// deterministic streaming mock LLM, mirroring the internal/qa test seam.
func fakeLLMEngine(t *testing.T, vecStore *store.InMemoryVectorStore) *qa.AnswerEngine {
	t.Helper()
	emb := provider.NewMockEmbeddingProvider("mock-provider", "mock-model-v1", 1536)
	retriever := dense.NewDenseRetriever(emb, vecStore, store.NewInMemoryContentStore())
	return qa.NewAnswerEngine(
		qa.NewRuleBasedAnalyzer(),
		retriever,
		qacontext.NewDefaultContextBuilder(nil, 4000),
		qaprompt.NewRAGPromptBuilder(),
		llmprovider.NewMockLLMProvider("mock-provider", "mock-model"),
		qaverification.NewDefaultVerificationPipeline(),
		nil, // nil gate: legacy behavior for tests
	)
}

// sseEvents extracts the data payload of every SSE event in a body.
func sseEvents(t *testing.T, body string) []string {
	t.Helper()
	var events []string
	for _, block := range strings.Split(body, "\n\n") {
		for _, line := range strings.Split(block, "\n") {
			if strings.HasPrefix(line, "data: ") {
				events = append(events, strings.TrimPrefix(line, "data: "))
			}
		}
	}
	return events
}

func TestQAStream(t *testing.T) {
	t.Run("missing q is a 400 JSON error", func(t *testing.T) {
		app := newTestApp(t, abstainEngine(t))
		req := httptest.NewRequest(http.MethodGet, "/api/v1/qa/stream", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("stream request failed: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
		var body map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode error body: %v", err)
		}
		if body["error"] == "" {
			t.Error("expected a non-empty error message")
		}
	})

	t.Run("no-retriever engine streams verification no_evidence then done", func(t *testing.T) {
		app := newTestApp(t, abstainEngine(t))
		req := httptest.NewRequest(http.MethodGet, "/api/v1/qa/stream?q=Where+is+the+evidence%3F", nil)
		resp, err := app.Test(req, 5000)
		if err != nil {
			t.Fatalf("stream request failed: %v", err)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
			t.Errorf("content-type = %q, want text/event-stream", ct)
		}
		if cc := resp.Header.Get("Cache-Control"); cc != "no-cache" {
			t.Errorf("cache-control = %q, want no-cache", cc)
		}
		events := sseEvents(t, readAll(t, resp))
		if len(events) != 2 {
			t.Fatalf("got %d events, want 2: %s", len(events), events)
		}
		var first qa.AnswerStreamChunk
		if err := json.Unmarshal([]byte(events[0]), &first); err != nil {
			t.Fatalf("decode first event: %v", err)
		}
		if first.Type != qa.StreamChunkVerification {
			t.Errorf("first event type = %q, want verification", first.Type)
		}
		if first.Verified == nil || first.Verified.Status != qaverification.StatusNoEvidence {
			t.Errorf("first event verified = %+v, want no_evidence", first.Verified)
		}
		var last qa.AnswerStreamChunk
		if err := json.Unmarshal([]byte(events[1]), &last); err != nil {
			t.Fatalf("decode last event: %v", err)
		}
		if last.Type != qa.StreamChunkDone {
			t.Errorf("last event type = %q, want done", last.Type)
		}
	})

	t.Run("fake streaming LLM emits token, verification, and done events", func(t *testing.T) {
		rt, vecStore := newTestRuntime(t)
		app := newApp(rt, fakeLLMEngine(t, vecStore), "http://localhost:3000")
		req := httptest.NewRequest(http.MethodGet, "/api/v1/qa/stream?q=What+is+creativity%3F", nil)
		resp, err := app.Test(req, 5000)
		if err != nil {
			t.Fatalf("stream request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		events := sseEvents(t, readAll(t, resp))

		var tokens []string
		var verified *qaverification.VerifiedAnswer
		for _, raw := range events {
			var chunk qa.AnswerStreamChunk
			if err := json.Unmarshal([]byte(raw), &chunk); err != nil {
				t.Fatalf("decode event %q: %v", raw, err)
			}
			switch chunk.Type {
			case qa.StreamChunkToken:
				tokens = append(tokens, chunk.Content)
			case qa.StreamChunkVerification:
				verified = chunk.Verified
			case qa.StreamChunkError:
				t.Errorf("unexpected error event: %s", chunk.Error)
			}
		}
		if len(tokens) == 0 {
			t.Fatal("expected token events from the streaming mock LLM")
		}
		want := "Based on the provided information, creativity is a discipline and a lifestyle [Ref 1]."
		if full := strings.Join(tokens, ""); full != want {
			t.Errorf("assembled tokens = %q, want %q", full, want)
		}
		if verified == nil {
			t.Fatal("expected a verification event")
		}
		if verified.Status != qaverification.StatusVerified {
			t.Errorf("verification status = %q, want verified", verified.Status)
		}
		var last qa.AnswerStreamChunk
		if err := json.Unmarshal([]byte(events[len(events)-1]), &last); err != nil {
			t.Fatalf("decode last event: %v", err)
		}
		if last.Type != qa.StreamChunkDone {
			t.Errorf("last event type = %q, want done", last.Type)
		}
	})
}

func TestCORS(t *testing.T) {
	app := newTestApp(t, abstainEngine(t))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/spaces", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("spaces request failed: %v", err)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Errorf("access-control-allow-origin = %q, want http://localhost:3000", got)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func readAll(t *testing.T, resp *http.Response) string {
	t.Helper()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(data)
}
