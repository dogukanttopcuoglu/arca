package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"arca/cmd/arc/cli"
	indexingmodel "arca/internal/indexing/model"
	"arca/internal/indexing/store"
	pdfmodel "arca/internal/pdfinspector/model"
	"arca/internal/qa"
	retrievalseam "arca/internal/retrieval/seam"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

// defaultSpaceID is the v1 space every point lands in. Indexing today never
// sets KnowledgeSpaceID, so the empty ID and the literal default both fold
// into one space.
const defaultSpaceID = "default"

// SpaceSummary is the v1 KnowledgeSpace listing shape: one entry per distinct
// KnowledgeSpaceID present in the index.
type SpaceSummary struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	DocumentCount int    `json:"document_count"`
}

// DocumentSummary is the v1 document listing shape. Title and author are
// derived from the document_profile point only when its payload carries them
// in the deterministic line format; page count and language have no vector
// payload source and stay omitted.
type DocumentSummary struct {
	DocumentID string `json:"document_id"`
	ChunkCount int    `json:"chunk_count"`
	Title      string `json:"title,omitempty"`
	Author     string `json:"author,omitempty"`
	PageCount  int    `json:"page_count,omitempty"`
	Language   string `json:"language,omitempty"`
}

// KnowledgeChunk is the v1 chunk shape resolved from a vector point. The
// point payload carries no hierarchy links, so parent/child ids do not exist
// at this seam.
type KnowledgeChunk struct {
	ChunkID         string `json:"chunk_id"`
	DocumentID      string `json:"document_id"`
	SectionPath     string `json:"section_path"`
	PageNumbers     []int  `json:"page_numbers"`
	ContentMarkdown string `json:"content_markdown"`
	ContentType     string `json:"content_type"`
}

// Upload stream event types (server-side v1 surface). The snake_case wire
// contract is decoupled from the internal Go names.
const (
	uploadEventPhase = "phase"
	uploadEventDone  = "done"
	uploadEventError = "error"
)

// Upload phase names streamed by POST /api/v1/documents in pipeline order.
const (
	uploadPhaseParse   = "parse"
	uploadPhaseChunk   = "chunk"
	uploadPhaseEmbed   = "embed"
	uploadPhaseIndexed = "indexed"
)

// UploadPhaseEvent marks one ingest phase boundary. chunk_count accompanies
// the chunk phase, indexed/skipped the indexed phase; both stay omitted on
// the other phases so the wire shape stays minimal. The contract is:
// phase parse, phase chunk (chunk_count), phase embed, phase indexed
// (indexed, skipped), done (document), error (error).
type UploadPhaseEvent struct {
	Type       string `json:"type"`
	Phase      string `json:"phase"`
	ChunkCount int    `json:"chunk_count,omitempty"`
	Indexed    int    `json:"indexed,omitempty"`
	Skipped    int    `json:"skipped,omitempty"`
}

// UploadDoneEvent terminates an upload stream with the ingested document
// summary. Status mirrors the inspection diagnostics status: success or
// partial_success; a failed pipeline never reaches done.
type UploadDoneEvent struct {
	Type     string                `json:"type"`
	Document UploadDocumentSummary `json:"document"`
}

// UploadDocumentSummary is the v1 document shape carried by the done event.
type UploadDocumentSummary struct {
	DocumentID   string `json:"document_id"`
	Title        string `json:"title"`
	ChunkCount   int    `json:"chunk_count"`
	Status       string `json:"status"`
	Indexed      int    `json:"indexed"`
	Skipped      int    `json:"skipped"`
	Deleted      int    `json:"deleted"`
	WarningCount int    `json:"warning_count"`
}

// UploadErrorEvent carries a failed upload's verbatim message inside the
// stream; a missing or invalid form field is a 400 JSON before the stream.
type UploadErrorEvent struct {
	Type  string `json:"type"`
	Error string `json:"error"`
}

// server holds the two seams the HTTP surface reads from: the runtime's
// shared vector store for listing and the answer engine for streaming.
type server struct {
	runtime *cli.Runtime
	engine  *qa.AnswerEngine
}

