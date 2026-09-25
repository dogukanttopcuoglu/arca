package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
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
			{DocumentID: "creative-act", ChunkCount: 2, Title: "The Creative Act", Author: "Rick Rubin"},
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

// uploadExtractionJSON is a minimal Firecrawl /v1/extract response whose
// markdown drives the real semantic chunker offline: one title section and
// two sub-sections yield a couple of deterministic chunks. The byte content
// of the uploaded "PDF" is ignored by the stub; only the %PDF header must
// pass the inspector's fail-fast validation.
const uploadExtractionJSON = `{
  "markdown": "# Sample Document\n\nA tiny offline fixture.\n\n## First Section\n\nSemantic chunking keeps boundaries clean.\n\n## Second Section\n\nSecond section content with a citation [1].\n\n[1] Fixture, A. (2025). Offline ingestion fixture.",
  "json_layout": {
    "pages": [
      {"page_number": 1, "markdown": "# Sample Document\n\nA tiny offline fixture."},
      {"page_number": 2, "markdown": "## First Section\n\nSemantic chunking keeps boundaries clean."},
      {"page_number": 3, "markdown": "## Second Section\n\nSecond section content with a citation [1]."}
    ]
  },
  "metadata": {"title": "Sample Document", "author": "Fixture Author", "page_count": 3, "searchable": true},
  "ocr_applied": false
}`

// uploadStub serves the canned extraction JSON for the duration of one
// upload test, keeping the whole ingest pipeline offline.
func uploadStub(t *testing.T) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(uploadExtractionJSON))
	}))
	t.Cleanup(ts.Close)
	return ts
}

// newUploadApp builds a fresh unseeded runtime over the in-memory store
// with mock embeddings and the given Firecrawl URL, then wires the real app
// over it.
func newUploadApp(t *testing.T, firecrawlURL string) (*fiber.App, *store.InMemoryVectorStore) {
	t.Helper()
	cfg := cli.DefaultConfig()
	cfg.FirecrawlBaseURL = firecrawlURL
	runtime, err := cli.NewRuntime(cfg)
	if err != nil {
		t.Fatalf("failed to construct runtime: %v", err)
	}
	vecStore, ok := runtime.VectorStore().(*store.InMemoryVectorStore)
	if !ok {
		t.Fatalf("expected in-memory vector store, got %T", runtime.VectorStore())
	}
	return newApp(runtime, abstainEngine(t), "http://localhost:3000"), vecStore
}

