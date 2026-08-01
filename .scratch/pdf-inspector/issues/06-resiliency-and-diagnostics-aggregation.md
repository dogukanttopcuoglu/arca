# 06 — Resiliency Policy & Diagnostics Aggregation

**What to build:** Resiliency policies, error mapping, fail-fast checks for encrypted or invalid PDFs, page-level partial recovery, and diagnostic aggregation across the inspection pipeline.

**Blocked by:** 04 — Hierarchical Semantic Chunking Engine, 05 — Document Asset and Citation Extraction

**Status:** ready-for-agent

- [ ] Returns `ENCRYPTED_DOCUMENT` or `INVALID_DOCUMENT` errors for unreadable PDFs.
- [ ] Degrades gracefully on localized OCR/asset failures, producing `diagnostics.status = "partial_success"`.
- [ ] Aggregates skipped pages, retry counts, warnings, and processing durations into `PDFInspectionResult.diagnostics`.
- [ ] Integration tests verify pipeline resiliency under simulated failure conditions.
