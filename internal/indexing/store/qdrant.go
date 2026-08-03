package store

import (
	"context"
	"fmt"
	"strings"

	"arca/internal/indexing/model"
	qdrant "github.com/qdrant/go-client/qdrant"
)

// Default Qdrant connection and collection parameters.
const (
	defaultQdrantPort = 6334
	scrollBatchSize   = 100
)

// QdrantVectorStore implements VectorStore against a Qdrant cluster via the
// official Go gRPC client. The collection stores 768-dimension vectors with
// Cosine distance; VectorMetadata is stored as the point payload.
type QdrantVectorStore struct {
	points      qdrant.PointsClient
	collections qdrant.CollectionsClient
	collection  string
	dimension   uint64
	closeFn     func() error
}

// QdrantOption configures a QdrantVectorStore instance.
type QdrantOption func(*QdrantVectorStore)

// WithQdrantDimension overrides the collection vector dimension (default 768).
func WithQdrantDimension(d uint64) QdrantOption {
	return func(s *QdrantVectorStore) {
		if d > 0 {
			s.dimension = d
		}
	}
}

// NewQdrantVectorStore constructs a QdrantVectorStore and ensures the target
// collection exists (created with 768-dim Cosine vectors if missing).
// host should be "hostname" or "hostname:port" (defaults gRPC port 6334).
func NewQdrantVectorStore(host, collection string, opts ...QdrantOption) (*QdrantVectorStore, error) {
	cfg := &qdrant.Config{
		Host:                  host,
		Port:                  defaultQdrantPort,
		SkipCompatibilityCheck: true,
	}
	if h, p, ok := splitHostPort(host); ok {
		cfg.Host = h
		cfg.Port = p
	}
	if collection == "" {
		collection = "arca_chunks"
	}

	client, err := qdrant.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create qdrant client: %w", err)
	}

	s := &QdrantVectorStore{
		points:      client.GetPointsClient(),
		collections: client.GetCollectionsClient(),
		collection:  collection,
		dimension:   768,
		closeFn:     client.Close,
	}
	for _, opt := range opts {
		opt(s)
	}

	return s, nil
}

// newQdrantVectorStoreWithClients constructs a QdrantVectorStore from explicit
// gRPC client seams (test/embedding use only).
func newQdrantVectorStoreWithClients(points qdrant.PointsClient, collections qdrant.CollectionsClient, collection string, opts ...QdrantOption) *QdrantVectorStore {
	s := &QdrantVectorStore{
		points:      points,
		collections: collections,
		collection:  collection,
		dimension:   768,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// NewQdrantVectorStoreForTest constructs a QdrantVectorStore from explicit gRPC
// client seams for testing. It is exposed so integration/fake-client tests can
// inject in-process servers without a live Qdrant instance.
func NewQdrantVectorStoreForTest(points qdrant.PointsClient, collections qdrant.CollectionsClient, collection string) *QdrantVectorStore {
	return newQdrantVectorStoreWithClients(points, collections, collection)
}

// splitHostPort splits "host" or "host:port" into its components.
func splitHostPort(addr string) (string, int, bool) {
	host, portStr, ok := strings.Cut(addr, ":")
	if !ok {
		return addr, 0, false
	}
	port := 0
	_, err := fmt.Sscanf(portStr, "%d", &port)
	if err != nil || port <= 0 {
		return addr, 0, false
	}
	return host, port, true
}

// ensureCollection checks for and creates the collection if missing.
func (s *QdrantVectorStore) ensureCollection(ctx context.Context) error {
	exists, err := s.collectionExists(ctx)
	if err != nil {
		return fmt.Errorf("failed to check qdrant collection %q: %w", s.collection, err)
	}
	if exists {
		return nil
	}

	req := &qdrant.CreateCollection{
		CollectionName: s.collection,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     s.dimension,
			Distance: qdrant.Distance_Cosine,
		}),
	}
	if _, err := s.collections.Create(ctx, req); err != nil {
		return fmt.Errorf("failed to create qdrant collection %q: %w", s.collection, err)
	}
	return nil
}

