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

