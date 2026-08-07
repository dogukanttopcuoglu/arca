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
The high-level RAG orchestration engine combining Query Understanding, Retrieval, Context Assembly, Prompt Engineering, and Evidence Verification via modular pipeline composition, emitting provider-agnostic `Answer` objects.

## Answer
The final generated, evidence-verified response to a user query. An `Answer` carries the answer text, verified `AnswerCitation`s, a `VerificationReport`, and provider-agnostic `AnswerMetadata`; it never carries provider-specific fields directly.
_Avoid_: Response, Completion, Reply

## AnswerDraft
The intermediate pre-generation payload (analyzed query, retrieved search results, and assembled context window) produced by the early stages of the `AnswerEngine` pipeline. A draft is not an answer until it has been generated and verified.
_Avoid_: IntermediateAnswer

## AnswerMetadata
The provider-neutral metadata attached to an `Answer` (provider name, model identifier, token usage), keeping the domain model independent of any specific LLM provider.
_Avoid_: ProviderInfo, ProviderDetails

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

## VerificationStatus
The explicit evidence state of an `Answer` (`verified`, `unverified`, `no_evidence`), carried on the answer itself rather than inferred from citation counts. `no_evidence` marks answers produced without any retrieved sources.

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

## ConceptSource
Typed provenance enum (`"rule_based"`, `"llm"`, `"hybrid"`).

## Concept
Minimal domain object representing an abstract topic or theme (`ID`, `Name`, `Score`, `Source`).

## ConceptExtractorPass
The compiler-pass stage running `ConceptExtractor` strategies to synthesize headings and key phrases into abstract topics.

## RelationSource
Typed provenance enum (`"rule_based"`, `"llm"`, `"hybrid"`).

## RelationType
Typed predicate classification for directed Knowledge Graph edges (`"founded_by"`, `"located_in"`, `"part_of"`, `"relates_to"`, `"author_of"`, `"associated_with"`).

## Relation
Minimal directed Subject-Predicate-Object domain record (`ID`, `SubjectID`, `Predicate`, `ObjectID`, `Confidence`, `ChunkID`, `Source`).

## RelationExtractorPass
The compiler-pass stage running `RelationExtractor` strategies to extract directed edges between entities and concepts.

## SummarySource
Typed provenance enum (`"rule_based"`, `"llm"`, `"hybrid"`).

## Summary
Minimal domain record encapsulating text and provenance (`Text`, `Source`).

## SummaryResult
Container holding document-level summary and map of chunk-level summaries.

## SummaryPass
The compiler-pass stage running `SummaryExtractor` strategies to attach executive and section summaries to documents and chunks.










## Gold Set
The versioned, human-curated, chunk-level evaluation dataset for retrieval benchmarks. Queries are built exclusively from the real indexed corpus — no synthetic chunks, no injected facts, no LLM-generated documents — and each query declares its expectations explicitly (`expected_chunk_ids`, `expected_sections`, `expected_no_evidence`).
_Avoid_: Test corpus, benchmark dataset, eval fixture

## Abstention Query
A Gold Set query expected to match zero relevant chunks, declaring `expected_no_evidence: true`. It measures whether the pipeline correctly abstains (no LLM call, `no_evidence` answer) instead of forcing an answer from nearest neighbors.

## Corpus Fingerprint
A deterministic digest of the indexed corpus used to prove benchmark validity: SHA-256 over the sorted `ContentHash` values of the indexed chunks of the gold document(s). The Gold Set declares the expected fingerprint and the benchmark runner hard-fails on mismatch before evaluating any query.

## SparseEncoder
The indexing-stage seam, symmetric to the dense embedding provider, that converts a `KnowledgeChunk` into a sparse vector representation (BM25 term weights in M3; SPLADE/learned sparse later). Sparse vectors share the same Qdrant collection and lifecycle as dense vectors — retrieval state is never process-local.

