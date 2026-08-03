package store_test

import (
	"context"
	"fmt"
	"net"
	"sort"
	"testing"

	"arca/internal/indexing/model"
	"arca/internal/indexing/sparse"
	"arca/internal/indexing/store"
	qdrant "github.com/qdrant/go-client/qdrant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func bufconnListen() *bufconn.Listener {
	return bufconn.Listen(1024 * 1024)
}

// fakeCollectionsServer implements the Qdrant collections service in-memory.
type fakeCollectionsServer struct {
	qdrant.UnimplementedCollectionsServer
	exists    bool
	created   bool
	createReq *qdrant.CreateCollection
}

func (f *fakeCollectionsServer) CollectionExists(ctx context.Context, req *qdrant.CollectionExistsRequest) (*qdrant.CollectionExistsResponse, error) {
	return &qdrant.CollectionExistsResponse{
		Result: &qdrant.CollectionExists{Exists: f.exists},
	}, nil
}

func (f *fakeCollectionsServer) Create(ctx context.Context, req *qdrant.CreateCollection) (*qdrant.CollectionOperationResponse, error) {
	f.created = true
	f.exists = true
	f.createReq = req
	return &qdrant.CollectionOperationResponse{Result: true}, nil
}

// fakePointsServer implements the Qdrant points service backed by an in-memory map.
type fakePointsServer struct {
	qdrant.UnimplementedPointsServer
	points   map[string]*storedPoint
	upserted int
	deleted  int
}

type storedPoint struct {
	id      *qdrant.PointId
	vector  []float32
	sparse  *sparse.SparseVector
	payload map[string]*qdrant.Value
}

func newFakePointsServer() *fakePointsServer {
	return &fakePointsServer{points: make(map[string]*storedPoint)}
}

func (f *fakePointsServer) pointKey(id *qdrant.PointId) string {
	if id == nil {
		return ""
	}
	if u, ok := id.PointIdOptions.(*qdrant.PointId_Uuid); ok {
		return u.Uuid
	}
	if n, ok := id.PointIdOptions.(*qdrant.PointId_Num); ok {
		return string(rune(n.Num))
	}
	return ""
}

func (f *fakePointsServer) Upsert(ctx context.Context, req *qdrant.UpsertPoints) (*qdrant.PointsOperationResponse, error) {
	for _, p := range req.GetPoints() {
		key := f.pointKey(p.GetId())
		if key == "" {
			continue
		}
		vec, sp := extractPointVectors(p)
		f.points[key] = &storedPoint{
			id:      p.GetId(),
			vector:  vec,
			sparse:  sp,
			payload: p.GetPayload(),
		}
		f.upserted++
	}
	return &qdrant.PointsOperationResponse{Result: &qdrant.UpdateResult{Status: qdrant.UpdateStatus_Completed}}, nil
}

// extractPointVectors pulls the dense vector data and the named sparse vector
// from a PointStruct, handling both the legacy flat Data field and the newer
// DenseVector/Dense oneofs plus NamedVectors.
func extractPointVectors(p *qdrant.PointStruct) ([]float32, *sparse.SparseVector) {
	if p == nil || p.GetVectors() == nil {
		return nil, nil
	}
	switch v := p.GetVectors().GetVectorsOptions().(type) {
	case *qdrant.Vectors_Vector:
		if v.Vector != nil {
			if dense := v.Vector.GetDense(); dense != nil {
				return dense.GetData(), nil
			}
			return v.Vector.GetData(), nil
		}
	case *qdrant.Vectors_Vectors:
		if v.Vectors != nil {
			var dense []float32
			for name, vec := range v.Vectors.GetVectors() {
				if vec == nil {
					continue
				}
				if name == "" {
					if d := vec.GetDense(); d != nil {
						dense = d.GetData()
					} else {
						dense = vec.GetData()
					}
				}
				if name == store.SparseVectorName {
					if sv := vec.GetSparse(); sv != nil {
						return dense, &sparse.SparseVector{
							Indices: sv.GetIndices(),
							Values:  sv.GetValues(),
						}
					}
				}
			}
			return dense, nil
		}
	}
	return nil, nil
}

