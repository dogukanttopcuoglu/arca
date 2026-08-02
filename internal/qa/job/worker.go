package job

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"
)

var qaJobCounter uint64

// QAJobWorker orchestrates background deep research jobs.
type QAJobWorker struct{}

// NewQAJobWorker constructs a QAJobWorker instance.
func NewQAJobWorker() *QAJobWorker {
	return &QAJobWorker{}
}

// CreateJob instantiates and initializes a new QAJob.
func (w *QAJobWorker) CreateJob(ctx context.Context, query string) (*QAJob, error) {
	if query == "" {
		return nil, fmt.Errorf("query string cannot be empty")
	}

	jobID := fmt.Sprintf("job-qa-%d-%d", time.Now().UnixNano(), atomic.AddUint64(&qaJobCounter, 1))
	return NewQAJob(jobID, query), nil
}
