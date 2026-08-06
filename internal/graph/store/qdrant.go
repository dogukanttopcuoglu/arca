package store

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	graphmodel "arca/internal/graph/model"
	"github.com/valyala/fasthttp"
)

// DefaultGraphCollection is the Qdrant collection holding entity nodes
// (ADR-0038: node-only, one collection; chunk evidence lives in the chunk
// collection payload).
const (
	DefaultGraphCollection = "arca_graph_nodes"
	graphScrollBatchSize   = 100
)

// QdrantGraphStore implements GraphStore against a Qdrant cluster over its
// REST API. It is node-only in v1 (ADR-0038): entity nodes are stored as
// vectorless points keyed by a deterministic UUID derived from the node ID
// (graphmodel.CalculatePointID); the payload carries the node ID, canonical
// name, entity type, score, and chunk evidence. Edge methods are not
// supported until a future iteration adds relation persistence.
//
// REST is used (not the gRPC client) because Qdrant's gRPC upsert rejects
// vectorless points with "Expected some vectors", while the REST API accepts
// them — verified against Qdrant 1.18.
type QdrantGraphStore struct {
	baseURL    string
	collection string
	client     *fasthttp.Client
}

// NewQdrantGraphStore constructs a QdrantGraphStore. baseURL must include the
// scheme and host, optionally with the /v1 API prefix (e.g.
// "http://localhost:6333" or "http://localhost:6333/v1").
func NewQdrantGraphStore(baseURL, collection string) (*QdrantGraphStore, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("qdrant base URL cannot be empty")
	}
	if collection == "" {
		collection = DefaultGraphCollection
	}
	return &QdrantGraphStore{
		baseURL:    baseURL,
		collection: collection,
		client:     &fasthttp.Client{},
	}, nil
}

// Close releases resources held by the store.
func (s *QdrantGraphStore) Close() error { return nil }

// AddNode appends chunk evidence to an entity node atomically (M7 audit A1):
// every incoming chunk becomes its own payload field via set_payload, so
// concurrent writers never overwrite each other's evidence (the previous
// read-modify-write lost updates 19/20 under concurrency). The score is
// recorded per document; reads take the max, so it never regresses. Node
// identity and the Node model are unchanged (ADR-0038; persistence schema
// errata 2026-08-05).
func (s *QdrantGraphStore) AddNode(ctx context.Context, node graphmodel.Node) error {
	if node.ID == "" {
		return fmt.Errorf("node ID cannot be empty")
	}
	if err := s.ensureCollection(ctx); err != nil {
		return err
	}
	if err := s.ensurePoint(ctx, node); err != nil {
		return err
	}

	payload := map[string]any{
		"name":           strings.ToLower(strings.TrimSpace(node.Name())),
		"canonical_name": canonicalNameOf(node),
		"entity_type":    entityTypeOf(node),
	}
	scoredDocs := make(map[string]bool)
	for _, cid := range node.ChunkIDs() {
		payload[chunkField(cid)] = true
		doc := docFromChunkID(cid)
		// Defensive: chunks from multiple documents each stamp their own
		// per-document score field; readers take the max.
		if !scoredDocs[doc] {
			scoredDocs[doc] = true
			payload[scoreField(doc)] = node.Score()
		}
	}
	return s.setPayload(ctx, node.ID, payload)
}

