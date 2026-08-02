# Technical Specification: Knowledge Indexing & Retrieval Engine

## Problem Statement

The ARC PDF Inspector pipeline extracts canonical PDF inspection results containing structured `KnowledgeChunk`s. However, without a dedicated, decoupled Knowledge Indexing & Retrieval Engine:
- Downstream Question Answering and RAG applications cannot perform semantic vector search or hybrid retrieval over extracted document chunks.
- Large document re-indexing causes expensive, redundant LLM embedding API calls.
- High-latency embedding API calls block the primary PDF inspection pipeline.
- Tight coupling to specific embedding models or vector databases restricts infrastructure flexibility.

## Solution

Build a decoupled, asynchronous Knowledge Indexing & Retrieval Engine (`internal/indexing`, `internal/retrieval`) based on **ADR-0008**. The engine introduces:
- A queue-agnostic **Async Indexing Worker** for background execution of durable `IndexingJob`s.
- Cost-optimized **Differential Re-Indexing** using a composite `IndexSignature`.
- A minimal, deep **EmbeddingProvider** seam (`OpenAI`, `Gemini`, `Ollama`, `Voyage`).
- A strongly-typed **VectorStore** seam with a zero-dependency **InMemoryVectorStore** adapter for offline testing.
- A canonical **MetadataFilter** for type-safe queries without database syntax leakage.
- A decoupled **Retriever** seam supporting `Dense`, `Sparse`, and `Hybrid` (RRF) retrieval strategies.

## User Stories

1. As a RAG pipeline developer, I want to execute semantic vector searches over extracted document chunks, so that I can provide context to LLM prompts.
2. As a backend engineer, I want PDF extraction to complete in ~500ms without waiting for external embedding API network calls, so that the ingestion API remains responsive.
3. As a DevOps engineer, I want embedding provider outages to not break PDF extraction, so that ingestion jobs complete resiliently.
4. As an API developer, I want document re-indexing to skip unchanged chunks automatically via `IndexSignature` matching, so that third-party LLM embedding API costs are minimized.
5. As an infrastructure engineer, I want to swap vector databases (Qdrant to PgVector or Milvus) by adding a single adapter, so that downstream retrieval applications require zero code changes.
6. As a machine learning engineer, I want to change embedding models or versions, so that the pipeline automatically detects the version mismatch and executes a full re-index.
7. As an integration tester, I want to execute vector indexing and retrieval unit tests completely offline using an in-memory vector store, so that CI test suites run fast without Docker dependencies.
8. As a search specialist, I want to filter vector searches by document ID, page numbers, or content types using a strongly-typed `MetadataFilter`, so that queries do not leak database-specific JSON/SQL syntax.
9. As a systems architect, I want retrieval queries to be exposed strictly behind a high-level `Retriever` interface, so that low-level `VectorStore` details are never exposed to LLM context builders.
10. As a RAG researcher, I want the retrieval layer to support Hybrid Retrieval combining Dense Vector and Sparse Lexical BM25 search via Reciprocal Rank Fusion (RRF), so that keyword and semantic queries yield high-precision results.

## Implementation Decisions

### 1. Invariant Domain Models & Point ID Generation
Point IDs **MUST** remain stable across content revisions, generated deterministically as:

```go
PointID = SHA256(DocumentID + ":" + SectionPath + ":" + ChunkOrder)
```

`ContentHash` is stored in `VectorMetadata` and **MUST NOT** be part of Point ID calculation.

### 2. Async Job-Based Indexing & State Machine
Indexing is executed asynchronously by an `IndexingWorker` processing durable `IndexingJob`s. Indexing status follows an explicit state machine: `Pending` → `Running` → `Completed` | `Failed` | `Retrying` | `Cancelled`.

### 3. EmbeddingProvider Seam
Abstracted via a minimal interface isolating provider-specific constraints:

```go
type EmbeddingProvider interface {
    GenerateEmbeddings(ctx context.Context, texts []string) (*EmbeddingResult, error)
    Capabilities() ProviderCapabilities
    Health(ctx context.Context) error
}

type ProviderCapabilities struct {
    Dimension     int
    MaxBatchSize  int
    MaxInputTokens int
}
```

### 4. VectorStore Seam & InMemory Adapter
`VectorStore` operates on strongly-typed `VectorMetadata` with collection scopes bound at construction time:

```go
type VectorStore interface {
    UpsertPoints(ctx context.Context, points []VectorPoint) error
    SearchVector(ctx context.Context, query VectorSearchQuery) ([]VectorSearchResult, error)
    Delete(ctx context.Context, filter MetadataFilter) error
    Health(ctx context.Context) error
}
```

An `InMemoryVectorStore` adapter implements cosine similarity for zero-dependency testing.

### 5. Composite IndexSignature & Differential Re-Indexing
Diff Engine compares the 5-part `IndexSignature`:

```go
IndexSignature = ContentHash + ":" + EmbeddingProvider + ":" + EmbeddingModel + ":" + EmbeddingVersion + ":" + ChunkSchemaVersion
```

Chunks are classified into: `UNCHANGED` (skip API call), `CONTENT_CHANGED` (re-embed & inplace upsert), `MODEL_CHANGED` (re-embed), `NEW` (insert), and `DELETED` (purge).

### 6. Canonical MetadataFilter
Domain-level query filter isolating database-specific syntax:

```go
type MetadataFilter struct {
    DocumentIDs       []string
    ChunkIDs          []string
    PageNumbers       []int
    ContentTypes      []model.ContentType
    SectionPathPrefix string
    IndexedAfter      *time.Time
}
```

### 7. Decoupled Retriever Seam & RRF Hybrid Scaffolding
Retrieval is exposed exclusively through the `Retriever` interface:

```go
type RetrievalMode int

const (
    RetrievalDense RetrievalMode = iota
    RetrievalSparse
    RetrievalHybrid
)

type RetrievalQuery struct {
    QueryText string
    Filter    MetadataFilter
    TopK      int
    Mode      RetrievalMode
}

type Retriever interface {
    Retrieve(ctx context.Context, query RetrievalQuery) ([]SearchResult, error)
}
```

RRF score fusion merges Dense and Sparse ranked streams using $RRF(d) = \sum \frac{1}{k + r(d)}$.

## Testing Decisions

### 1. Test Surfaces & Highest Seams
- **Primary Execution Seam (`IndexingWorker.ExecuteSync`)**: Tested using `MockEmbeddingProvider` and `InMemoryVectorStore` to verify end-to-end ingestion, differential diff calculation, and point storage.
- **Primary Search Seam (`Retriever.Retrieve`)**: Tested via `DenseRetriever` and `HybridRetriever` using `InMemoryVectorStore` to verify metadata filtering, cosine similarity ranking, and RRF score fusion.

### 2. Testing Principles
- Tests **MUST** verify external behavior through public interface seams. No testing of internal private struct state.
- All core indexing and retrieval tests **MUST** run completely offline without external network or Docker services.
- Concurrent read/write safety **MUST** be verified via `go test -race`.

## Out of Scope

- Production HTTP integrations with OpenAI, Gemini, Ollama, or Voyage (deferred to adapter tickets).
- External vector database drivers for Qdrant, PgVector, or Milvus (deferred to adapter tickets).
- Full inverted-index BM25 storage engine (scaffolding and RRF fusion provided only).
- Direct mutation of canonical PDF inspection JSONs.

## Further Notes

- All implementation packages (`internal/indexing/...` and `internal/retrieval/...`) must maintain zero direct dependencies on Fiber or Firecrawl types.
- The `InMemoryVectorStore` adapter will be retained permanently in the codebase for fast unit testing and benchmark suites.