func (f *fakePointsServer) Search(ctx context.Context, req *qdrant.SearchPoints) (*qdrant.SearchResponse, error) {
	var results []*qdrant.ScoredPoint
	for _, pt := range f.points {
		if !matchesFakeFilter(pt, req.GetFilter()) {
			continue
		}
		results = append(results, &qdrant.ScoredPoint{
			Id:      pt.id,
			Payload: pt.payload,
			Score:   float32(len(pt.vector)), // deterministic pseudo-score
		})
	}
	// Truncate to limit
	if req.GetLimit() > 0 && uint64(len(results)) > req.GetLimit() {
		results = results[:req.GetLimit()]
	}
	// Sort by score desc (ID asc as a deterministic tie-break) to mirror real
	// Qdrant best-match-first ordering; the fake otherwise iterates a map.
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return testPointID(results[i].Id) < testPointID(results[j].Id)
	})
	return &qdrant.SearchResponse{Result: results}, nil
}

// Query implements the query API used for sparse search: it computes the dot
// product of the nearest sparse input against stored sparse vectors.
func (f *fakePointsServer) Query(ctx context.Context, req *qdrant.QueryPoints) (*qdrant.QueryResponse, error) {
	nearest, ok := req.GetQuery().GetVariant().(*qdrant.Query_Nearest)
	if !ok || nearest.Nearest == nil {
		return &qdrant.QueryResponse{}, nil
	}
	sp := nearest.Nearest.GetSparse()
	if sp == nil {
		return &qdrant.QueryResponse{}, nil
	}

	var results []*qdrant.ScoredPoint
	for _, pt := range f.points {
		if !matchesFakeFilter(pt, req.GetFilter()) {
			continue
		}
		if pt.sparse == nil {
			continue
		}
		score := sparseDotTest(sp, pt.sparse)
		if req.ScoreThreshold != nil && score < *req.ScoreThreshold {
			continue
		}
		results = append(results, &qdrant.ScoredPoint{
			Id:      pt.id,
			Payload: pt.payload,
			Score:   score,
		})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return testPointID(results[i].Id) < testPointID(results[j].Id)
	})
	if req.Limit != nil && uint64(len(results)) > *req.Limit {
		results = results[:*req.Limit]
	}
	return &qdrant.QueryResponse{Result: results}, nil
}

// sparseDotTest computes the dot product over shared sparse indices.
func sparseDotTest(a *qdrant.SparseVector, b *sparse.SparseVector) float32 {
	i, j := 0, 0
	var sum float64
	for i < len(a.GetIndices()) && j < len(b.Indices) {
		switch {
		case a.GetIndices()[i] < b.Indices[j]:
			i++
		case a.GetIndices()[i] > b.Indices[j]:
			j++
		default:
			sum += float64(a.GetValues()[i]) * float64(b.Values[j])
			i++
			j++
		}
	}
	return float32(sum)
}

// testPointID renders a PointId as its string form for deterministic ordering.
func testPointID(id *qdrant.PointId) string {
	if id == nil {
		return ""
	}
	if u := id.GetUuid(); u != "" {
		return u
	}
	return fmt.Sprintf("%d", id.GetNum())
}

