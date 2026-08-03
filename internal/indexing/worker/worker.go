package worker

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"arca/internal/indexing/diff"
	indexingjob "arca/internal/indexing/job"
	indexingmodel "arca/internal/indexing/model"
	"arca/internal/indexing/provider"
	"arca/internal/indexing/store"
	pdfmodel "arca/internal/pdfinspector/model"
)

var jobCounter uint64

// IndexingWorker orchestrates differential diff calculation, provider embedding batching, and vector store upserts.
type IndexingWorker struct {
	provider     provider.EmbeddingProvider
	store        store.VectorStore
	contentStore store.ContentStore
}

// NewIndexingWorker constructs an IndexingWorker instance.
func NewIndexingWorker(p provider.EmbeddingProvider, s store.VectorStore, c store.ContentStore) *IndexingWorker {
	return &IndexingWorker{
		provider:     p,
		store:        s,
		contentStore: c,
	}
}

// ExecuteSync executes document indexing synchronously and returns the completed IndexingJob.
func (w *IndexingWorker) ExecuteSync(ctx context.Context, documentID string, chunks []pdfmodel.KnowledgeChunk) (*indexingjob.IndexingJob, error) {
	if documentID == "" {
		return nil, fmt.Errorf("documentID cannot be empty")
	}

	jobID := fmt.Sprintf("job-%d-%d", time.Now().UnixNano(), atomic.AddUint64(&jobCounter, 1))
	jobObj := indexingjob.NewIndexingJob(jobID, documentID, len(chunks))

	caps := w.provider.Capabilities()
	jobObj.EmbeddingProvider = w.provider.Provider()
	jobObj.EmbeddingModel = w.provider.Model()

	if err := jobObj.TransitionTo(indexingjob.StatusRunning); err != nil {
		return nil, err
	}

	// 1. Enumerate existing vector points for this document to perform differential
	// diff. Listing is a read operation (ListPoints), not a similarity search, so it
	// is never truncated by TopK and needs no query vector.
	existingPoints, err := w.store.ListPoints(ctx, indexingmodel.MetadataFilter{DocumentIDs: []string{documentID}})
	if err != nil {
		jobObj.SetError(err)
		return jobObj, err
	}

	existingMeta := make([]indexingmodel.VectorMetadata, len(existingPoints))
	for i, pt := range existingPoints {
		existingMeta[i] = pt.Metadata
	}

	// 2. Compute DiffPlan using DiffEngine
	diffEngine := diff.NewEngine(jobObj.EmbeddingProvider, jobObj.EmbeddingModel, "1.0.0", "1.0")
	diffPlan := diffEngine.ComputeDiffPlan(documentID, chunks, existingMeta)

	// 3. Process deletions for removed sections (DiffPlan.DeletedPointIDs holds
	// stable point IDs, so deletion must filter by PointIDs, not ChunkIDs).
	if len(diffPlan.DeletedPointIDs) > 0 {
		jobObj.SetDeletedChunks(len(diffPlan.DeletedPointIDs))
		if err := w.store.Delete(ctx, indexingmodel.MetadataFilter{
			DocumentIDs: []string{documentID},
			PointIDs:    diffPlan.DeletedPointIDs,
		}); err != nil {
			jobObj.SetError(err)
			return jobObj, err
		}
		if err := w.contentStore.DeleteContent(ctx, diffPlan.DeletedChunkIDs); err != nil {
			jobObj.SetError(err)
			return jobObj, err
		}
	}

	// 4. Batch & Generate Embeddings for new/modified chunks
	chunksToEmbed := diffPlan.ChunksToEmbed()
	batchSize := caps.MaxBatchSize
	if batchSize <= 0 {
		batchSize = 50
	}

	var newPoints []store.VectorPoint

	for idx := 0; idx < len(chunksToEmbed); idx += batchSize {
		end := idx + batchSize
		if end > len(chunksToEmbed) {
			end = len(chunksToEmbed)
		}

		batchChunks := chunksToEmbed[idx:end]
		texts := make([]string, len(batchChunks))
		for bIdx, chk := range batchChunks {
			texts[bIdx] = chk.ContentMarkdown
		}

		embRes, err := w.provider.EmbedDocuments(ctx, texts)
		if err != nil {
			jobObj.SetError(err)
			return jobObj, err
		}

		for bIdx, chk := range batchChunks {
			ptID := store.CalculatePointID(documentID, chk.SectionPath, chk.ChunkOrder)
			sig := indexingmodel.CalculateIndexSignature(chk.ContentHash, jobObj.EmbeddingProvider, jobObj.EmbeddingModel, "1.0.0", "1.0")

			meta := indexingmodel.VectorMetadata{
				DocumentID:        documentID,
				ChunkID:           chk.ChunkID,
				ChunkOrder:        chk.ChunkOrder,
				SectionPath:       chk.SectionPath,
				PageNumbers:       chk.PageNumbers,
				Citations:         extractCitationTexts(chk.Citations),
				ContentHash:       chk.ContentHash,
				EmbeddingProvider: embRes.Provider,
				EmbeddingModel:    embRes.Model,
				EmbeddingVersion:  embRes.Version,
				ChunkSchemaVer:    "1.0",
				IndexSignature:    sig,
			}

			newPoints = append(newPoints, store.VectorPoint{
				ID:       ptID,
				Vector:   embRes.Vectors[bIdx],
				Metadata: meta,
			})
		}
	}

	// 5. Upsert points into store and persist chunk markdown content.
	if len(newPoints) > 0 {
		if err := w.store.UpsertPoints(ctx, newPoints); err != nil {
			jobObj.SetError(err)
			return jobObj, err
		}

		chunkContents := make([]store.ChunkContent, len(chunksToEmbed))
		for i, chk := range chunksToEmbed {
			chunkContents[i] = store.ChunkContent{
				ChunkID:         chk.ChunkID,
				ContentMarkdown: chk.ContentMarkdown,
			}
		}
		if err := w.contentStore.PutContent(ctx, chunkContents); err != nil {
			jobObj.SetError(err)
			return jobObj, err
		}
	}

	// 6. Update progress and mark job completed
	jobObj.UpdateProgress(len(chunksToEmbed), len(diffPlan.UnchangedChunks))
	_ = jobObj.TransitionTo(indexingjob.StatusCompleted)

	return jobObj, nil
}

// extractCitationTexts flattens chunk citations into their raw reference texts for
// vector metadata, preserving the source references for retrieval and QA citation work.
func extractCitationTexts(citations []pdfmodel.Citation) []string {
	if len(citations) == 0 {
		return nil
	}
	texts := make([]string, 0, len(citations))
	for _, cit := range citations {
		if cit.RawText != "" {
			texts = append(texts, cit.RawText)
		}
	}
	if len(texts) == 0 {
		return nil
	}
	return texts
}
