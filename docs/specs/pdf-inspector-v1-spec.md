# Specification: ARC PDF Inspector V1

## Problem Statement

Users and downstream knowledge processing engines within ARC need to transform raw, unstructured PDF documents into clean, structured, and searchable knowledge artifacts. Standard PDF metadata viewers or naive text splitters break semantic boundaries (tables, code snippets, section hierarchies), lose document provenance, fail to preserve parent-child context, and do not provide structured intermediate representations for vector embedding generation and Knowledge Graph construction.

## Solution

ARC PDF Inspector V1 serves as the dedicated entry-point stage of the knowledge ingestion pipeline. It leverages an isolated, Dockerized Firecrawl microservice over HTTP for reliable document parsing and Markdown conversion, while the core Go backend performs semantic section analysis, hierarchical semantic chunking (with parent-child links), asset extraction, and citation preservation. It emits a deterministic, serializable, and versioned `PDFInspectionResult` schema accompanied by detailed diagnostic metrics and graceful degradation resiliency.

## User Stories

1. As a Knowledge Engineer, I want PDF Inspector to extract document metadata (title, author, page count, fonts, creation date), so that I can maintain full administrative provenance for ingested documents.
2. As a RAG Pipeline Developer, I want PDF Inspector to convert PDFs into structured Markdown with page maps, so that downstream models receive LLM-friendly textual representations.
3. As a Search Engineer, I want PDF Inspector to identify searchable versus scanned PDFs and report OCR requirements, so that unreadable pages are flagged immediately.
4. As a Knowledge Graph Architect, I want PDF Inspector to construct a hierarchical `SemanticTree` of headings and sections, so that graph builder services can construct parent-child domain relationships.
5. As a RAG Developer, I want PDF Inspector to perform Hierarchical Semantic Chunking, so that section boundaries, tables, code blocks, lists, and math equations are never split arbitrarily across chunk limits.
6. As an AI Agent, I want each generated `KnowledgeChunk` to maintain explicit `parent_chunk_id`, `child_chunk_ids`, `section_path`, and `page_numbers`, so that I can perform parent document retrieval and contextual expansion.
7. As a Retrieval Specialist, I want PDF Inspector to target a preferred chunk size range of 400–700 tokens (with a soft limit of 1000 and absolute max of 1200 tokens), so that chunk sizes are optimized for embedding model context windows.
8. As a Researcher, I want PDF Inspector to extract non-prose document assets (tables, figures, code blocks, equations, citations) with structural references, so that complex data structures can be queried independently.
9. As a System Operator, I want PDF Inspector to handle encrypted or unreadable PDFs with explicit fail-fast errors (`ENCRYPTED_DOCUMENT`, `INVALID_DOCUMENT`), so that pipeline workers do not waste compute resources.
10. As a Reliability Engineer, I want PDF Inspector to degrade gracefully on localized page/OCR extraction failures, so that partial inspection results with `diagnostics` status (`partial_success`) are returned rather than dropping the entire document.
11. As a DevOps Engineer, I want the Firecrawl extraction service isolated as an HTTP microservice in Docker, so that the Go backend remains completely decoupled from Node.js dependencies and can scale independently.
12. As a Pipeline Developer, I want the `PDFInspectionResult` output schema to be independent of vector database or graph database implementations, so that the same document inspection artifact can be reused across multiple storage engines.

## Implementation Decisions

### Modules & Architecture
- **Firecrawl PDF Service (Node.js/TypeScript Microservice)**: Isolated HTTP microservice exposing endpoints for PDF parsing, OCR detection, layout reconstruction, and Markdown conversion.
- **Firecrawl HTTP Client (Go Module)**: Client library in Go managing HTTP communication, request serialization, configurable timeouts, exponential backoff retries, and error mapping to `SERVICE_UNAVAILABLE`.
- **Semantic Processor (Go Module)**: Component consuming raw extraction output from Firecrawl and generating `SemanticTree`, extracting assets (tables, figures, code blocks, equations, citations), and constructing hierarchical chunks.
- **PDF Inspector Core Service (Go Package)**: Orchestrator executing the complete pipeline (`InspectPDF`), coordinating HTTP extraction and semantic processing, and returning `PDFInspectionResult`.

### Data Models & Schemas

#### PDFInspectionResult
```typescript
interface PDFInspectionResult {
  document: DocumentMetadata;
  semanticTree: SemanticTree;
  content: {
    markdown: string;
    pageMap: PageMap[];
  };
  chunks: KnowledgeChunk[];
  assets: {
    tables: Table[];
    figures: Figure[];
    codeBlocks: CodeBlock[];
    equations: Equation[];
    citations: Citation[];
  };
  diagnostics: Diagnostics;
}
```

#### KnowledgeChunk
```typescript
interface KnowledgeChunk {
  chunk_id: string;
  parent_chunk_id: string | null;
  child_chunk_ids: string[];
  document_id: string;
  section_path: string;
  heading_level: number;
  page_numbers: number[];
  content_markdown: string;
  token_estimate: number;
  character_count: number;
  citations: Citation[];
  source_offsets: SourceOffset;
  content_type: 'paragraph' | 'table' | 'code' | 'list' | 'equation' | 'figure';
}
```

#### Diagnostics
```typescript
interface Diagnostics {
  status: 'success' | 'partial_success' | 'failed';
  extractionEngine: string;
  extractionVersion: string;
  processingTimeMs: number;
  warnings: string[];
  errors: string[];
  skippedPages: number[];
  retryCount: number;
}
```

## Testing Decisions

### Good Test Principles
- Test external behavior and data contracts rather than internal private helpers.
- Rely on deterministic fixtures for extracted Markdown and intermediate payloads.
- Use explicit test seams for network isolation and fast feedback cycles.

### Seams & Modules Tested
1. **Firecrawl HTTP API Seam**: Tested using Go `httptest.Server` to verify request construction, timeout, retry with backoff, and response mapping.
2. **Semantic Processing Seam (`ProcessExtraction`)**: Tested by passing pre-canned JSON/Markdown extraction outputs to evaluate hierarchy generation, chunk token bounds (400–700 tokens), asset extraction, and parent-child linking without PDF parsing overhead.
3. **PDF Inspector Core Service Seam (`InspectPDF`)**: Tested with sample PDF streams (valid multi-page PDF, encrypted PDF, corrupted PDF, PDF with broken OCR pages) to verify end-to-end execution, schema compliance, and diagnostic reporting.

## Out of Scope

- Malware, virus, or security vulnerability scanning.
- JavaScript, launch action, or executable payload inspection.
- Prepress / print production quality checks (CMYK, DPI, color profiles).
- Direct vector database embedding generation or indexing.
- Direct Knowledge Graph node or edge database persistence.

## Further Notes

- `PDFInspectionResult` represents the canonical intermediate representation for document ingestion across all ARC microservices.
- Future upgrades to the extraction engine can be executed seamlessly behind the Firecrawl HTTP API seam without changing downstream ARC Go services.