## IntentHint
A minimal signal emitted by query understanding carrying only `Intent`, `Decompose`, and `Source` (initially `"rule_based"`, derived from the existing deterministic analyzer signal). Proven intents: `comparison` (from `SubQueries`), `entity` (from the analyzer's entity question classification, M7), and `document_overview` (document-level questions — M9, document profile retrieval). A hint never decides retrieval behavior — the Retrieval Orchestrator holds policy and may ignore, partially use, or combine hints with other signals. No confidence score is attached until a benchmarked classifier justifies probabilistic semantics.
_Avoid_: Intent classification (the label alone is not a decision), intent score

## Retrieval Orchestrator
The pure decision function in `qa` translating `IntentHint + RuntimeConfig` into a `RetrievalDecision` (`Decompose`, optional `TopKOverride`). It is the only place where hints translate into behavior; the classifier stays infrastructure. Benchmark results decide whether classification exists at all. Runtime fusion-policy selection is currently config-frozen (`Balanced`); a `Policy` decision field appears only when a production benchmark demonstrates a per-intent gain beyond the 5% tolerance.

## RetrievalDecision
The output of the Retrieval Orchestrator, kept minimal and benchmark-backed: `Decompose bool`, an optional `TopKOverride` (evidence budget), `UseGraph` — the M7 graph gate opened only for entity queries when the calibrated graph weight is positive (ADR-0042), and `DocumentOverview` — the M9 path selecting the document-level retrieval component for `document_overview` intent (ADR-0048). No `Policy` field until a second benchmark-validated runtime policy path exists. `AnswerEngine` executes the decision; retrievers remain execution components.

## Document Profile Point
The document-level retrieval artifact stored in the vector store — one per indexed document — carrying the deterministic profile content (title, author, extractive summary, key topics, section list) derived from enrichment outputs at index time (ADR-0048). It is tagged `content_type=document_profile`, participates in the indexing diff lifecycle like any point, and is excluded from corpus fingerprint computation (the fingerprint covers indexed *chunks*, and a profile is not a chunk). It is the retrieval target of the `document_overview` intent.
_Avoid_: document summary chunk, overview chunk, metadata point

## Document Overview Intent
The third benchmark-proven query intent (after `comparison` and `entity`): queries asking for document-level understanding — "what is this book about", "bu kitap ne anlatıyor", "who is the author", "ana fikri nedir" — detected deterministically by the rule-based analyzer with a capped EN+TR pattern set (ADR-0048). It routes to the Document Overview Retriever; queries the patterns miss fall back to normal retrieval (a UX miss, never a correctness failure — the EvidenceGate still abstains honestly).
_Avoid_: document intent, overview intent (vague)

## Document Overview Retriever
The retrieval execution component for `document_overview` intent, implementing the existing `Retriever` seam (ADR-0048). With a document filter: the document's profile point plus the first real content chunks (deterministic `chunk_order > 1` selection, known front-matter noise excluded at retrieval time). Without a filter: all profile points (library-level overview). It never runs graph, decomposition, or hybrid paths; it hands the assembled context to the unchanged ContextBuilder → EvidenceGate → LLM flow.
_Avoid_: overview pipeline, second retrieval pipeline

## Expected Retrieval Points
The gold set query expectation field generalizing `expected_chunk_ids` (ADR-0027, ADR-0048): the set of retrieval artifacts a query must surface — knowledge chunks, document profile points, or both. It exists because a `document_profile` point is not a chunk, and document-overview benchmarks measure document-level retrieval behavior, not chunk retrieval. At most one of `expected_chunk_ids` / `expected_retrieval_points` is declared per query.
_Avoid_: expected_context_points (implies an assembled window)

## EvidenceBudget
The maximum number of retrieved chunks admitted to evidence assembly, expressed in M6 as a `TopKOverride` on the RetrievalDecision — never a token-budget or gate change. The concrete value is calibrated per intent by benchmark (e.g. comparison false-abstention reduction on Gold Set v2) before it freezes; the default is the caller's TopK.

## RerankedRetriever
The M8 execution component (ADR-0044) wrapping any `Retriever` with a second-stage rerank: it requests an internal candidate budget N from the inner retriever, reranks, and returns the caller's TopK. Reranker failures degrade gracefully to the inner ordering (fail-open); `StabilizeOrdering` is the single enforcement point of the ordering contract (deterministic ChunkID-ASC tie-break). Production activation (M10) wraps the graph fusion retriever behind the M7 entity gate; the global rerank remains benchmark-rejected.
_Avoid_: rerank wrapper, reranker retriever

## Reranker Service
The provider-agnostic reranking microservice (M10, ADR-0049) — a Python HTTP service (`reranker-service`) exposing a minimal `POST /rerank` contract (`{query, candidates:[{id,text}]}` → `{ranked_ids, scores}`), with the BGE cross-encoder as its first model implementation. Model choice lives entirely in the service; the retrieval engine only knows the HTTP contract. Runs beside the other services (Docker, GPU), model loaded once per service lifetime.
_Avoid_: bge-service, bge-reranker-service, rerank model server

## HTTP Reranker
The production `Reranker` seam adapter (M10, ADR-0049): an HTTP client depending only on the `POST /rerank` contract, never on the Python implementation. It carries the minimal runtime observability (atomic counters `reranker_requests_total`, `reranker_failures_total`, `reranker_latency_ms_total` + a structured debug log line per call: candidate_count, reranked_count, latency_ms, degraded). Fail-open: timeout/connection/5xx/invalid response fall back to the inner retriever ordering with `degraded=true`.
_Avoid_: reranker client, rerank provider adapter

## FusionPolicy
A frozen, calibrated retrieval fusion configuration (`DenseWeight`, `SparseWeight`, `SparseCap`) produced by offline benchmark calibration — not raw tuned parameters. Named policies (e.g. `Balanced`, `DenseBiased`, `LexicalBiased`) are the only fusion variants an orchestrator may select; numerical optimization ends at calibration (M4). Runtime selection stays config-frozen (`Balanced`) until a production benchmark demonstrates a per-intent gain beyond the 5% tolerance (M6).