// collectionExists reports whether the configured collection exists.
func (s *QdrantVectorStore) collectionExists(ctx context.Context) (bool, error) {
	resp, err := s.collections.CollectionExists(ctx, &qdrant.CollectionExistsRequest{CollectionName: s.collection})
	if err != nil {
		return false, err
	}
	if resp.GetResult() == nil {
		return false, nil
	}
	return resp.GetResult().GetExists(), nil
}

// Health verifies Qdrant connectivity and collection readiness.
func (s *QdrantVectorStore) Health(ctx context.Context) error {
	exists, err := s.collectionExists(ctx)
	if err != nil {
		return fmt.Errorf("qdrant health check failed: %w", err)
	}
	if !exists {
		return fmt.Errorf("qdrant collection %q does not exist", s.collection)
	}
	return nil
}

// UpsertPoints stores or updates vector points in-place based on Point ID.
func (s *QdrantVectorStore) UpsertPoints(ctx context.Context, points []VectorPoint) error {
	if err := s.ensureCollection(ctx); err != nil {
		return err
	}

	pointStructs := make([]*qdrant.PointStruct, 0, len(points))
	for _, pt := range points {
		if err := pt.Validate(); err != nil {
			return err
		}

		payload, err := metadataToPayload(pt.Metadata)
		if err != nil {
			return err
		}

		pointStructs = append(pointStructs, &qdrant.PointStruct{
			Id:      qdrant.NewID(pt.ID),
			Vectors: qdrant.NewVectorsDense(pt.Vector),
			Payload: payload,
		})
	}

	if _, err := s.points.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: s.collection,
		Points:         pointStructs,
	}); err != nil {
		return fmt.Errorf("qdrant upsert failed: %w", err)
	}
	return nil
}

// SearchVector executes nearest-neighbor Cosine similarity search with MetadataFilter.
func (s *QdrantVectorStore) SearchVector(ctx context.Context, query VectorSearchQuery) ([]VectorSearchResult, error) {
	if err := s.ensureCollection(ctx); err != nil {
		return nil, err
	}

	filter, err := filterToQdrant(query.Filter)
	if err != nil {
		return nil, err
	}

	req := &qdrant.SearchPoints{
		CollectionName: s.collection,
		Vector:         query.Vector,
		Limit:          uint64(query.TopK),
		Filter:         filter,
		WithPayload:    qdrant.NewWithPayload(true),
	}
	if query.MinScore > 0 {
		score := query.MinScore
		req.ScoreThreshold = &score
	}
	if query.TopK <= 0 {
		req.Limit = 10
	}

	scored, err := s.points.Search(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("qdrant search failed: %w", err)
	}

	results := make([]VectorSearchResult, 0, len(scored.Result))
	for _, sp := range scored.Result {
		meta, err := payloadToMetadata(sp.Payload)
		if err != nil {
			return nil, err
		}
		results = append(results, VectorSearchResult{
			ID:       pointIDToString(sp.Id),
			Score:    sp.Score,
			Metadata: meta,
		})
	}
	return results, nil
}

// ListPoints enumerates all stored points matching the filter via the Qdrant
// scroll API. This is a read operation for differential indexing, not a
// similarity search, and is never truncated by TopK.
func (s *QdrantVectorStore) ListPoints(ctx context.Context, filter model.MetadataFilter) ([]VectorPoint, error) {
	if err := s.ensureCollection(ctx); err != nil {
		return nil, err
	}

	qfilter, err := filterToQdrant(filter)
	if err != nil {
		return nil, err
	}

	var points []VectorPoint
	var offset *qdrant.PointId
	limit := uint32(scrollBatchSize)

	for {
		req := &qdrant.ScrollPoints{
			CollectionName: s.collection,
			Filter:         qfilter,
			WithPayload:    qdrant.NewWithPayload(true),
			WithVectors:    qdrant.NewWithVectors(true),
			Limit:          &limit,
			Offset:         offset,
		}

		batch, nextOffset, err := s.scrollBatch(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("qdrant scroll failed: %w", err)
		}

		for _, rp := range batch {
			meta, err := payloadToMetadata(rp.Payload)
			if err != nil {
				return nil, err
			}
			points = append(points, VectorPoint{
				ID:       pointIDToString(rp.Id),
				Vector:   vectorsOutputToSlice(rp.Vectors),
				Metadata: meta,
			})
		}

		if nextOffset == nil || len(batch) == 0 {
			break
		}
		offset = nextOffset
	}

	return points, nil
}