// newApp wires the v1 HTTP surface with CORS over the injected seams. Tests
// inject a runtime over an in-memory store and an engine built with a fake
// streaming LLM, so no network is touched.
func newApp(runtime *cli.Runtime, engine *qa.AnswerEngine, allowedOrigin string) *fiber.App {
	s := &server{runtime: runtime, engine: engine}

	// BodyLimit covers multi-megabyte uploads: FormFile buffers the full
	// multipart body before the handler runs, and a 5MB book must pass
	// without the 4MB default rejecting it.
	app := fiber.New(fiber.Config{BodyLimit: 100 << 20})
	app.Use(cors.New(cors.Config{
		AllowOrigins: allowedOrigin,
		AllowMethods: "GET,POST,DELETE,OPTIONS",
		AllowHeaders: "Content-Type",
	}))

	app.Get("/healthz", handleHealthz)

	api := app.Group("/api/v1")
	api.Get("/spaces", s.handleListSpaces)
	api.Get("/spaces/:spaceId/documents", s.handleListDocuments)
	// Chunk IDs embed slashes (documentID/section/ordinal); the wildcard
	// captures the full id rather than the first path segment.
	api.Get("/chunks/*", s.handleGetChunk)
	api.Get("/qa/stream", s.handleQAStream)
	api.Post("/documents", s.handleUploadDocument)
	api.Delete("/documents/:documentId", s.handleDeleteDocument)

	return app
}

// httpAddr reads the HTTP bind address: ARC_HTTP_PORT wins over PORT, both
// defaulting to 8080.
func httpAddr() string {
	if p := os.Getenv("ARC_HTTP_PORT"); p != "" {
		return ":" + p
	}
	if p := os.Getenv("PORT"); p != "" {
		return ":" + p
	}
	return ":8080"
}

// allowedOrigin reads the single CORS origin, defaulting to the frontend dev
// server.
func allowedOrigin() string {
	if o := os.Getenv("ALLOWED_ORIGIN"); o != "" {
		return o
	}
	return "http://localhost:3000"
}

func handleHealthz(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": "ok"})
}

// handleListSpaces answers with the distinct spaces of the index, folding
// empty KnowledgeSpaceIDs into the default space. One full scan replaces any
// space-aware store query for v1.
func (s *server) handleListSpaces(c *fiber.Ctx) error {
	points, err := s.runtime.VectorStore().ListPoints(c.Context(), indexingmodel.MetadataFilter{})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(groupSpaces(points))
}

// groupSpaces buckets a full point scan by KnowledgeSpaceID and counts
// distinct documents per space.
func groupSpaces(points []store.VectorPoint) []SpaceSummary {
	bySpace := make(map[string]map[string]bool)
	for _, pt := range points {
		spaceID := pt.Metadata.KnowledgeSpaceID
		if spaceID == "" {
			spaceID = defaultSpaceID
		}
		docs := bySpace[spaceID]
		if docs == nil {
			docs = make(map[string]bool)
			bySpace[spaceID] = docs
		}
		if pt.Metadata.DocumentID != "" {
			docs[pt.Metadata.DocumentID] = true
		}
	}
	spaces := make([]SpaceSummary, 0, len(bySpace))
	for id, docs := range bySpace {
		name := id
		if id == defaultSpaceID {
			name = "Default Space"
		}
		spaces = append(spaces, SpaceSummary{ID: id, Name: name, DocumentCount: len(docs)})
	}
	sort.Slice(spaces, func(i, j int) bool { return spaces[i].ID < spaces[j].ID })
	return spaces
}

// handleListDocuments answers with the documents of one space, grouping a
// full scan by document_id, and enriches each from its document_profile
// point when present.
func (s *server) handleListDocuments(c *fiber.Ctx) error {
	points, err := s.runtime.VectorStore().ListPoints(c.Context(), indexingmodel.MetadataFilter{})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(groupDocuments(points, c.Params("spaceId")))
}