// ensurePoint creates the node point if missing, with only static fields.
// Concurrent creators write identical content, so the last-write-wins
// outcome is harmless; variable evidence is appended atomically afterwards
// via set_payload (M7 audit A1). Only the not-found (HTTP 404) outcome
// triggers creation: transient errors abort instead of replacing an existing
// point's evidence (M7 audit A2).
func (s *QdrantGraphStore) ensurePoint(ctx context.Context, node graphmodel.Node) error {
	_, err := s.GetNode(ctx, node.ID)
	if err == nil {
		return nil
	}
	if !strings.Contains(err.Error(), "status 404") {
		return fmt.Errorf("graph node existence check failed: %w", err)
	}
	var out struct {
		Result struct {
			Status string `json:"status"`
		} `json:"result"`
	}
	err = s.doJSON(ctx, http.MethodPut, "/collections/"+s.collection+"/points", map[string]any{
		"points": []map[string]any{{
			"id": graphmodel.CalculatePointID(node.ID),
			"payload": map[string]any{
				"id":        node.ID,
				"node_type": string(node.Type),
			},
			// Empty object: Qdrant requires an explicit (empty) vector field
			// on vectorless collections (verified on Qdrant 1.18).
			"vector": map[string]any{},
		}},
	}, &out)
	if err != nil {
		return fmt.Errorf("graph node creation failed: %w", err)
	}
	return nil
}

// Payload field prefixes for the atomic field-per-chunk schema. The field
// codec is the single place that owns the protocol: writers and readers must
// use chunkField/scoreField rather than string literals.
const (
	chunkFieldPrefix = "chunk_"
	scoreFieldPrefix = "score_"
)

// chunkField maps a chunk ID to its payload evidence field. The ID is
// hex-encoded because Qdrant payload keys are JSON paths: "/" and "." in raw
// chunk IDs break set_payload/clear_payload (verified on Qdrant 1.18).
func chunkField(chunkID string) string {
	return chunkFieldPrefix + hex.EncodeToString([]byte(chunkID))
}

// scoreField maps a document ID to its per-document score field (hex-encoded
// for the same JSON-path reason).
func scoreField(documentID string) string {
	return scoreFieldPrefix + hex.EncodeToString([]byte(documentID))
}

// docFromChunkID derives the document ID from a chunk ID ("doc/section/NNN").
func docFromChunkID(chunkID string) string {
	if i := strings.Index(chunkID, "/"); i > 0 {
		return chunkID[:i]
	}
	return chunkID
}

// setPayload atomically sets payload fields on the node point; existing
// fields are preserved.
func (s *QdrantGraphStore) setPayload(ctx context.Context, nodeID string, payload map[string]any) error {
	return s.doJSON(ctx, http.MethodPost, "/collections/"+s.collection+"/points/payload", map[string]any{
		"payload": payload,
		"points":  []string{graphmodel.CalculatePointID(nodeID)},
	}, nil)
}

// clearPayload atomically removes the given payload fields from the node
// point; other fields are preserved.
func (s *QdrantGraphStore) clearPayload(ctx context.Context, nodeID string, fields []string) error {
	if len(fields) == 0 {
		return nil
	}
	return s.doJSON(ctx, http.MethodPost, "/collections/"+s.collection+"/points/payload/delete", map[string]any{
		"keys":   fields,
		"points": []string{graphmodel.CalculatePointID(nodeID)},
	}, nil)
}

// GetNode retrieves a Node by its graph node ID (deterministic point ID).
func (s *QdrantGraphStore) GetNode(ctx context.Context, id string) (*graphmodel.Node, error) {
	var out struct {
		Result *struct {
			Payload map[string]any `json:"payload"`
		} `json:"result"`
	}
	err := s.doJSON(ctx, http.MethodGet, "/collections/"+s.collection+"/points/"+graphmodel.CalculatePointID(id)+"?with_payload=true", nil, &out)
	if err != nil {
		return nil, fmt.Errorf("graph node get failed: %w", err)
	}
	if out.Result == nil || out.Result.Payload == nil {
		return nil, fmt.Errorf("node not found: %s", id)
	}
	node, err := nodeFromPayload(out.Result.Payload)
	if err != nil {
		return nil, err
	}
	if node.ID != id {
		return nil, fmt.Errorf("point id mismatch: expected %s, got %s", id, node.ID)
	}
	return node, nil
}