// scrollBatch performs one scroll page and returns the batch plus the next offset.
func (s *QdrantVectorStore) scrollBatch(ctx context.Context, req *qdrant.ScrollPoints) ([]*qdrant.RetrievedPoint, *qdrant.PointId, error) {
	resp, err := s.points.Scroll(ctx, req)
	if err != nil {
		return nil, nil, err
	}
	return resp.GetResult(), resp.GetNextPageOffset(), nil
}

// Delete removes vector points matching the given MetadataFilter.
func (s *QdrantVectorStore) Delete(ctx context.Context, filter model.MetadataFilter) error {
	if err := s.ensureCollection(ctx); err != nil {
		return err
	}

	var selector *qdrant.PointsSelector
	if len(filter.PointIDs) > 0 {
		ids := make([]*qdrant.PointId, 0, len(filter.PointIDs))
		for _, id := range filter.PointIDs {
			ids = append(ids, qdrant.NewID(id))
		}
		selector = qdrant.NewPointsSelectorIDs(ids)
	} else {
		qfilter, err := filterToQdrant(filter)
		if err != nil {
			return err
		}
		selector = qdrant.NewPointsSelectorFilter(qfilter)
	}

	if _, err := s.points.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: s.collection,
		Points:         selector,
	}); err != nil {
		return fmt.Errorf("qdrant delete failed: %w", err)
	}
	return nil
}

// Close releases the underlying Qdrant connection.
func (s *QdrantVectorStore) Close() error {
	if s.closeFn == nil {
		return nil
	}
	return s.closeFn()
}

// metadataToPayload serializes VectorMetadata into a Qdrant payload map.
// Keep this schema stable: it is the source of truth for filter translation
// and metadata reconstruction.
func metadataToPayload(meta model.VectorMetadata) (map[string]*qdrant.Value, error) {
	raw := map[string]any{
		"workspace_id":        meta.WorkspaceID,
		"knowledge_space_id":  meta.KnowledgeSpaceID,
		"document_id":         meta.DocumentID,
		"chunk_id":            meta.ChunkID,
		"chunk_order":         int64(meta.ChunkOrder),
		"section_path":        meta.SectionPath,
		"content_type":        meta.ContentType,
		"content_hash":        meta.ContentHash,
		"embedding_provider":  meta.EmbeddingProvider,
		"embedding_model":     meta.EmbeddingModel,
		"embedding_version":   meta.EmbeddingVersion,
		"chunk_schema_version": meta.ChunkSchemaVer,
		"index_signature":     meta.IndexSignature,
	}
	if len(meta.PageNumbers) > 0 {
		pages := make([]any, len(meta.PageNumbers))
		for i, p := range meta.PageNumbers {
			pages[i] = int64(p)
		}
		raw["page_numbers"] = pages
	}
	if len(meta.Citations) > 0 {
		cites := make([]any, len(meta.Citations))
		for i, c := range meta.Citations {
			cites[i] = c
		}
		raw["citations"] = cites
	}

	payload, err := qdrant.TryValueMap(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to build qdrant payload: %w", err)
	}
	return payload, nil
}

