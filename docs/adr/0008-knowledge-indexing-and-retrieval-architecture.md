# ADR 0008: Knowledge Indexing and Retrieval Architecture

- Status: Accepted
- Date: 2026-08-02
- Decision Makers: ARC Engineering Team

## Context

The ARC PDF Inspector pipeline extracts canonical `PDFInspectionResult` documents containing structured `KnowledgeChunk`s. The extracted canonical document becomes the persistent source of truth from which all downstream indexing, embedding generation, and retrieval pipelines operate.

To support downstream Vector Search, Question Answering, and RAG (Retrieval-Augmented Generation) workflows, ARC requires a robust, scalable, and decoupled Knowledge Indexing & Retrieval Architecture.

Key requirements for this architecture include:

- Provider flexibility (OpenAI, Gemini, Ollama, Voyage) without API leakage.
- Vector store flexibility (Qdrant, PgVector, Milvus, In-Memory) without query syntax leakage.
- Idempotent upserts and cost-optimized differential re-indexing.
- Asynchronous execution to keep PDF inspection latency low and resilient to provider outages.
- Strongly-typed filtering and future-proof Hybrid Retrieval (Dense Vector + Sparse Lexical BM25).

## Architectural Principles & Decisions

### Decision 1: Stable Point IDs & In-Place Revision Upserts
Vector Store Point IDs **SHALL** be stable across content revisions, derived deterministically as:

$$\text{PointID} = \text{SHA256}(\text{DocumentID} + \text{":"} + \text{SectionPath} + \text{":"} + \text{ChunkOrder})$$

`ContentHash` **SHALL NOT** participate in Point ID generation; it **SHALL** be stored within `VectorMetadata` for change detection and idempotent in-place upserts.

### Decision 2: Asynchronous Job-Based Indexing & Queue Agnosticism
Embedding generation **SHALL** be asynchronous by default, executed by a dedicated `Indexing Worker` consuming durable indexing jobs. The PDF inspection pipeline **SHALL** complete independently of embedding generation. A synchronous indexing mode **MAY** be provided for CLI and testing scenarios.

The architecture **SHALL NOT** depend on a specific queue implementation (e.g. Go worker pools, PostgreSQL job queues, RabbitMQ, Kafka, or NATS).

Document indexing status **SHALL** be tracked explicitly through a formal state machine:
`Pending` → `Running` → `Completed` | `Failed` | `Retrying` | `Cancelled`.

### Decision 3: Minimal EmbeddingProvider Interface & Capabilities Encapsulation
Embedding providers **SHALL** be abstracted via a minimal `EmbeddingProvider` interface:

```go
type EmbeddingProvider interface {
    GenerateEmbeddings(ctx context.Context, texts []string) (*EmbeddingResult, error)
    Capabilities() ProviderCapabilities
    Health(ctx context.Context) error
}
```

`EmbeddingResult` **SHALL** include explicit `Provider`, `Model`, and `Version` metadata. Provider-specific constraints (dimensions, max batch sizes, token limits) **SHALL** be encapsulated within `ProviderCapabilities` to preserve interface stability when adding new provider adapters (OpenAI, Gemini, Ollama, Voyage).

### Decision 4: Strongly-Typed VectorStore Seam & InMemory Testing Adapter
Vector storage **SHALL** be abstracted via a `VectorStore` interface utilizing strongly-typed `VectorMetadata` and constructor-bound collection scopes.

`VectorStore` **SHALL** remain unaware of higher-level domain entities (`Document`, `KnowledgeChunk`) or retrieval semantics; it operates exclusively on low-level points, vectors, and metadata payloads.

An `InMemoryVectorStore` adapter **SHALL** be implemented to support zero-dependency unit tests, benchmarks, and offline CLI execution without requiring Docker or external vector database services.

### Decision 5 & 6: Differential Re-Indexing via Composite IndexSignature
The indexing pipeline **SHALL** perform differential re-indexing using an explicit `IndexSignature`:

$$\text{IndexSignature} = \text{ContentHash} + \text{":"} + \text{EmbeddingProvider} + \text{":"} + \text{EmbeddingModel} + \text{":"} + \text{EmbeddingVersion} + \text{":"} + \text{ChunkSchemaVersion}$$

Chunks with unchanged signatures **SHALL** be skipped to minimize API costs and latency. New chunks **SHALL** be inserted, removed chunks **SHALL** be deleted, and modified chunks **SHALL** be updated in-place.

### Decision 7: Canonical Strongly-Typed MetadataFilter
`MetadataFilter` **SHALL** be the canonical, strongly-typed filtering abstraction across ingestion and retrieval pipelines:

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

Storage-specific query languages (e.g., Qdrant JSON filters, SQL WHERE clauses, Milvus expressions) **SHALL** remain private implementation details of the corresponding `VectorStore` adapters.

### Decision 8: Decoupled Retrieval Seam & Hybrid Retrieval Architecture
Retrieval **SHALL** be exposed exclusively through the `Retriever` abstraction:

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

`VectorStore` **SHALL** remain a low-level persistence component responsible only for vector storage and nearest-neighbor search. Retrieval strategies (Dense, Sparse, Hybrid) and ranking algorithms (e.g., Reciprocal Rank Fusion) **SHALL** be encapsulated behind the `Retriever` seam.

Future extension: A `Query Planner` **MAY** automatically select `Dense`, `Sparse`, or `Hybrid` retrieval modes based on query characteristics.

## End-to-End Architecture Data Flow

```
Upload PDF
    │
    ▼
InspectPDF (PDFInspector Pipeline)
    │
    ▼
Canonical PDFInspectionResult (Source of Truth)
    │
    ▼
Persist Document & Enqueue Indexing Job
    │
    ├─────────────────────────────► Response: Document Extracted (Indexing: Pending)
    │
    ▼
Indexing Worker (Queue-Agnostic Engine)
    │
    ├── Compute IndexSignature Diff
    ├── Generate Embeddings via EmbeddingProvider (OpenAI / Gemini / Ollama)
    ├── Upsert Vectors into VectorStore (Qdrant / PgVector / InMemory)
    └── Update Indexing Status (Completed)
    │
    ▼
Retriever Seam (Dense / Sparse / Hybrid RRF / Query Planner)
    │
    ▼
RAG Pipeline / LLM Context Builder
```

## Consequences

- Operational Resilience: Embedding provider outages or rate limits do not affect PDF extraction because indexing is executed asynchronously.
- High Throughput: PDF extraction completes in ~500ms without blocking on external embedding network calls.
- Cost Efficiency: Differential re-indexing skips unchanged chunks based on `IndexSignature`.
- Testability: `InMemoryVectorStore` enables 100% offline unit tests without external services.
- Provider Agility: Switching embedding providers or vector databases requires creating a new adapter without touching downstream pipelines.
