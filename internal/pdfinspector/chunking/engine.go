package chunking

import (
	"context"
	"fmt"

	"arca/internal/pdfinspector/model"
)

// Engine defines the interface for Hierarchical Semantic Chunking.
type Engine interface {
	ChunkDocument(ctx context.Context, tree *model.SemanticTree, markdown string) ([]model.KnowledgeChunk, error)
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
func (e *DefaultEngine) ChunkDocument(ctx context.Context, tree *model.SemanticTree, markdown string) ([]model.KnowledgeChunk, error) {
	e.collector.Clear()

	blocks, err := e.parser.Parse(ctx, tree, markdown)
	if err != nil {
		e.collector.AddWarning(fmt.Sprintf("block parser error: %v", err))
		return nil, fmt.Errorf("failed to parse markdown blocks: %w", err)
	}

	chunks, err := e.builder.Build(ctx, blocks, e.opts, e.collector)
	if err != nil {
		e.collector.AddWarning(fmt.Sprintf("chunk builder error: %v", err))
		return nil, fmt.Errorf("failed to build knowledge chunks: %w", err)
	}

	return chunks, nil
}
