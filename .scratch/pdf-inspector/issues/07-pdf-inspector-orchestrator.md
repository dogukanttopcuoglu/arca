# 07 — PDF Inspector Orchestrator (InspectPDF) & Core Service Seam

**What to build:** The top-level `InspectPDF(ctx, pdfReader) (*PDFInspectionResult, error)` orchestrator connecting the Firecrawl HTTP client, Semantic Processor, Chunking Engine, Asset Extractor, and Diagnostics Aggregator into a seamless end-to-end pipeline.

**Blocked by:** 01 — Firecrawl HTTP Service Integration Client & Test Seam, 06 — Resiliency Policy & Diagnostics Aggregation

**Status:** completed

- [x] `InspectPDF` provides the top-level API entry point for PDF processing in ARC.
- [x] Orchestrates HTTP extraction, semantic reconstruction, chunking, asset extraction, and diagnostics.
- [x] End-to-end integration tests confirm valid `PDFInspectionResult` outputs across real/fixture PDF inputs.
- [x] Verifies complete pipeline performance, memory safety, and context cancellation support.
