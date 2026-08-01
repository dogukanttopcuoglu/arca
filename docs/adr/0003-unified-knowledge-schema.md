# 3. Output Data Contract as Pipeline-Ready PDFInspectionResult Schema

* Status: Accepted
* Date: 2026-08-01

## Context and Problem Statement

ARC PDF Inspector transforms raw PDF documents into structured knowledge artifacts for downstream storage, graph indexing, vector embedding, and retrieval. We need to define a stable, decoupled intermediate data representation contract.

## Decision Drivers

* Strict separation of concerns between document inspection and graph/vector indexing.
* Deterministic, serializable, versioned, and LLM-friendly schema design.
* Reusability of the inspection output across different downstream graph models or retrieval engines.

## Decision Outcome

Chosen Option: Expose a `PDFInspectionResult` struct containing `document`, `semanticTree`, `content` (Markdown & PageMap), `chunks`, `assets` (tables, figures, codeBlocks, equations, citations), and `diagnostics`.

Graph node/edge generation is explicitly deferred to downstream services consuming `semanticTree` and `chunks`.

### Positive Consequences

* Keeps PDF Inspector agnostic of graph databases or vector database schemas.
* Provides complete extraction of text, structure, assets, and diagnostic metrics in a single payload.
