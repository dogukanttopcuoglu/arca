# Glossary

## PDF Inspector
The entry-point stage of the knowledge ingestion pipeline inside ARC's Go backend (built with Fiber), responsible for calling the Firecrawl PDF Service via HTTP, analyzing the returned Markdown/JSON into semantic sections, generating knowledge chunks, preserving citations, and producing a unified `PDFInspectionResult`.

## Fiber
The fast, Express-inspired Go HTTP framework chosen for ARC's backend services and HTTP routing.

## Firecrawl PDF Service
A dedicated Node.js/TypeScript microservice running alongside ARC (e.g. via Docker) that handles raw PDF parsing, OCR detection, layout reconstruction, basic structure extraction, and Markdown conversion, exposing an HTTP API to ARC.

## Knowledge Ingestion Pipeline
The multi-stage transformation workflow in ARC that processes raw input documents (starting with PDFs) into structured Markdown, semantic sections, knowledge chunks, vector embeddings, and knowledge graph nodes for retrieval and QA.

## PDFInspectionResult
The deterministic, serializable, versioned intermediate representation emitted by PDF Inspector. It contains `document` metadata, `semanticTree`, `content` (Markdown + pageMap), `chunks` (KnowledgeChunk[]), extracted `assets` (tables, figures, codeBlocks, equations, citations), and pipeline `diagnostics`.

## Semantic Section / Tree
The hierarchical structural breakdown of a document (headings, sections, paragraphs) preserving reading order and parent-child relationships.

## Knowledge Chunk
A discrete, semantically intact segment generated via Hierarchical Semantic Chunking. Each chunk preserves section paths, parent/child chunk links, heading levels, page numbers, citations, source offsets, and content types (paragraph, table, code, list, equation, figure).

## Hierarchical Semantic Chunking
A chunking strategy combining section-aware splitting with parent-child relationships, prioritizing semantic boundary integrity over strict token limits (preferred 400-700 tokens, soft max 1000, absolute max 1200).

## Graceful Degradation
The resiliency strategy where partial failures (e.g., OCR or table extraction failure on isolated pages) do not fail the entire pipeline, returning partial inspection outputs accompanied by explicit diagnostics status (`success`, `partial_success`, `failed`), warnings, skipped pages, and error logs.

## Document Asset
Extracted non-prose elements (tables, figures, code blocks, equations, citations) indexed with structural location and page context.

## Semantic Metadata Enrichment Layer
The non-destructive post-extraction phase in PDF Inspector that resolves page numbers for `SemanticTree` heading nodes using normalized heading matching across `PageMap` and `KnowledgeChunk` provenance, while deriving document titles from first meaningful headings (`H1`/`H2`) when PDF metadata is absent.

## Indexing Job
A background task managing the asynchronous vector generation, differential signature comparison, and vector database upserting of extracted `KnowledgeChunk`s.

## EmbeddingProvider
The deep module interface abstracting external LLM embedding APIs (OpenAI, Gemini, Ollama, Voyage) behind a unified batching and capability seam.

## VectorStore
The persistence abstraction managing vector point storage, nearest-neighbor searches, and document deletions (implemented via Qdrant, PgVector, or InMemory adapters).

## IndexSignature
The composite hash (`ContentHash:EmbeddingProvider:EmbeddingModel:EmbeddingVersion:ChunkSchemaVersion`) used for differential re-indexing to avoid redundant embedding API calls.

## MetadataFilter
The strongly-typed, canonical filtering structure allowing domain queries by document ID, chunk ID, page numbers, and content types without leaking database-specific query syntax.

## Retriever
The top-level search abstraction encapsulating Dense Vector, Sparse Lexical (BM25), and Hybrid (RRF) retrieval strategies behind a clean query interface.

## AnswerEngine
The high-level RAG orchestration engine combining Query Understanding, Retrieval, Context Assembly, Prompt Engineering, and Evidence Verification via modular pipeline composition.

## QueryAnalyzer
The domain seam responsible for extracting user intent, named entities, and structural metadata filters from natural language queries.

## ContextBuilder
The QA orchestration component responsible for token budgeting, duplicate removal, section re-ordering, and immutable citation marker (`[Ref N]`) injection.

## ContextWindow
The prompt-ready structured payload containing deduplicated content, immutable source references, and total token count produced by `ContextBuilder`.

## PromptBuilder
The domain seam constructing system instructions, RAG citation rules, and user context payloads into vendor-agnostic `PromptMessage` structures.

## LLMProvider
The deep module interface abstracting external text generation APIs (OpenAI, Anthropic Claude, Ollama, Llama 3) behind a unified prompt execution seam.

## CitationExtractor
The verification component that parses inline reference markers (`[Ref N]`) from LLM outputs, maps them back to verified `ContextWindow` sources, and flags invalid or hallucinated references.

## VerificationPipeline
The evidence verification pipeline enforcing structural reference existence (Phase 1) and semantic NLI entailment scoring (Phase 2).

## QAJob
The asynchronous state machine (`Pending` -> `Planning` -> `Retrieving` -> `Generating` -> `Verifying` -> `Completed`) managing long-running multi-document deep research tasks.

## KnowledgeSpace
The canonical 3-tier domain isolation boundary (`Workspace` -> `KnowledgeSpace` -> `Document` -> `KnowledgeChunk`) enabling multi-document RAG across libraries and projects.

## GraphStore
The graph persistence abstraction managing nodes (`Document`, `Section`, `Chunk`, `Entity`, `Concept`, `Citation`) and edges for relational GraphRAG traversals.

## GraphRetriever
The graph search adapter implementing the `Retriever` interface seam to enable 3-way RRF score fusion (Dense + Sparse + Graph).

## AgentEngine
The top-level autonomous reasoning controller (`Planner` + `Executor`) orchestrating multi-step research tasks, tool calls (`KnowledgeTool`, `GraphTool`, `MCPTool`), and security policies (`MaxSteps`, `ApprovalSeam`).

## EnricherPass
The deep module interface seam representing an isolated compiler-pass style enrichment stage (`Name`, `Requires`, `Provides`, `Execute`).

## CompositeEnricher
The enrichment pipeline orchestrator executing an ordered sequence of `EnricherPass` stages with contract validation.

## TitleResolver
The fallback chain seam (`PDFMetadata` -> `Heading` -> `TOC` -> `LLM` -> `Unknown`) deriving document title without hardcoded book heuristics.

## LanguageDetectionPass
An early enrichment pass determining document ISO language code (`"en"`, `"tr"`) to configure downstream stopwords and tokenization.

## KeywordExtractor
The pluggable strategy seam (`Extract(ctx, chunks, lang) ([]Keyword, error)`) and registry supporting `rule_based`, `llm`, and `hybrid` extractors.

## Keyword
Structured metadata object (`Value`, `Score`, `Source`, `ChunkIDs`) attached to both document and individual `KnowledgeChunk` instances.

## EntityType
Typed classification enum (`"person"`, `"organization"`, `"location"`, `"product"`, `"event"`, `"miscellaneous"`).

## EntityMention
Surface text occurrence of a typed entity (`Text`, `Type`, `ChunkID`, `Confidence`) attached to individual chunks.

## Entity
Document-level aggregated entity record (`ID`, `Name`, `Type`, `Aliases`, `Mentions`, `Score`).

## EntityExtractorPass
The compiler-pass stage running `EntityExtractor` strategies to extract mentions and populate document/chunk entities.






