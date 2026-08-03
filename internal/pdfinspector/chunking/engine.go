package chunking

import (
	"context"
	"fmt"
	"strings"

	"arca/internal/pdfinspector/model"
)

// Engine defines the interface for Hierarchical Semantic Chunking.
// ChunkDocument requires an explicit document ID so every produced KnowledgeChunk
// carries the correct document_id for multi-document isolation and indexing.
// When a PageMap (from json_layout.pages) is provided it is used to resolve
// authoritative page numbers for every chunk.
type Engine interface {
	ChunkDocument(ctx context.Context, docID string, tree *model.SemanticTree, markdown string, pageMap []model.PageMap) ([]model.KnowledgeChunk, error)
}

// DefaultEngine implements Engine with thread-safe diagnostic warning collection.
type DefaultEngine struct {
	opts      Options
	parser    BlockParser
	builder   ChunkBuilder
	collector *WarningCollector
}

// NewEngine creates a new Chunking Engine instance with functional options.
func NewEngine(opts ...Option) *DefaultEngine {
	cfg := DefaultOptions()
	for _, o := range opts {
		o(&cfg)
	}

	return &DefaultEngine{
		opts:      cfg,
		parser:    NewBlockParser(),
		builder:   NewChunkBuilder(),
		collector: NewWarningCollector(),
	}
}

// Warnings returns diagnostic warnings accumulated during document chunking.
func (e *DefaultEngine) Warnings() []string {
	return e.collector.Warnings()
}

// ChunkDocument performs hierarchical section-aware semantic chunking.
// docID is authoritative per call; it overrides any construction-time WithDocumentID
// default. If both are empty the call fails loudly rather than silently colliding
// on a shared default document.
func (e *DefaultEngine) ChunkDocument(ctx context.Context, docID string, tree *model.SemanticTree, markdown string, pageMap []model.PageMap) ([]model.KnowledgeChunk, error) {
	e.collector.Clear()

	if strings.TrimSpace(docID) == "" {
		docID = e.opts.DocumentID
	}
	if strings.TrimSpace(docID) == "" {
		return nil, fmt.Errorf("chunking requires a non-empty document id")
	}

	blocks, err := e.parser.Parse(ctx, tree, markdown)
	if err != nil {
		e.collector.AddWarning(fmt.Sprintf("block parser error: %v", err))
		return nil, fmt.Errorf("failed to parse markdown blocks: %w", err)
	}

	// json_layout.pages is the authoritative page layout when present. The bundled
	// service emits no inline page markers, so without this step every chunk would
	// incorrectly land on page 1.
	if len(pageMap) > 0 {
		resolveBlockPages(blocks, pageMap)
	}

	cfg := e.opts
	cfg.DocumentID = docID

	chunks, err := e.builder.Build(ctx, blocks, cfg, e.collector)
	if err != nil {
		e.collector.AddWarning(fmt.Sprintf("chunk builder error: %v", err))
		return nil, fmt.Errorf("failed to build knowledge chunks: %w", err)
	}

	return chunks, nil
}
