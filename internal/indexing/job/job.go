package job

import (
	"sync"
	"time"
)

// IndexingJob models an asynchronous vector indexing task with progress metrics.
type IndexingJob struct {
	mu               sync.RWMutex
	JobID            string         `json:"job_id"`
	DocumentID       string         `json:"document_id"`
	Status           IndexingStatus `json:"status"`
	TotalChunks      int            `json:"total_chunks"`
	IndexedChunks    int            `json:"indexed_chunks"`
	SkippedChunks    int            `json:"skipped_chunks"`
	ErrorSummary     string         `json:"error_summary,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	CompletedAt      *time.Time     `json:"completed_at,omitempty"`
	EmbeddingModel   string         `json:"embedding_model,omitempty"`
	EmbeddingProvider string        `json:"embedding_provider,omitempty"`
}

// NewIndexingJob constructs a new IndexingJob in Pending status.
func NewIndexingJob(jobID, documentID string, totalChunks int) *IndexingJob {
	now := time.Now()
	if totalChunks < 0 {
		totalChunks = 0
	}
	return &IndexingJob{
		JobID:       jobID,
		DocumentID:  documentID,
		Status:      StatusPending,
		TotalChunks: totalChunks,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// TransitionTo updates job status if transition is valid.
func (j *IndexingJob) TransitionTo(target IndexingStatus) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	if err := CanTransitionTo(j.Status, target); err != nil {
		return err
	}

	j.Status = target
	j.UpdatedAt = time.Now()
	if target == StatusCompleted || target == StatusFailed || target == StatusCancelled {
		now := time.Now()
		j.CompletedAt = &now
	}
	return nil
}

// UpdateProgress safely updates indexed and skipped chunk counters.
func (j *IndexingJob) UpdateProgress(indexed, skipped int) {
	j.mu.Lock()
	defer j.mu.Unlock()

	j.IndexedChunks = indexed
	j.SkippedChunks = skipped
	j.UpdatedAt = time.Now()
}

// ProgressPercentage calculates progress completion percentage [0.0 - 100.0].
func (j *IndexingJob) ProgressPercentage() float64 {
	j.mu.RLock()
	defer j.mu.RUnlock()

	if j.TotalChunks <= 0 {
		return 100.0
	}

	processed := j.IndexedChunks + j.SkippedChunks
	pct := (float64(processed) / float64(j.TotalChunks)) * 100.0
	if pct > 100.0 {
		pct = 100.0
	}
	return pct
}

// SetError records an error summary and sets status to Failed.
func (j *IndexingJob) SetError(err error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	if err != nil {
		j.ErrorSummary = err.Error()
	}
	_ = CanTransitionTo(j.Status, StatusFailed)
	j.Status = StatusFailed
	now := time.Now()
	j.UpdatedAt = now
	j.CompletedAt = &now
}
