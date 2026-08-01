# 05 — Document Asset and Citation Extraction

**What to build:** Modules for identifying and extracting non-prose document assets (tables, figures, code blocks, equations) alongside citation preservation, populating `PDFInspectionResult.assets` with page locations and structural references.

**Blocked by:** 03 — Semantic Document Reconstruction Engine (ProcessExtraction)

**Status:** completed

- [x] Extracts tables, figures, code blocks, and math equations into structured asset records.
- [x] Preserves footnotes, inline references, and bibliography citations with page context.
- [x] Attaches extracted assets to their corresponding `section_path` and `page_numbers`.
- [x] Unit tests verify asset and citation extraction across markdown fixtures.
