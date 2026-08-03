package diff

import (
	pdfmodel "arca/internal/pdfinspector/model"
)

// DiffAction represents the classified action state for a chunk during differential re-indexing.
type DiffAction string

const (
	ActionUnchanged      DiffAction = "UNCHANGED"
	ActionContentChanged DiffAction = "CONTENT_CHANGED"
	ActionModelChanged   DiffAction = "MODEL_CHANGED"
	ActionSchemaChanged  DiffAction = "SCHEMA_CHANGED"
	ActionNew            DiffAction = "NEW"
	ActionDeleted        DiffAction = "DELETED"
)

// DiffPlan encapsulates the operational tasks produced by DiffEngine for an IndexingJob.
type DiffPlan struct {
	DocumentID      string                   `json:"document_id"`
	UnchangedChunks []pdfmodel.KnowledgeChunk `json:"unchanged_chunks"`
	ModifiedChunks  []pdfmodel.KnowledgeChunk `json:"modified_chunks"`
	NewChunks       []pdfmodel.KnowledgeChunk `json:"new_chunks"`
	DeletedPointIDs []string                 `json:"deleted_point_ids"`
	DeletedChunkIDs []string                 `json:"deleted_chunk_ids"`
}

// ChunksToEmbed returns all new and modified chunks requiring LLM embedding API calls.
func (p *DiffPlan) ChunksToEmbed() []pdfmodel.KnowledgeChunk {
	res := make([]pdfmodel.KnowledgeChunk, 0, len(p.NewChunks)+len(p.ModifiedChunks))
	res = append(res, p.NewChunks...)
	res = append(res, p.ModifiedChunks...)
	return res
}