// groupDocuments groups points by document_id within one space and derives
// title and author from the first profile point's deterministic payload.
func groupDocuments(points []store.VectorPoint, spaceID string) []DocumentSummary {
	docs := make(map[string]*DocumentSummary)
	for _, pt := range points {
		if !spaceMatches(pt.Metadata.KnowledgeSpaceID, spaceID) {
			continue
		}
		docID := pt.Metadata.DocumentID
		if docID == "" {
			continue
		}
		ds := docs[docID]
		if ds == nil {
			ds = &DocumentSummary{DocumentID: docID}
			docs[docID] = ds
		}
		// Chunk count excludes the document_profile point (ADR-0048): the
		// upload done event and the listing must agree on what a chunk is.
		if pt.Metadata.ContentType != pdfmodel.ContentTypeDocumentProfile {
			ds.ChunkCount++
		}
		if pt.Metadata.ContentType == pdfmodel.ContentTypeDocumentProfile {
			title, author := profileFields(pt.ContentMarkdown)
			if title != "" {
				ds.Title = title
			}
			if author != "" {
				ds.Author = author
			}
		}
	}
	out := make([]DocumentSummary, 0, len(docs))
	for _, ds := range docs {
		out = append(out, *ds)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DocumentID < out[j].DocumentID })
	return out
}

// spaceMatches decides whether a point belongs to a requested space. v1
// indexing never sets the KnowledgeSpaceID, so the empty ID and the literal
// default resolve identically.
func spaceMatches(pointSpace, requested string) bool {
	if requested == "" || requested == defaultSpaceID {
		return pointSpace == "" || pointSpace == defaultSpaceID
	}
	return pointSpace == requested
}

// profileFields reads the title and author lines out of a document_profile
// point's deterministic Markdown (worker.BuildDocumentProfileContent shape).
// Exact line prefixes only; a payload in any other shape contributes nothing.
func profileFields(content string) (title, author string) {
	for _, line := range strings.Split(content, "\n") {
		switch {
		case strings.HasPrefix(line, "Title: "):
			title = strings.TrimPrefix(line, "Title: ")
		case strings.HasPrefix(line, "Author: "):
			author = strings.TrimPrefix(line, "Author: ")
		}
	}
	return title, author
}

// handleGetChunk resolves one KnowledgeChunk from the vector store by chunk
// id, 404ing when the id is unknown.
func (s *server) handleGetChunk(c *fiber.Ctx) error {
	chunkID := c.Params("*")
	points, err := s.runtime.VectorStore().ListPoints(c.Context(), indexingmodel.MetadataFilter{ChunkIDs: []string{chunkID}})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if len(points) == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": fmt.Sprintf("chunk %q not found", chunkID)})
	}
	pt := points[0]
	return c.JSON(KnowledgeChunk{
		ChunkID:         pt.Metadata.ChunkID,
		DocumentID:      pt.Metadata.DocumentID,
		SectionPath:     pt.Metadata.SectionPath,
		PageNumbers:     pt.Metadata.PageNumbers,
		ContentMarkdown: pt.ContentMarkdown,
		ContentType:     pt.Metadata.ContentType,
	})
}

// parseDocumentIDs splits a comma-separated documentIds query parameter
// into trimmed, non-empty ids. Empty input returns nil so the filter field
// stays absent and retrieval keeps its unfiltered behavior.
func parseDocumentIDs(raw string) []string {
	var ids []string
	for _, part := range strings.Split(raw, ",") {
		if id := strings.TrimSpace(part); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return ids
}

// clampTopK parses the topK query parameter with a default of 5, bound to
// 1..20. Unparseable values fall back to the default.
func clampTopK(raw string) int {
	topK := 5
	if raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			topK = n
		}
	}
	if topK < 1 {
		return 1
	}
	if topK > 20 {
		return 20
	}
	return topK
}

