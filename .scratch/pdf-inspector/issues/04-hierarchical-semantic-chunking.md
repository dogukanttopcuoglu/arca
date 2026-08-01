# 04 — Hierarchical Semantic Chunking Engine

**What to build:** An isolated chunking engine that operates on `SemanticTree` outputs to produce `KnowledgeChunk` records, enforcing section-aware boundaries, targeting 400–700 tokens, maintaining `parent_chunk_id` and `child_chunk_ids` links, and preserving breadcrumb section paths.

**Blocked by:** 03 — Semantic Document Reconstruction Engine (ProcessExtraction)

**Status:** ready-for-agent

- [ ] Generates `KnowledgeChunk` lists targeting 400–700 tokens (soft max 1000, absolute max 1200).
- [ ] Never splits sections, paragraphs, tables, code blocks, or math expressions across chunk boundaries.
- [ ] Correctly populates `parent_chunk_id`, `child_chunk_ids`, `section_path`, `heading_level`, and `content_type`.
- [ ] Unit tests verify semantic integrity and chunk hierarchy against sample document trees.
