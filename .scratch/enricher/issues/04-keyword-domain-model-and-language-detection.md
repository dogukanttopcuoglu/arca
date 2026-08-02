# 04 — Keyword Domain Model & LanguageDetectionPass

**What to build:**
Add `Keyword` and `KeywordSource` domain models to `internal/pdfinspector/model/inspection.go`, add `Language` field to `DocumentMetadata`, and implement `LanguageDetectionPass` in `internal/pdfinspector/enrichment/language.go`.

**Blocked by:** None.

**Status:** ready-for-agent

- [ ] Add `Keyword` struct (`Value`, `Score`, `Source`, `ChunkIDs`) and `KeywordSource` enum to `model/inspection.go`.
- [ ] Add `Language` string field to `DocumentMetadata` and `KnowledgeChunk`.
- [ ] Implement `LanguageDetectionPass` detecting `"tr"` and `"en"` language codes.
- [ ] Add unit tests verifying language detection for Turkish and English document snippets.
