package worker_test

import (
	"context"
	"testing"

	"arca/internal/indexing/model"
	"arca/internal/indexing/provider"
	"arca/internal/indexing/sparse"
	"arca/internal/indexing/store"
	"arca/internal/indexing/worker"
	pdfmodel "arca/internal/pdfinspector/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSparseEncoder returns a deterministic sparse vector per call.
type fakeSparseEncoder struct {
	calls int
}

func (e *fakeSparseEncoder) Encode(ctx context.Context, text string) (sparse.SparseVector, error) {
	e.calls++
	return sparse.SparseVector{
		Indices: []uint32{uint32(e.calls)},
		Values:  []float32{1.0},
	}, nil
}

// fakeSparseProvider returns a fake encoder and records the chunks it received.
type fakeSparseProvider struct {
	encoder *fakeSparseEncoder
	chunks  []sparse.DocumentChunk
}

func (p *fakeSparseProvider) Encoder(ctx context.Context, chunks []sparse.DocumentChunk) (sparse.SparseEncoder, error) {
	return p.encoderFor(chunks), nil
}

func (p *fakeSparseProvider) EncoderForCorpus(ctx context.Context) (sparse.SparseEncoder, error) {
	return p.encoderFor(nil), nil
}

func (p *fakeSparseProvider) encoderFor(chunks []sparse.DocumentChunk) sparse.SparseEncoder {
	p.chunks = chunks
	p.encoder = &fakeSparseEncoder{}
	return p.encoder
}

// sparseRecordingStore captures the last upserted points.
type sparseRecordingStore struct {
	inner        *store.InMemoryVectorStore
	lastUpserted []store.VectorPoint
}

func (s *sparseRecordingStore) UpsertPoints(ctx context.Context, points []store.VectorPoint) error {
	s.lastUpserted = points
	return s.inner.UpsertPoints(ctx, points)
}

func (s *sparseRecordingStore) SearchVector(ctx context.Context, query store.VectorSearchQuery) ([]store.VectorSearchResult, error) {
	return s.inner.SearchVector(ctx, query)
}

func (s *sparseRecordingStore) ListPoints(ctx context.Context, filter model.MetadataFilter) ([]store.VectorPoint, error) {
	return s.inner.ListPoints(ctx, filter)
}

func (s *sparseRecordingStore) Delete(ctx context.Context, filter model.MetadataFilter) error {
	return s.inner.Delete(ctx, filter)
}

func (s *sparseRecordingStore) Health(ctx context.Context) error {
	return s.inner.Health(ctx)
}

func TestIndexingWorker_EncodesSparseVectors(t *testing.T) {
	ctx := context.Background()

	storeImpl := &sparseRecordingStore{inner: store.NewInMemoryVectorStore()}
	contentStore := store.NewInMemoryContentStore()
	embProvider := provider.NewMockEmbeddingProvider("mock-provider", "mock-model-v1", 1536)
	sparseProvider := &fakeSparseProvider{}

	w := worker.NewIndexingWorker(embProvider, storeImpl, contentStore, worker.WithSparseEncoderProvider(sparseProvider))

	chunks := []pdfmodel.KnowledgeChunk{
		{
			ChunkID:         "chk-1",
			SectionPath:     "Introduction",
			ChunkOrder:      0,
			ContentMarkdown: "Creativity is a fundamental human quality.",
			ContentHash:     "hash-1",
		},
		{
			ChunkID:         "chk-2",
			SectionPath:     "Practice",
			ChunkOrder:      1,
			ContentMarkdown: "Discipline turns impulses into works.",
			ContentHash:     "hash-2",
		},
	}

	job, err := w.ExecuteSync(ctx, "doc-1", "Doc 1", chunks)
	require.NoError(t, err)
	require.Equal(t, 2, job.IndexedChunks)

	require.Len(t, sparseProvider.chunks, 2, "provider must receive the document chunks")
	assert.Equal(t, "chk-1", sparseProvider.chunks[0].ID)
	assert.Equal(t, "Creativity is a fundamental human quality.", sparseProvider.chunks[0].Content)

	upserted := storeImpl.lastUpserted
	require.Len(t, upserted, 2, "points must be upserted with sparse vectors")
	for _, pt := range upserted {
		require.NotNil(t, pt.Sparse, "every indexed point must carry a sparse vector")
		assert.Equal(t, 1, len(pt.Sparse.Indices))
	}
}

func TestIndexingWorker_WithoutSparseProvider(t *testing.T) {
	ctx := context.Background()

	storeImpl := &sparseRecordingStore{inner: store.NewInMemoryVectorStore()}
	contentStore := store.NewInMemoryContentStore()
	embProvider := provider.NewMockEmbeddingProvider("mock-provider", "mock-model-v1", 1536)

	// No provider: worker must behave exactly as before (no sparse vectors).
	w := worker.NewIndexingWorker(embProvider, storeImpl, contentStore)

	chunks := []pdfmodel.KnowledgeChunk{
		{
			ChunkID:         "chk-1",
			SectionPath:     "Intro",
			ChunkOrder:      0,
			ContentMarkdown: "Content.",
			ContentHash:     "hash-1",
		},
	}

	job, err := w.ExecuteSync(ctx, "doc-1", "Doc 1", chunks)
	require.NoError(t, err)
	require.Equal(t, 1, job.IndexedChunks)
	require.Len(t, storeImpl.lastUpserted, 1)
	assert.Nil(t, storeImpl.lastUpserted[0].Sparse, "no sparse vectors without a provider")
}