// handleQAStream streams every AnswerStreamChunk as one SSE data event. An
// empty q is a client error (400 JSON); pipeline failures arrive as an error
// chunk inside the stream, never as a curl-visible status code. A comma-
// separated documentIds scopes retrieval to those documents and ANDs with
// spaceId, so a canvas question against one dropped document never sees
// other documents' evidence.
func (s *server) handleQAStream(c *fiber.Ctx) error {
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "query parameter q is required"})
	}
	filter := indexingmodel.MetadataFilter{}
	if spaceID := c.Query("spaceId"); spaceID != "" && spaceID != defaultSpaceID {
		filter.KnowledgeSpaceID = spaceID
	}
	filter.DocumentIDs = parseDocumentIDs(c.Query("documentIds"))
	query := retrievalseam.RetrievalQuery{
		QueryText: q,
		TopK:      clampTopK(c.Query("topK")),
		Filter:    filter,
	}

	chunks, err := s.engine.AnswerStream(c.Context(), query)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	setSSEHeaders(c)

	// The stream writer flushes after each event so the canvas renders
	// tokens as they arrive instead of at stream end.
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		for chunk := range chunks {
			data, err := json.Marshal(chunk)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
				return
			}
			if err := w.Flush(); err != nil {
				return
			}
		}
	})
	return nil
}

// setSSEHeaders marks a response as a no-cache event stream; both streaming
// endpoints share the same header set so proxies treat them identically.
func setSSEHeaders(c *fiber.Ctx) {
	c.Set(fiber.HeaderContentType, "text/event-stream")
	c.Set(fiber.HeaderCacheControl, "no-cache")
	c.Set(fiber.HeaderConnection, "keep-alive")
	c.Set("X-Accel-Buffering", "no")
}

// deriveDocumentID applies the CLI's document id rule to an uploaded
// filename: the base name without its extension, so the browser upload and
// `arc inspect` land on the same id for the same file.
func deriveDocumentID(filename string) string {
	return filepath.Base(strings.TrimSuffix(filename, filepath.Ext(filename)))
}

// handleUploadDocument ingests one uploaded PDF through the shared
// inspect -> index pipeline, streaming each phase as an SSE event so a
// 1-3 minute job is not a silent spinner. A missing file field or an
// unsupported spaceId is a 400 JSON before the stream; any failure after
// the stream starts is an error event. The document always lands in the
// default space: v1 indexing persists no space, so spaceId is validated
// but never stored.
func (s *server) handleUploadDocument(c *fiber.Ctx) error {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "multipart form field 'file' is required"})
	}
	if spaceID := c.FormValue("spaceId"); spaceID != "" && spaceID != defaultSpaceID {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fmt.Sprintf("spaceId %q is not supported: v1 uploads land in the default space", spaceID),
		})
	}
	docID := deriveDocumentID(fileHeader.Filename)
	if docID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "cannot derive a document id from the uploaded filename"})
	}

	setSSEHeaders(c)

	// The upload body is fully buffered by FormFile before the stream
	// starts; from here on every failure becomes an error event so the
	// browser never mistakes a broken ingest for a connection loss.
	ctx := c.Context()
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		if !strings.EqualFold(filepath.Ext(fileHeader.Filename), ".pdf") {
			writeUploadEvent(w, UploadErrorEvent{Type: uploadEventError, Error: fmt.Sprintf(
				"unsupported file type %q: only .pdf documents are accepted", filepath.Ext(fileHeader.Filename))})
			return
		}

		data, err := readUploadFile(fileHeader)
		if err != nil {
			writeUploadEvent(w, UploadErrorEvent{Type: uploadEventError, Error: err.Error()})
			return
		}

		writeUploadEvent(w, UploadPhaseEvent{Type: uploadEventPhase, Phase: uploadPhaseParse})

		result, err := s.runtime.Inspect(ctx, docID, data)
		if err != nil {
			writeUploadEvent(w, inspectErrorEvent(err, result))
			return
		}
		if result.Diagnostics.Status == pdfmodel.StatusFailed {
			writeUploadEvent(w, UploadErrorEvent{Type: uploadEventError, Error: fmt.Sprintf(
				"inspection failed: %v", result.Diagnostics.Errors)})
			return
		}

		writeUploadEvent(w, UploadPhaseEvent{Type: uploadEventPhase, Phase: uploadPhaseChunk, ChunkCount: len(result.Chunks)})
		writeUploadEvent(w, UploadPhaseEvent{Type: uploadEventPhase, Phase: uploadPhaseEmbed})

		jobObj, err := s.runtime.Index(ctx, result.Document.DocumentID, result.Document.Title, result.Chunks, &result.Document)
		if err != nil {
			writeUploadEvent(w, UploadErrorEvent{Type: uploadEventError, Error: fmt.Sprintf("indexing failed: %v", err)})
			return
		}

		writeUploadEvent(w, UploadPhaseEvent{Type: uploadEventPhase, Phase: uploadPhaseIndexed, Indexed: jobObj.IndexedChunks, Skipped: jobObj.SkippedChunks})
		writeUploadEvent(w, UploadDoneEvent{Type: uploadEventDone, Document: UploadDocumentSummary{
			DocumentID:   result.Document.DocumentID,
			Title:        result.Document.Title,
			ChunkCount:   len(result.Chunks),
			Status:       result.Diagnostics.Status,
			Indexed:      jobObj.IndexedChunks,
			Skipped:      jobObj.SkippedChunks,
			Deleted:      jobObj.DeletedChunks,
			WarningCount: len(result.Diagnostics.Warnings),
		}})
	})
	return nil
}