func (f *fakePointsServer) Scroll(ctx context.Context, req *qdrant.ScrollPoints) (*qdrant.ScrollResponse, error) {
	var results []*qdrant.RetrievedPoint
	for _, pt := range f.points {
		if !matchesFakeFilter(pt, req.GetFilter()) {
			continue
		}
		vo := &qdrant.VectorsOutput{
			VectorsOptions: &qdrant.VectorsOutput_Vector{
				Vector: &qdrant.VectorOutput{
					Vector: &qdrant.VectorOutput_Dense{
						Dense: &qdrant.DenseVector{Data: pt.vector},
					},
				},
			},
		}
		if pt.sparse != nil {
			vo = &qdrant.VectorsOutput{
				VectorsOptions: &qdrant.VectorsOutput_Vectors{
					Vectors: &qdrant.NamedVectorsOutput{
						Vectors: map[string]*qdrant.VectorOutput{
							"": {
								Vector: &qdrant.VectorOutput_Dense{
									Dense: &qdrant.DenseVector{Data: pt.vector},
								},
							},
							store.SparseVectorName: {
								Vector: &qdrant.VectorOutput_Sparse{
									Sparse: &qdrant.SparseVector{
										Indices: pt.sparse.Indices,
										Values:  pt.sparse.Values,
									},
								},
							},
						},
					},
				},
			}
		}
		results = append(results, &qdrant.RetrievedPoint{
			Id:      pt.id,
			Payload: pt.payload,
			Vectors: vo,
		})
	}
	limit := int(req.GetLimit())
	if limit == 0 {
		limit = len(results)
	}
	if limit > len(results) {
		limit = len(results)
	}
	return &qdrant.ScrollResponse{Result: results[:limit]}, nil
}

func (f *fakePointsServer) Delete(ctx context.Context, req *qdrant.DeletePoints) (*qdrant.PointsOperationResponse, error) {
	selector := req.GetPoints()
	if selector == nil {
		return &qdrant.PointsOperationResponse{Result: &qdrant.UpdateResult{Status: qdrant.UpdateStatus_Completed}}, nil
	}

	ids := selector.GetPoints().GetIds()
	if len(ids) == 0 {
		// Filter-based delete
		for key, pt := range f.points {
			if matchesFakeFilter(pt, selector.GetFilter()) {
				delete(f.points, key)
				f.deleted++
			}
		}
		return &qdrant.PointsOperationResponse{Result: &qdrant.UpdateResult{Status: qdrant.UpdateStatus_Completed}}, nil
	}

	for _, id := range ids {
		key := f.pointKey(id)
		if _, ok := f.points[key]; ok {
			delete(f.points, key)
			f.deleted++
		}
	}
	return &qdrant.PointsOperationResponse{Result: &qdrant.UpdateResult{Status: qdrant.UpdateStatus_Completed}}, nil
}

