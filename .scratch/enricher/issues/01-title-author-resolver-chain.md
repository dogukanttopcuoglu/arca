# 01 — Title & Author Resolver Fallback Chains

**What to build:**
Implement `TitleResolver` and `AuthorResolver` fallback chains in `internal/pdfinspector/enrichment/resolver.go`, eradicating hardcoded `"The Creative Act"` and `"Rick Rubin"` fallback strings.

**Blocked by:** None.

**Status:** ready-for-agent

- [ ] Define `TitleResolver` and `AuthorResolver` interface seams.
- [ ] Implement `PDFMetadataTitleResolver`, `HeadingTitleResolver`, `TOCPageTitleResolver`, and `UnknownTitleResolver`.
- [ ] Implement `PDFMetadataAuthorResolver`, `EarlyPageAuthorResolver`, and `UnknownAuthorResolver`.
- [ ] Add unit tests verifying fallback chain ordering and non-generic title/author derivation.
