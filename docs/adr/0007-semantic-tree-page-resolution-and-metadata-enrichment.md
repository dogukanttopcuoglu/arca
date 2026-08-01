# ADR 0007: Semantic Tree Page Resolution and Metadata Enrichment

- Status: Accepted
- Date: 2026-08-01
- Decision Makers: ARC Engineering Team

## Context

The ARC PDF Inspector pipeline successfully extracts:

- Document metadata
- Semantic tree structure
- Hierarchical knowledge chunks
- Assets
- Citations
- Diagnostics

During validation with a 248-page document (`The Creative Act - Rick Rubin`), the generated `inspection_result.json` revealed metadata enrichment issues.

Current behavior:

- `KnowledgeChunk.page_numbers` values are accurate.
- `DocumentContent.pageMap` preserves correct page-level provenance.
- `SemanticTree.RootNodes[].PageNumbers` incorrectly defaults to `[1]`.
- Document title falls back to `"Extracted PDF Document"` when PDF metadata does not contain a title.

Example:

```json
{
  "heading": "Beginner’s Mind",
  "level": 3,
  "pageNumbers": [1]
}
```

Existing chunk output:

```json
{
  "section_path": "Beginner’s Mind",
  "page_numbers": [71]
}
```

The required provenance information already exists in the pipeline but is not propagated into the semantic representation.

## Decision

Introduce a Semantic Metadata Enrichment Layer after semantic extraction.

The enrichment layer will improve incomplete semantic metadata by resolving:

- Semantic node page numbers
- Document title fallback
- Metadata completeness

The enrichment process must be non-destructive.

The pipeline must continue successfully even if enrichment cannot resolve some metadata.

### Semantic Tree Page Resolution

For each semantic node:

1. Search available document page content.
2. Find the first page where the heading appears.
3. Assign the discovered page number to `SemanticNode.PageNumbers`.

Example:

```markdown
### Beginner’s Mind

Some three thousand years ago in China...
```

Semantic node:

```json
{
  "heading": "Beginner’s Mind"
}
```

Enriched result:

```json
{
  "heading": "Beginner’s Mind",
  "pageNumbers": [71]
}
```

#### Heading Matching Rules

Heading comparison must support normalized matching.

Normalization rules:

- Convert text to lowercase.
- Normalize unicode characters.
- Remove apostrophe variations.
- Normalize whitespace.
- Ignore non-semantic punctuation.

Examples that should resolve as the same heading:

- `Beginner’s Mind`
- `Beginner's Mind`
- `beginners mind`

#### Provenance Fallback Strategy

If direct heading resolution fails, the system must fallback to existing knowledge chunk provenance.

Resolution flow:

`SemanticNode.heading` → `Chunk.section_path` → `Chunk.page_numbers` → `SemanticNode.PageNumbers`

Knowledge chunks already contain validated page provenance and represent a reliable fallback source.

### Document Title Enrichment

When PDF metadata does not contain a valid title, ARC will derive the document title using the following priority order:

1. PDF Metadata Title
2. First meaningful H1 heading
3. First meaningful H2 heading
4. Source filename
5. `"Untitled Document"`

Generic headings must not be selected:

- `Chapter 1`
- `Introduction`
- `Contents`
- `Table of Contents`

Example:

Before:

```json
{
  "title": "Extracted PDF Document"
}
```

After:

```json
{
  "title": "The Creative Act"
}
```

### Non-Destructive Enrichment

Semantic enrichment must never become a pipeline failure point.

If enrichment cannot resolve metadata:

- Existing semantic nodes remain valid.
- Document inspection continues.
- Diagnostics record a warning.

Example:

```json
{
  "status": "partial_success",
  "warnings": [
    "semantic page resolution unavailable"
  ]
}
```

## Consequences

### Positive
- Semantic nodes gain accurate page provenance.
- Canvas UI can navigate from semantic nodes directly to PDF pages.
- Knowledge graph generation becomes more reliable.
- Document metadata becomes more meaningful.
- AI agents receive richer document context.

### Negative
- Semantic processing requires an additional enrichment phase.
- Heading matching introduces additional complexity.
- Poorly structured PDFs may require fallback behavior.

## Implementation Scope

Affected components:

- `internal/pdfinspector/enrichment`
  - semantic page resolver
  - heading normalization
  - metadata enrichment
- `internal/pdfinspector/inspector`
  - document title fallback integration
- `internal/pdfinspector/model`
  - metadata validation updates

## Verification Criteria

Implementation is complete when:

- [x] `SemanticTree` nodes contain accurate page numbers.
- [x] Existing chunk page mappings remain unchanged.
- [x] Document title fallback works without PDF metadata.
- [x] Heading normalization handles extraction variations.
- [x] Generic headings are ignored during title resolution.
- [x] Enrichment failures produce diagnostics warnings instead of pipeline failures.
- [x] Existing test suites continue passing.

## Final Decision

Approved.

ARC will implement Semantic Metadata Enrichment as a dedicated layer while preserving existing extraction accuracy, provenance guarantees, and resiliency behavior.