// FindNodeByName retrieves a Node by its normalized name (exact keyword match
// on the lowercase "name" payload property).
func (s *QdrantGraphStore) FindNodeByName(ctx context.Context, name string) (*graphmodel.Node, error) {
	key := strings.ToLower(strings.TrimSpace(name))
	var out struct {
		Result struct {
			Points []struct {
				Payload map[string]any `json:"payload"`
			} `json:"points"`
		} `json:"result"`
	}
	err := s.doJSON(ctx, http.MethodPost, "/collections/"+s.collection+"/points/scroll", map[string]any{
		"limit":        1,
		"with_payload": true,
		"filter": map[string]any{
			"must": []map[string]any{{
				"key":   "name",
				"match": map[string]any{"value": key},
			}},
		},
	}, &out)
	if err != nil {
		return nil, fmt.Errorf("graph node find failed: %w", err)
	}
	if len(out.Result.Points) == 0 {
		return nil, fmt.Errorf("node not found by name: %s", name)
	}
	return nodeFromPayload(out.Result.Points[0].Payload)
}

// DeleteByDocument removes every chunk evidence reference belonging to the
// document from all nodes via atomic field clears; nodes left without
// evidence are deleted.
func (s *QdrantGraphStore) DeleteByDocument(ctx context.Context, documentID string) error {
	nodes, err := s.scrollNodes(ctx, nil)
	if err != nil {
		return err
	}
	for i := range nodes {
		node := nodes[i]
		var clear []string
		kept := 0
		for _, cid := range node.ChunkIDs() {
			if chunkBelongsToDocument(cid, documentID) {
				clear = append(clear, chunkField(cid))
			} else {
				kept++
			}
		}
		if len(clear) > 0 {
			clear = append(clear, scoreField(documentID))
			if err := s.clearPayload(ctx, node.ID, clear); err != nil {
				return err
			}
		}
		if kept == 0 {
			if err := s.deletePoint(ctx, graphmodel.CalculatePointID(node.ID)); err != nil {
				return err
			}
		}
	}
	return nil
}

// deletePoint removes the point with the given deterministic point ID.
func (s *QdrantGraphStore) deletePoint(ctx context.Context, pointID string) error {
	return s.doJSON(ctx, http.MethodPost, "/collections/"+s.collection+"/points/delete", map[string]any{
		"points": []string{pointID},
	}, nil)
}

// ListEntityNodes returns every entity node in the collection (scrolled in
// batches). Order is unspecified; the retriever applies its own ordering.
func (s *QdrantGraphStore) ListEntityNodes(ctx context.Context) ([]graphmodel.Node, error) {
	return s.scrollNodes(ctx, map[string]any{
		"must": []map[string]any{{
			"key":   "node_type",
			"match": map[string]any{"value": string(graphmodel.NodeTypeEntity)},
		}},
	})
}

// scrollNodes pages through the collection with the given filter and returns
// every matching node.
func (s *QdrantGraphStore) scrollNodes(ctx context.Context, filter any) ([]graphmodel.Node, error) {
	var nodes []graphmodel.Node
	var next any
	for {
		req := map[string]any{
			"limit":        graphScrollBatchSize,
			"with_payload": true,
		}
		if filter != nil {
			req["filter"] = filter
		}
		if next != nil {
			req["offset"] = next
		}
		var out struct {
			Result struct {
				Points []struct {
					Payload map[string]any `json:"payload"`
				} `json:"points"`
				NextPageOffset any `json:"next_page_offset"`
			} `json:"result"`
		}
		if err := s.doJSON(ctx, http.MethodPost, "/collections/"+s.collection+"/points/scroll", req, &out); err != nil {
			return nil, fmt.Errorf("graph node scroll failed: %w", err)
		}
		for _, pt := range out.Result.Points {
			node, err := nodeFromPayload(pt.Payload)
			if err != nil {
				return nil, err
			}
			nodes = append(nodes, *node)
		}
		if out.Result.NextPageOffset == nil {
			break
		}
		next = out.Result.NextPageOffset
	}
	return nodes, nil
}

// AddEdge is not supported in v1 (entity-only graph, no relation persistence,
// ADR-0038).
func (s *QdrantGraphStore) AddEdge(ctx context.Context, edge graphmodel.Edge) error {
	return fmt.Errorf("graph edges not supported in v1 (entity-only)")
}

