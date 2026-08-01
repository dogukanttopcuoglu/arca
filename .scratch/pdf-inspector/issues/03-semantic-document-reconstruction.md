# 03 — Semantic Document Reconstruction Engine (ProcessExtraction)

**What to build:** The `ProcessExtraction(ctx, extractionResult)` semantic processing engine that reconstructs document heading hierarchy, paragraph layout, page mappings, and logical `SemanticTree` structure from raw Firecrawl extraction JSON/Markdown.

**Blocked by:** 02 — Canonical Knowledge Document (PDFInspectionResult)

**Status:** ready-for-agent

- [ ] Reconstructs document heading hierarchy (`#`, `##`, `###`) into a clean tree structure.
- [ ] Maintains reading order, paragraph boundaries, and page references across the document.
- [ ] Accepts raw extraction payloads via the `ProcessExtraction` test seam without requiring real PDF files.
- [ ] Emits structured diagnostic warnings for unmapped or ambiguous layout nodes.