// matchesFakeFilter evaluates a Filter against stored point payload (limited subset).
func matchesFakeFilter(pt *storedPoint, filter *qdrant.Filter) bool {
	if filter == nil || len(filter.GetMust()) == 0 {
		return true
	}
	for _, cond := range filter.GetMust() {
		field := cond.GetField()
		if field == nil {
			continue
		}
		key := field.GetKey()
		switch match := field.GetMatch().GetMatchValue().(type) {
		case *qdrant.Match_Keyword:
			if val := payloadString(pt.payload, key); val != match.Keyword {
				return false
			}
		case *qdrant.Match_Keywords:
			found := false
			val := payloadString(pt.payload, key)
			for _, kw := range match.Keywords.GetStrings() {
				if val == kw {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		case *qdrant.Match_Integer:
			if int64(payloadInt(pt.payload, key)) != match.Integer {
				return false
			}
		case *qdrant.Match_Integers:
			found := false
			values := payloadInts(pt.payload, key)
			for _, i := range match.Integers.GetIntegers() {
				for _, val := range values {
					if int64(val) == i {
						found = true
						break
					}
				}
				if found {
					break
				}
			}
			if !found {
				return false
			}
		}
	}
	return true
}

func payloadString(payload map[string]*qdrant.Value, key string) string {
	if v := payload[key]; v != nil {
		return v.GetStringValue()
	}
	return ""
}

func payloadInt(payload map[string]*qdrant.Value, key string) int {
	if v := payload[key]; v != nil {
		return int(v.GetIntegerValue())
	}
	return 0
}

func payloadInts(payload map[string]*qdrant.Value, key string) []int {
	var out []int
	if v := payload[key]; v != nil {
		if list := v.GetListValue(); list != nil {
			for _, item := range list.GetValues() {
				out = append(out, int(item.GetIntegerValue()))
			}
		}
	}
	return out
}

// newTestQdrantStore spins up an in-process gRPC server with fake Qdrant services
// and returns a QdrantVectorStore pointed at it.
func newTestQdrantStore(t *testing.T, points *fakePointsServer, collections *fakeCollectionsServer) *store.QdrantVectorStore {
	t.Helper()

	if points == nil {
		points = newFakePointsServer()
	}
	if collections == nil {
		collections = &fakeCollectionsServer{}
	}

	lis := bufconnListen()
	srv := grpc.NewServer()
	qdrant.RegisterCollectionsServer(srv, collections)
	qdrant.RegisterPointsServer(srv, points)

	go func() {
		_ = srv.Serve(lis)
	}()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	grpcClient := qdrant.NewGrpcClientFromConn(conn)
	qs := store.NewQdrantVectorStoreForTest(
		grpcClient.Points(),
		grpcClient.Collections(),
		"test_collection",
	)
	return qs
}

func samplePoints() []store.VectorPoint {
	return []store.VectorPoint{
		{
			ID:              "pt-1",
			Vector:          []float32{1.0, 0.0, 0.0},
			ContentMarkdown: "Sample markdown content for the first chunk.",
			Metadata: model.VectorMetadata{
				WorkspaceID:       "ws-1",
				KnowledgeSpaceID:  "space-1",
				DocumentID:        "doc-1",
				ChunkID:           "chk-1",
				ChunkOrder:        0,
				SectionPath:       "Intro",
				PageNumbers:       []int{1, 2},
				ContentType:       "paragraph",
				Citations:         []string{"[1] Smith et al."},
				ContentHash:       "hash-1",
				EmbeddingProvider: "Ollama",
				EmbeddingModel:    "nomic-embed-text",
				EmbeddingVersion:  "1.0.0",
				ChunkSchemaVer:    "1.0",
				IndexSignature:    "sig-1",
			},
		},
		{
			ID:     "pt-2",
			Vector: []float32{0.0, 1.0, 0.0},
			Metadata: model.VectorMetadata{
				WorkspaceID:      "ws-1",
				KnowledgeSpaceID: "space-1",
				DocumentID:       "doc-1",
				ChunkID:          "chk-2",
				ChunkOrder:       1,
				SectionPath:      "Architecture",
				PageNumbers:      []int{3},
				ContentType:      "table",
				ContentHash:      "hash-2",
			},
		},
	}
}

func TestQdrantVectorStore_UpsertSearchListDelete(t *testing.T) {
	ctx := context.Background()
	qs := newTestQdrantStore(t, nil, nil)

	points := samplePoints()
	require.NoError(t, qs.UpsertPoints(ctx, points))

	t.Run("search returns stored points with metadata", func(t *testing.T) {
		results, err := qs.SearchVector(ctx, store.VectorSearchQuery{
			Vector: []float32{1.0, 0.0, 0.0},
			TopK:   10,
		})
		require.NoError(t, err)
		require.Len(t, results, 2)
		assert.Equal(t, "pt-1", results[0].ID)
		assert.Equal(t, "chk-1", results[0].Metadata.ChunkID)
		assert.Equal(t, "ws-1", results[0].Metadata.WorkspaceID)
		assert.Equal(t, []int{1, 2}, results[0].Metadata.PageNumbers)
		assert.Equal(t, []string{"[1] Smith et al."}, results[0].Metadata.Citations)
		assert.Equal(t, "sig-1", results[0].Metadata.IndexSignature)
		assert.Equal(t, "Sample markdown content for the first chunk.", results[0].ContentMarkdown, "search must return persisted content")
	})

	t.Run("search respects document filter", func(t *testing.T) {
		results, err := qs.SearchVector(ctx, store.VectorSearchQuery{
			Vector: []float32{1.0, 0.0, 0.0},
			TopK:   10,
			Filter: model.MetadataFilter{DocumentIDs: []string{"doc-1"}},
		})
		require.NoError(t, err)
		require.Len(t, results, 2)
	})

	t.Run("search respects chunk id filter", func(t *testing.T) {
		results, err := qs.SearchVector(ctx, store.VectorSearchQuery{
			Vector: []float32{1.0, 0.0, 0.0},
			TopK:   10,
			Filter: model.MetadataFilter{ChunkIDs: []string{"chk-2"}},
		})
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "pt-2", results[0].ID)
	})

	t.Run("list points returns all without ranking", func(t *testing.T) {
		listed, err := qs.ListPoints(ctx, model.MetadataFilter{DocumentIDs: []string{"doc-1"}})
		require.NoError(t, err)
		require.Len(t, listed, 2)
		for _, pt := range listed {
			require.NotEmpty(t, pt.Vector, "ListPoints must preserve vectors")
			assert.Equal(t, "doc-1", pt.Metadata.DocumentID)
		}
		contentByChunk := map[string]string{}
		for _, pt := range listed {
			contentByChunk[pt.Metadata.ChunkID] = pt.ContentMarkdown
		}
		assert.Equal(t, "Sample markdown content for the first chunk.", contentByChunk["chk-1"], "ListPoints must preserve persisted content")
	})

	t.Run("list points respects content type filter", func(t *testing.T) {
		listed, err := qs.ListPoints(ctx, model.MetadataFilter{ContentTypes: []string{"table"}})
		require.NoError(t, err)
		require.Len(t, listed, 1)
		assert.Equal(t, "pt-2", listed[0].ID)
	})

	t.Run("delete by point IDs removes only those points", func(t *testing.T) {
		require.NoError(t, qs.Delete(ctx, model.MetadataFilter{PointIDs: []string{"pt-2"}}))
		results, err := qs.SearchVector(ctx, store.VectorSearchQuery{Vector: []float32{0, 1, 0}, TopK: 10})
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "pt-1", results[0].ID)
	})
}

