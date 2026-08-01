# 4. Adopt Hierarchical Semantic Chunking Strategy

* Status: Accepted
* Date: 2026-08-01

## Context and Problem Statement

Downstream RAG, vector retrieval, and GraphRAG pipelines require chunks that retain high semantic coherence, parent-child context, and strict document provenance without breaking semantic units like tables, code blocks, math expressions, or lists.

## Decision Drivers

* Semantic integrity over arbitrary token cutoffs.
* Support for advanced retrieval patterns (Parent Document Retrieval, Context Expansion, GraphRAG).
* Detailed provenance and context preservation (section path, page numbers, citations, parent/child IDs).

## Decision Outcome

Chosen Option: Hierarchical Semantic Chunking.

* Target chunk size: Preferred 400–700 tokens, soft max 1000 tokens, absolute max 1200 tokens.
* Never break semantic boundaries (sections, paragraphs, lists, tables, code blocks, equations, figures).
* Maintain explicit `parent_chunk_id` and `child_chunk_ids` links alongside `section_path` and `content_type`.

### Positive Consequences

* Enables parent document retrieval and contextual expansion without re-parsing.
* Prevents truncated code blocks, orphaned table rows, or split equations.
* Fully compatible with GraphRAG and hybrid search pipelines.
