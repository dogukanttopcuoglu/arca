package job

import (
	"sync"
	"time"

	qaverification "arca/internal/qa/verification"
)

// QAJob models an asynchronous deep research RAG task.
type QAJob struct {
	mu           sync.RWMutex
	JobID        string                       `json:"job_id"`
	QueryText    string                       `json:"query_text"`
	Status       QAJobStatus                  `json:"status"`
	Answer       *qaverification.VerifiedAnswer `json:"answer,omitempty"`
	ErrorSummary string                       `json:"error_summary,omitempty"`
	CreatedAt    time.Time                    `json:"created_at"`
	UpdatedAt    time.Time                    `json:"updated_at"`
	CompletedAt  *time.Time                   `json:"completed_at,omitempty"`
}

// NewQAJob constructs a new QAJob in Pending status.
func NewQAJob(jobID, query string) *QAJob {
	now := time.Now()
	return &QAJob{
		JobID:     jobID,
		QueryText: query,
		Status:    QAStatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// TransitionTo safely updates status if transition is valid.
func (j *QAJob) TransitionTo(target QAJobStatus) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	if err := CanTransitionTo(j.Status, target); err != nil {
		return err
	}

	j.Status = target
	j.UpdatedAt = time.Now()
	if target == QAStatusCompleted || target == QAStatusFailed || target == QAStatusCancelled {
		now := time.Now()
		j.CompletedAt = &now
	}
	return nil
}

// SetAnswer sets the verified answer output and transitions status to Completed.
func (j *QAJob) SetAnswer(answer *qaverification.VerifiedAnswer) {
	j.mu.Lock()
	defer j.mu.Unlock()

	j.Answer = answer
	j.Status = QAStatusCompleted
	now := time.Now()
	j.UpdatedAt = now
	j.CompletedAt = &now
}

// SetError records error summary and sets status to Failed.
func (j *QAJob) SetError(err error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	if err != nil {
		j.ErrorSummary = err.Error()
	}
	j.Status = QAStatusFailed
	now := time.Now()
	j.UpdatedAt = now
	j.CompletedAt = &now
}