func TestQdrantVectorStore_CollectionAutoCreate(t *testing.T) {
	ctx := context.Background()
	collections := &fakeCollectionsServer{}
	qs := newTestQdrantStore(t, nil, collections)

	require.False(t, collections.exists, "collection should start absent")

	// Health before creation should report missing collection
	err := qs.Health(ctx)
	require.Error(t, err, "health should fail when collection missing")
	assert.Contains(t, err.Error(), "does not exist")

	// Upsert triggers auto-create with 768-dim cosine
	require.NoError(t, qs.UpsertPoints(ctx, samplePoints()))
	require.True(t, collections.created, "expected collection auto-creation")
	require.Equal(t, "test_collection", collections.createReq.GetCollectionName())
	params := collections.createReq.GetVectorsConfig().GetParams()
	require.NotNil(t, params)
	assert.Equal(t, uint64(768), params.GetSize())
	assert.Equal(t, qdrant.Distance_Cosine, params.GetDistance())

	require.NoError(t, qs.Health(ctx), "health should pass after creation")
}

func TestQdrantVectorStore_PageAndContentTypeFilters(t *testing.T) {
	ctx := context.Background()
	qs := newTestQdrantStore(t, nil, nil)
	require.NoError(t, qs.UpsertPoints(ctx, samplePoints()))

	t.Run("page number filter matches points containing that page", func(t *testing.T) {
		listed, err := qs.ListPoints(ctx, model.MetadataFilter{PageNumbers: []int{2}})
		require.NoError(t, err)
		require.Len(t, listed, 1)
		assert.Equal(t, "pt-1", listed[0].ID)
	})

	t.Run("workspace filter narrows results", func(t *testing.T) {
		listed, err := qs.ListPoints(ctx, model.MetadataFilter{WorkspaceID: "ws-1", KnowledgeSpaceID: "space-1"})
		require.NoError(t, err)
		require.Len(t, listed, 2)
	})
}
