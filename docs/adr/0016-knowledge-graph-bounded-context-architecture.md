# 0016: Knowledge Graph Bounded Context Architecture

- **Status:** Accepted
- **Date:** 2026-08-02
- **Deciders:** Staff Software Architect & Lead Engineer

## Context and Problem Statement

ARC extracts rich semantic structures (`SemanticTree`, `Assets`, `Citations`). To support multi-hop reasoning and relational concept searches (GraphRAG), we need a formal Knowledge Graph architecture.

## Decision Drivers

- **GraphRAG Support**: Multi-hop queries (e.g. "What concepts connect Rick Rubin's view on fear to flow state?") require explicit graph relations.
- **Deep Module Seam Integration**: `GraphRetriever` must implement the clean `Retriever` interface seam to participate in Reciprocal Rank Fusion (RRF).

## Decided Options

### Option A: `internal/graph` Bounded Context (ACCEPTED)
- Knowledge Graph is an independent Bounded Context in `internal/graph` (`model`, `store`, `extractor`, `traverser`).
- Domain Nodes: `DocumentNode`, `SectionNode`, `ChunkNode`, `EntityNode`, `ConceptNode`, `CitationNode`.
- `GraphStore` seam: `AddNode`, `AddEdge`, `Traverse` (with `InMemoryGraphStore` MVP adapter).
- `GraphRetriever`: Implements `Retriever` interface seam. `HybridRetriever` performs 3-way RRF fusion:
  $$\text{Dense Vector Search} + \text{Sparse BM25 Search} + \text{Graph Traversal Search} \xrightarrow{\text{RRF Fusion}} \text{SearchResults}$$
- `EntityExtractor` seam: Decoupled from specific LLM providers.

## Consequences

### Positive
- True GraphRAG capability without coupling vector store logic to graph traversal logic.
- Graph search plugs directly into existing `HybridRetriever` via RRF score fusion.
