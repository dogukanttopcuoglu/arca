package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
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

	app := fiber.New()
	app.Use(cors.New(cors.Config{AllowOrigins: allowedOrigin}))

	app.Get("/healthz", handleHealthz)

	api := app.Group("/api/v1")
	api.Get("/spaces", s.handleListSpaces)
	api.Get("/spaces/:spaceId/documents", s.handleListDocuments)
	// Chunk IDs embed slashes (documentID/section/ordinal); the wildcard
	// captures the full id rather than the first path segment.
	api.Get("/chunks/*", s.handleGetChunk)
	api.Get("/qa/stream", s.handleQAStream)

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
		ds.ChunkCount++
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
// chunk inside the stream, never as a curl-visible status code.
func (s *server) handleQAStream(c *fiber.Ctx) error {
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "query parameter q is required"})
	}
	query := retrievalseam.RetrievalQuery{
		QueryText: q,
		TopK:      clampTopK(c.Query("topK")),
	}
	if spaceID := c.Query("spaceId"); spaceID != "" && spaceID != defaultSpaceID {
		query.Filter = indexingmodel.MetadataFilter{KnowledgeSpaceID: spaceID}
	}

	chunks, err := s.engine.AnswerStream(c.Context(), query)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	c.Set(fiber.HeaderContentType, "text/event-stream")
	c.Set(fiber.HeaderCacheControl, "no-cache")
	c.Set(fiber.HeaderConnection, "keep-alive")
	c.Set("X-Accel-Buffering", "no")

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
	log.Printf("ARC server listening on %s (REST: /api/v1/spaces, /api/v1/spaces/:id/documents, /api/v1/chunks/:id, /api/v1/qa/stream)", addr)
	log.Fatal(app.Listen(addr))
}
