# 02 — Canonical Knowledge Document (PDFInspectionResult)

**What to build:** The core versioned Go data structures, JSON schemas, serialization/deserialization logic, and initial diagnostic metric structures for `PDFInspectionResult`, `DocumentMetadata`, `SemanticTree`, `KnowledgeChunk`, `DocumentAsset`, and `Diagnostics`.

**Blocked by:** 00 — Project Skeleton & Development Environment

**Status:** ready-for-agent

- [ ] All Go struct definitions for `PDFInspectionResult` match the agreed specification.
- [ ] JSON serialization and deserialization are deterministic and version-stamped.
- [ ] Initial `Diagnostics` fields (`status`, `warnings`, `errors`, `processingTimeMs`) are supported across all data models.
- [ ] Unit tests verify round-trip serialization and schema compliance.
