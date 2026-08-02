# 07 — Knowledge Graph Bounded Context & GraphRetriever

**What to build:**
The `internal/graph` Bounded Context featuring graph domain models (`Node`, `Edge`, `NodeType`, `RelationType`), `GraphStore` interface seam, `InMemoryGraphStore` adapter, `EntityExtractor` seam, and `GraphRetriever` adapter implementing the `Retriever` interface seam for 3-way RRF score fusion (GraphRAG).

**Blocked by:** 06 — KnowledgeSpace Boundary, Existing Retrieval modules.

**Status:** ready-for-agent

- [ ] Define `Node`, `Edge`, `NodeType`, and `RelationType` domain models.
- [ ] Define `GraphStore` interface seam (`AddNode`, `AddEdge`, `Traverse`) and implement `InMemoryGraphStore`.
- [ ] Define `EntityExtractor` interface seam and `RuleBasedEntityExtractor` adapter.
- [ ] Implement `GraphRetriever` adhering to the `Retriever` interface seam.
- [ ] Update `HybridRetriever` to support 3-way RRF fusion (Dense + Sparse + Graph).
- [ ] Add unit tests verifying multi-hop graph traversal, entity extraction, and 3-way RRF score fusion.