// uploadRequest builds a multipart POST body for the documents endpoint
// with one file field carrying the given filename and content.
func uploadRequest(t *testing.T, filename, content string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write([]byte(content)); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

// uploadDone performs one upload and returns the done event payload.
func uploadDone(t *testing.T, app *fiber.App, req *http.Request) UploadDoneEvent {
	t.Helper()
	resp, err := app.Test(req, 30000)
	if err != nil {
		t.Fatalf("upload request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	events := sseEvents(t, readAll(t, resp))
	if len(events) == 0 {
		t.Fatal("got no events from upload stream")
	}
	var doneEv UploadDoneEvent
	if err := json.Unmarshal([]byte(events[len(events)-1]), &doneEv); err != nil {
		t.Fatalf("decode done event %q: %v", events[len(events)-1], err)
	}
	if doneEv.Type != "done" {
		t.Fatalf("last event type = %q, want done (events: %s)", doneEv.Type, events)
	}
	return doneEv
}

// listUploadedDocuments fetches the default-space document listing.
func listUploadedDocuments(t *testing.T, app *fiber.App) []DocumentSummary {
	t.Helper()
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
	return docs
}

func TestUploadDocument(t *testing.T) {
	stub := uploadStub(t)
	app, vecStore := newUploadApp(t, stub.URL)

	// The PDF header satisfies the inspector's fail-fast validation; the
	// stub never inspects the uploaded bytes.
	req := uploadRequest(t, "sample.pdf", "%PDF-1.4 fixture\n")
	resp, err := app.Test(req, 30000)
	if err != nil {
		t.Fatalf("upload request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("content-type = %q, want text/event-stream", ct)
	}
	events := sseEvents(t, readAll(t, resp))
	if len(events) != 5 {
		t.Fatalf("got %d events, want 5 (parse, chunk, embed, indexed, done): %s", len(events), events)
	}

	var parseEv, chunkEv, embedEv, indexedEv UploadPhaseEvent
	if err := json.Unmarshal([]byte(events[0]), &parseEv); err != nil {
		t.Fatalf("decode parse event: %v", err)
	}
	if parseEv.Type != "phase" || parseEv.Phase != "parse" {
		t.Errorf("event[0] = %+v, want phase/parse", parseEv)
	}
	if err := json.Unmarshal([]byte(events[1]), &chunkEv); err != nil {
		t.Fatalf("decode chunk event: %v", err)
	}
	if chunkEv.Type != "phase" || chunkEv.Phase != "chunk" || chunkEv.ChunkCount < 1 {
		t.Errorf("event[1] = %+v, want phase/chunk with chunk_count >= 1", chunkEv)
	}
	if err := json.Unmarshal([]byte(events[2]), &embedEv); err != nil {
		t.Fatalf("decode embed event: %v", err)
	}
	if embedEv.Type != "phase" || embedEv.Phase != "embed" {
		t.Errorf("event[2] = %+v, want phase/embed", embedEv)
	}
	if err := json.Unmarshal([]byte(events[3]), &indexedEv); err != nil {
		t.Fatalf("decode indexed event: %v", err)
	}
	if indexedEv.Type != "phase" || indexedEv.Phase != "indexed" {
		t.Errorf("event[3] = %+v, want phase/indexed", indexedEv)
	}

	var doneEv UploadDoneEvent
	if err := json.Unmarshal([]byte(events[4]), &doneEv); err != nil {
		t.Fatalf("decode done event: %v", err)
	}
	if doneEv.Type != "done" {
		t.Errorf("event[4] type = %q, want done", doneEv.Type)
	}
	doc := doneEv.Document
	if doc.DocumentID != "sample" {
		t.Errorf("document_id = %q, want sample (derived from the filename)", doc.DocumentID)
	}
	if doc.Title != "Sample Document" {
		t.Errorf("title = %q, want Sample Document", doc.Title)
	}
	if doc.ChunkCount != chunkEv.ChunkCount {
		t.Errorf("chunk_count = %d, want %d (from the inspection result)", doc.ChunkCount, chunkEv.ChunkCount)
	}
	// The aggregator degrades success to partial_success on warnings or
	// skipped pages; either is a completed ingest, never failed.
	if doc.Status != pdfmodel.StatusSuccess && doc.Status != pdfmodel.StatusPartialSuccess {
		t.Errorf("status = %q, want success or partial_success", doc.Status)
	}
	if doc.Indexed != chunkEv.ChunkCount {
		t.Errorf("indexed = %d, want %d on a fresh upload", doc.Indexed, chunkEv.ChunkCount)
	}
	if doc.Skipped != 0 {
		t.Errorf("skipped = %d, want 0 on a fresh upload", doc.Skipped)
	}
	if doc.Deleted != 0 {
		t.Errorf("deleted = %d, want 0 on a fresh upload", doc.Deleted)
	}

	// Every point in the store belongs to the uploaded document.
	points, err := vecStore.ListPoints(context.Background(), indexingmodel.MetadataFilter{})
	if err != nil {
		t.Fatalf("list points: %v", err)
	}
	if len(points) == 0 {
		t.Fatal("expected points in the store after a successful upload")
	}
	for _, pt := range points {
		if pt.Metadata.DocumentID != "sample" {
			t.Errorf("point %q document_id = %q, want sample", pt.ID, pt.Metadata.DocumentID)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/spaces", nil)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("spaces request failed: %v", err)
	}
	var spaces []SpaceSummary
	if err := json.NewDecoder(resp.Body).Decode(&spaces); err != nil {
		t.Fatalf("decode spaces: %v", err)
	}
	if !reflect.DeepEqual(spaces, []SpaceSummary{{ID: "default", Name: "Default Space", DocumentCount: 1}}) {
		t.Errorf("spaces = %+v, want the default space with one document", spaces)
	}

	docs := listUploadedDocuments(t, app)
	if len(docs) != 1 {
		t.Fatalf("documents = %+v, want exactly one entry", docs)
	}
	// The listing counts chunk points only (the document profile point is
	// excluded, ADR-0052), so chunk_count there equals the done count.
	if docs[0].DocumentID != "sample" || docs[0].ChunkCount != doc.ChunkCount {
		t.Errorf("documents[0] = %+v, want sample with %d chunks", docs[0], doc.ChunkCount)
	}
	if docs[0].Title != "Sample Document" {
		t.Errorf("title = %q, want Sample Document", docs[0].Title)
	}
}

func TestUploadDocumentReupload(t *testing.T) {
	stub := uploadStub(t)
	app, _ := newUploadApp(t, stub.URL)

	done1 := uploadDone(t, app, uploadRequest(t, "sample.pdf", "%PDF-1.4 fixture\n"))
	done2 := uploadDone(t, app, uploadRequest(t, "sample.pdf", "%PDF-1.4 fixture\n"))

	// The diff worker sees identical content hashes and index signatures:
	// nothing re-embeds, everything is skipped, and no point is duplicated.
	if done2.Document.Indexed != 0 {
		t.Errorf("re-upload indexed = %d, want 0 (everything unchanged)", done2.Document.Indexed)
	}
	if done2.Document.Skipped < done1.Document.ChunkCount {
		t.Errorf("re-upload skipped = %d, want >= %d", done2.Document.Skipped, done1.Document.ChunkCount)
	}
	docs := listUploadedDocuments(t, app)
	if len(docs) != 1 {
		t.Fatalf("documents = %+v, want exactly one entry after a re-upload", docs)
	}
	if docs[0].DocumentID != "sample" {
		t.Errorf("document id = %q, want sample", docs[0].DocumentID)
	}
}

func TestUploadDocumentUnsupportedExtension(t *testing.T) {
	stub := uploadStub(t)
	app, vecStore := newUploadApp(t, stub.URL)

	resp, err := app.Test(uploadRequest(t, "notes.txt", "plain notes"), 5000)
	if err != nil {
		t.Fatalf("upload request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 with an in-stream error event", resp.StatusCode)
	}
	events := sseEvents(t, readAll(t, resp))
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1 error event: %s", len(events), events)
	}
	var errEv UploadErrorEvent
	if err := json.Unmarshal([]byte(events[0]), &errEv); err != nil {
		t.Fatalf("decode error event: %v", err)
	}
	if errEv.Type != "error" || !strings.Contains(errEv.Error, "only .pdf") {
		t.Errorf("error event = %+v, want a message about the .pdf restriction", errEv)
	}
	if got := vecStore.Points(); got != 0 {
		t.Errorf("store points = %d, want 0 (unsupported upload writes nothing)", got)
	}
}

func TestUploadDocumentMissingFile(t *testing.T) {
	stub := uploadStub(t)
	app, _ := newUploadApp(t, stub.URL)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("spaceId", "default"); err != nil {
		t.Fatalf("write form field: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("upload request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content-type = %q, want application/json", ct)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body["error"] == "" {
		t.Error("expected a non-empty error message")
	}
}

func TestUploadDocumentFirecrawlDown(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL
	dead.Close()

	app, vecStore := newUploadApp(t, deadURL)
	resp, err := app.Test(uploadRequest(t, "sample.pdf", "%PDF-1.4 fixture\n"), 30000)
	if err != nil {
		t.Fatalf("upload request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 with an in-stream error event", resp.StatusCode)
	}
	events := sseEvents(t, readAll(t, resp))
	if len(events) < 2 {
		t.Fatalf("got %d events, want parse then error: %s", len(events), events)
	}
	var last UploadErrorEvent
	if err := json.Unmarshal([]byte(events[len(events)-1]), &last); err != nil {
		t.Fatalf("decode last event: %v", err)
	}
	if last.Type != "error" || last.Error == "" {
		t.Errorf("last event = %+v, want an error event with a message", last)
	}
	if got := vecStore.Points(); got != 0 {
		t.Errorf("store points = %d, want 0 (failed inspection writes nothing)", got)
	}
}

func TestDeleteDocument(t *testing.T) {
	stub := uploadStub(t)
	app, vecStore := newUploadApp(t, stub.URL)
	uploadDone(t, app, uploadRequest(t, "sample.pdf", "%PDF-1.4 fixture\n"))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/documents/sample", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("delete request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode delete body: %v", err)
	}
	if body["deleted"] != "sample" {
		t.Errorf("deleted = %q, want sample", body["deleted"])
	}
	if got := vecStore.Points(); got != 0 {
		t.Errorf("store points = %d, want 0 after deletion", got)
	}
	if docs := listUploadedDocuments(t, app); len(docs) != 0 {
		t.Errorf("documents = %+v, want the empty list after deletion", docs)
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/v1/documents/nope", nil)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("delete request failed: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for an unknown document", resp.StatusCode)
	}
	var errBody map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&errBody); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if errBody["error"] == "" {
		t.Error("expected a non-empty error message")
	}
}

func TestUploadCORS(t *testing.T) {
	app := newTestApp(t, abstainEngine(t))

	for _, method := range []string{"POST", "DELETE"} {
		req := httptest.NewRequest(http.MethodOptions, "/api/v1/documents", nil)
		req.Header.Set("Origin", "http://localhost:3000")
		req.Header.Set("Access-Control-Request-Method", method)
		req.Header.Set("Access-Control-Request-Headers", "content-type")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("preflight for %s failed: %v", method, err)
		}
		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
			t.Errorf("%s preflight allow-origin = %q, want http://localhost:3000", method, got)
		}
		if allow := resp.Header.Get("Access-Control-Allow-Methods"); !strings.Contains(allow, method) {
			t.Errorf("%s preflight allow-methods = %q, want it to contain %s", method, allow, method)
		}
		if allow := strings.ToLower(resp.Header.Get("Access-Control-Allow-Headers")); !strings.Contains(allow, "content-type") {
			t.Errorf("%s preflight allow-headers = %q, want it to contain content-type", method, allow)
		}
	}
}
