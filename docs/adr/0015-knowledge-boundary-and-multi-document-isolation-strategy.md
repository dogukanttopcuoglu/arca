# 0015: Knowledge Boundary & Multi-Document Isolation Strategy

- **Status:** Accepted
- **Date:** 2026-08-02
- **Deciders:** Staff Software Architect & Lead Engineer

## Context and Problem Statement

To support multi-document analysis, workspace grouping, and enterprise multi-tenancy, ARC requires a clear domain hierarchy and data isolation strategy across vector stores and indexing pipelines.

## Decision Drivers

- **Domain Model Alignment**: Users organize knowledge into libraries/projects, not isolated single PDFs.
- **Backward Compatibility**: Single-document queries must continue working without breaking API changes.
- **Security & Multi-tenancy**: Security filtering (`SecurityContext`) must be isolated from user-supplied query filters to prevent tenant data leaks.

## Decided Options

### Option A: Canonical `KnowledgeSpace` Domain Model (ACCEPTED)
- 3-Tier Domain Hierarchy: `Workspace / Tenant` -> `KnowledgeSpace` -> `Document` -> `KnowledgeChunk`.
- `MetadataFilter` expansion: `WorkspaceID`, `KnowledgeSpaceID`, `DocumentIDs`, `ChunkIDs`.
- **Logical Isolation**: A single unified vector store collection indexed by metadata fields, rather than creating thousands of physical collections per user.
- **Security Isolation**: `SecurityContext` (`TenantID`, `UserID`, `Permissions`) is enforced at the `Retriever` level independently of user-supplied `MetadataFilter`.

## Consequences

### Positive
- Enables cross-document RAG across entire libraries or projects.
- Backward-compatible with existing single-document indexing and search code.
- High operational efficiency without vector store collection bloat.