// inspectErrorEvent builds the error event for an inspection failure,
// mirroring the CLI's error semantics: the aggregate diagnostics errors join
// the message when the inspector returned a failed result alongside the
// error.
func inspectErrorEvent(err error, result *pdfmodel.PDFInspectionResult) UploadErrorEvent {
	if result != nil && result.Diagnostics.Status == pdfmodel.StatusFailed {
		return UploadErrorEvent{Type: uploadEventError, Error: fmt.Sprintf(
			"inspection failed: %v (errors: %v)", err, result.Diagnostics.Errors)}
	}
	return UploadErrorEvent{Type: uploadEventError, Error: fmt.Sprintf("inspection failed: %v", err)}
}

// readUploadFile slurps a multipart file header into memory so the inspect
// seam receives the same byte slice the CLI reads from disk.
func readUploadFile(fileHeader *multipart.FileHeader) ([]byte, error) {
	file, err := fileHeader.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open uploaded file: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read uploaded file: %w", err)
	}
	return data, nil
}

// writeUploadEvent marshals one upload stream event to the SSE data line
// shape shared with qa/stream and flushes it. Marshal errors and broken
// connections end the stream best-effort; the client sees the stream close.
func writeUploadEvent(w *bufio.Writer, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return
	}
	_ = w.Flush()
}

// handleDeleteDocument removes one document's points and graph nodes,
// answering 200 with the deleted id. The existence check runs before the
// delete: a document with no matching points is a 404, so a client cannot
// mistake a re-delete for a successful removal.
func (s *server) handleDeleteDocument(c *fiber.Ctx) error {
	docID := c.Params("documentId")
	points, err := s.runtime.VectorStore().ListPoints(c.Context(), indexingmodel.MetadataFilter{DocumentIDs: []string{docID}})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if len(points) == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": fmt.Sprintf("document %q not found", docID)})
	}
	if err := s.runtime.DeleteDocument(c.Context(), docID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"deleted": docID})
}

func main() {
	cfg := cli.LoadFromEnv()
	runtime, err := cli.NewRuntime(cfg)
	if err != nil {
		log.Fatalf("failed to construct ARC runtime: %v", err)
	}
	engine, err := runtime.AnswerEngine()
	if err != nil {
		log.Fatalf("failed to construct answer engine: %v", err)
	}

	app := newApp(runtime, engine, allowedOrigin())
	addr := httpAddr()
	log.Printf("ARC server listening on %s (REST: /api/v1/spaces, /api/v1/spaces/:id/documents, /api/v1/chunks/:id, /api/v1/qa/stream, /api/v1/documents)", addr)
	log.Fatal(app.Listen(addr))
}