// Traverse is not supported in v1 (edge-based traversal requires relations).
func (s *QdrantGraphStore) Traverse(ctx context.Context, startNodeID string, maxDepth int) ([]graphmodel.Node, error) {
	return nil, fmt.Errorf("graph traversal not supported in v1 (entity-only)")
}

// ensureCollection creates the vectorless node collection if missing.
func (s *QdrantGraphStore) ensureCollection(ctx context.Context) error {
	var out struct {
		Result *struct {
			Status string `json:"status"`
		} `json:"result"`
	}
	err := s.doJSON(ctx, http.MethodGet, "/collections/"+s.collection, nil, &out)
	if err == nil && out.Result != nil {
		return nil
	}
	err = s.doJSON(ctx, http.MethodPut, "/collections/"+s.collection, map[string]any{
		"vectors_config": map[string]any{},
	}, nil)
	if err != nil {
		return fmt.Errorf("failed to create graph collection %q: %w", s.collection, err)
	}
	return nil
}

// doJSON performs a REST call, marshaling body (when non-nil) and unmarshaling
// the response into out (when non-nil).
func (s *QdrantGraphStore) doJSON(ctx context.Context, method, path string, body any, out any) error {
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(s.baseURL + path)
	req.Header.SetMethod(method)
	req.Header.Set("Content-Type", "application/json")
	// One connection per request: the Qdrant REST server closes pooled
	// keep-alive connections mid-request under fasthttp, surfacing as
	// "server closed connection before returning the first response byte".
	req.Header.Set("Connection", "close")
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		req.SetBody(raw)
	}

	if err := s.client.DoTimeout(req, resp, 30*time.Second); err != nil {
		return fmt.Errorf("qdrant request %s %s failed: %w", method, path, err)
	}
	status := resp.StatusCode()
	respBody := resp.Body()
	if status < 200 || status >= 300 {
		return fmt.Errorf("qdrant returned status %d: %s", status, truncate(string(respBody), 200))
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// --- payload helpers --------------------------------------------------------

func propString(payload map[string]any, key string) string {
	if v, ok := payload[key].(string); ok {
		return v
	}
	return ""
}

// nodeFromPayload reconstructs a Node from the atomic field-per-chunk
// payload schema: chunk evidence fields are "chunk_<id>", per-document score
// fields are "score_<doc>", and the node score is the max of all document
// scores (never regresses). Chunk order is sorted for determinism (payload
// map iteration is unordered).
func nodeFromPayload(payload map[string]any) (*graphmodel.Node, error) {
	id := propString(payload, "id")
	if id == "" {
		return nil, fmt.Errorf("graph point payload missing id")
	}
	var chunks []string
	maxScore := 0.0
	for key, v := range payload {
		switch {
		case strings.HasPrefix(key, chunkFieldPrefix) && key != "chunk_ids":
			if raw, err := hex.DecodeString(strings.TrimPrefix(key, chunkFieldPrefix)); err == nil {
				chunks = append(chunks, string(raw))
			}
		case strings.HasPrefix(key, scoreFieldPrefix):
			if f, ok := v.(float64); ok && f > maxScore {
				maxScore = f
			}
		}
	}
	sort.Strings(chunks)
	return &graphmodel.Node{
		ID:   id,
		Type: graphmodel.NodeType(propString(payload, "node_type")),
		Properties: map[string]any{
			"name":           propString(payload, "name"),
			"canonical_name": propString(payload, "canonical_name"),
			"entity_type":    propString(payload, "entity_type"),
			"score":          maxScore,
			"chunk_ids":      chunks,
		},
	}, nil
}

func canonicalNameOf(node graphmodel.Node) string {
	if node.Properties == nil {
		return ""
	}
	name, _ := node.Properties["canonical_name"].(string)
	return name
}

func entityTypeOf(node graphmodel.Node) string {
	if node.Properties == nil {
		return ""
	}
	t, _ := node.Properties["entity_type"].(string)
	return t
}
