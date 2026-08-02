# 06 — KnowledgeSpace & Multi-Document Isolation Strategy

**What to build:**
The 3-tier domain hierarchy (`Workspace / Tenant` -> `KnowledgeSpace` -> `Document` -> `Chunk`), expanded `MetadataFilter` (`WorkspaceID`, `KnowledgeSpaceID`), logical vector store isolation, and `SecurityContext` enforcement in `internal/security`.

**Blocked by:** Existing Indexing & Retrieval modules.

**Status:** ready-for-agent

- [ ] Define `KnowledgeSpace` and `Workspace` domain models.
- [ ] Update `MetadataFilter` in `internal/indexing/model` with `WorkspaceID` and `KnowledgeSpaceID` fields.
- [ ] Define `SecurityContext` (`TenantID`, `UserID`, `Permissions`) in `internal/security`.
- [ ] Update `Retriever` implementations to enforce `SecurityContext` logical boundaries independently of user-supplied filters.
- [ ] Add unit tests verifying cross-space isolation and security context enforcement.