// payloadToMetadata reconstructs VectorMetadata from a Qdrant payload map.
func payloadToMetadata(payload map[string]*qdrant.Value) (model.VectorMetadata, error) {
	var meta model.VectorMetadata

	if v := payload["workspace_id"]; v != nil {
		meta.WorkspaceID = v.GetStringValue()
	}
	if v := payload["knowledge_space_id"]; v != nil {
		meta.KnowledgeSpaceID = v.GetStringValue()
	}
	if v := payload["document_id"]; v != nil {
		meta.DocumentID = v.GetStringValue()
	}
	if v := payload["chunk_id"]; v != nil {
		meta.ChunkID = v.GetStringValue()
	}
	if v := payload["chunk_order"]; v != nil {
		meta.ChunkOrder = int(v.GetIntegerValue())
	}
	if v := payload["section_path"]; v != nil {
		meta.SectionPath = v.GetStringValue()
	}
	if v := payload["content_type"]; v != nil {
		meta.ContentType = v.GetStringValue()
	}
	if v := payload["content_hash"]; v != nil {
		meta.ContentHash = v.GetStringValue()
	}
	if v := payload["embedding_provider"]; v != nil {
		meta.EmbeddingProvider = v.GetStringValue()
	}
	if v := payload["embedding_model"]; v != nil {
		meta.EmbeddingModel = v.GetStringValue()
	}
	if v := payload["embedding_version"]; v != nil {
		meta.EmbeddingVersion = v.GetStringValue()
	}
	if v := payload["chunk_schema_version"]; v != nil {
		meta.ChunkSchemaVer = v.GetStringValue()
	}
	if v := payload["index_signature"]; v != nil {
		meta.IndexSignature = v.GetStringValue()
	}

	if v := payload["page_numbers"]; v != nil {
		if list := v.GetListValue(); list != nil {
			for _, item := range list.GetValues() {
				meta.PageNumbers = append(meta.PageNumbers, int(item.GetIntegerValue()))
			}
		}
	}
	if v := payload["citations"]; v != nil {
		if list := v.GetListValue(); list != nil {
			for _, item := range list.GetValues() {
				meta.Citations = append(meta.Citations, item.GetStringValue())
			}
		}
	}

	return meta, nil
}

// filterToQdrant translates a domain MetadataFilter into a Qdrant Filter.
func filterToQdrant(filter model.MetadataFilter) (*qdrant.Filter, error) {
	if err := filter.Validate(); err != nil {
		return nil, err
	}

	var must []*qdrant.Condition

	if filter.WorkspaceID != "" {
		must = append(must, qdrant.NewMatchKeyword("workspace_id", filter.WorkspaceID))
	}
	if filter.KnowledgeSpaceID != "" {
		must = append(must, qdrant.NewMatchKeyword("knowledge_space_id", filter.KnowledgeSpaceID))
	}
	if len(filter.DocumentIDs) > 0 {
		must = append(must, qdrant.NewMatchKeywords("document_id", filter.DocumentIDs...))
	}
	if len(filter.ChunkIDs) > 0 {
		must = append(must, qdrant.NewMatchKeywords("chunk_id", filter.ChunkIDs...))
	}
	if len(filter.PageNumbers) > 0 {
		pages := make([]int64, len(filter.PageNumbers))
		for i, p := range filter.PageNumbers {
			pages[i] = int64(p)
		}
		must = append(must, qdrant.NewMatchInts("page_numbers", pages...))
	}
	if len(filter.ContentTypes) > 0 {
		must = append(must, qdrant.NewMatchKeywords("content_type", filter.ContentTypes...))
	}

	if len(must) == 0 {
		return nil, nil
	}
	return &qdrant.Filter{Must: must}, nil
}

// pointIDToString renders a PointId as its UUID string form.
func pointIDToString(id *qdrant.PointId) string {
	if id == nil {
		return ""
	}
	switch v := id.PointIdOptions.(type) {
	case *qdrant.PointId_Uuid:
		return v.Uuid
	case *qdrant.PointId_Num:
		return fmt.Sprintf("%d", v.Num)
	default:
		return ""
	}
}

// vectorsOutputToSlice extracts the default dense vector from a VectorsOutput.
func vectorsOutputToSlice(vo *qdrant.VectorsOutput) []float32 {
	if vo == nil {
		return nil
	}
	switch v := vo.VectorsOptions.(type) {
	case *qdrant.VectorsOutput_Vector:
		return vectorOutputToSlice(v.Vector)
	case *qdrant.VectorsOutput_Vectors:
		if v.Vectors == nil {
			return nil
		}
		for _, vec := range v.Vectors.GetVectors() {
			if vec != nil {
				return vectorOutputToSlice(vec)
			}
		}
	}
	return nil
}

// vectorOutputToSlice extracts the dense data from a VectorOutput.
func vectorOutputToSlice(v *qdrant.VectorOutput) []float32 {
	if v == nil {
		return nil
	}
	if dense := v.GetDense(); dense != nil {
		return dense.GetData()
	}
	return v.GetData()
}

// compile-time interface assertion.
var _ VectorStore = (*QdrantVectorStore)(nil)
