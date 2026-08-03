package diff

import (
	indexingmodel "arca/internal/indexing/model"
	indexingstore "arca/internal/indexing/store"
	pdfmodel "arca/internal/pdfinspector/model"
)

// Engine evaluates document chunk signatures against existing vector point metadata to produce a DiffPlan.
type Engine struct {
	provider     string
	modelName    string
	version      string
	schemaVersion string
}

// NewEngine constructs a DiffEngine with current indexing parameters.
func NewEngine(provider, modelName, version, schemaVersion string) *Engine {
	if provider == "" {
		provider = "OpenAI"
	}
	if modelName == "" {
		modelName = "text-embedding-3-large"
	}
	if version == "" {
		version = "v1"
	}
	if schemaVersion == "" {
		schemaVersion = "1.0"
	}
	return &Engine{
		provider:      provider,
		modelName:     modelName,
		version:       version,
		schemaVersion: schemaVersion,
	}
}

// ComputeDiffPlan calculates differential actions across incoming chunks and existing vector points.
func (e *Engine) ComputeDiffPlan(documentID string, chunks []pdfmodel.KnowledgeChunk, existingPoints []indexingmodel.VectorMetadata) *DiffPlan {
	plan := &DiffPlan{
		DocumentID:      documentID,
		UnchangedChunks: []pdfmodel.KnowledgeChunk{},
		ModifiedChunks:  []pdfmodel.KnowledgeChunk{},
		NewChunks:       []pdfmodel.KnowledgeChunk{},
		DeletedPointIDs: []string{},
		DeletedChunkIDs: []string{},
	}

	// Index existing points by stable Point ID
	existingMap := make(map[string]indexingmodel.VectorMetadata)
	matchedPointIDs := make(map[string]bool)

	for _, pt := range existingPoints {
		if pt.DocumentID == documentID {
			// Compute expected Point ID for lookup matching
			ptID := indexingstore.CalculatePointID(pt.DocumentID, pt.SectionPath, extractOrderFromMetadata(pt))
			if ptID != "" {
				existingMap[ptID] = pt
			}
		}
	}

	for _, chk := range chunks {
		pointID := indexingstore.CalculatePointID(documentID, chk.SectionPath, chk.ChunkOrder)
		expectedSig := indexingmodel.CalculateIndexSignature(
			chk.ContentHash,
			e.provider,
			e.modelName,
			e.version,
			e.schemaVersion,
		)

		if existingMeta, exists := existingMap[pointID]; exists {
			matchedPointIDs[pointID] = true

			if existingMeta.IndexSignature == expectedSig {
				plan.UnchangedChunks = append(plan.UnchangedChunks, chk)
			} else {
				plan.ModifiedChunks = append(plan.ModifiedChunks, chk)
			}
		} else {
			plan.NewChunks = append(plan.NewChunks, chk)
		}
	}

	// Any existing point for this document that was not matched in the new chunk layout is marked for DELETED
	for _, pt := range existingPoints {
		if pt.DocumentID == documentID {
			ptID := indexingstore.CalculatePointID(pt.DocumentID, pt.SectionPath, extractOrderFromMetadata(pt))
			if !matchedPointIDs[ptID] {
				plan.DeletedPointIDs = append(plan.DeletedPointIDs, ptID)
				plan.DeletedChunkIDs = append(plan.DeletedChunkIDs, pt.ChunkID)
			}
		}
	}

	return plan
}

func extractOrderFromMetadata(meta indexingmodel.VectorMetadata) int {
	return meta.ChunkOrder
}
