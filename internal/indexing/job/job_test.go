package job_test

import (
	"testing"

	"arca/internal/indexing/job"
)

func TestIndexingJobStateMachine(t *testing.T) {
	t.Run("creates new indexing job in Pending status", func(t *testing.T) {
		j := job.NewIndexingJob("job-123", "doc-456", 100)
		if j.Status != job.StatusPending {
			t.Errorf("expected status Pending, got %s", j.Status)
		}
		if j.TotalChunks != 100 {
			t.Errorf("expected TotalChunks 100, got %d", j.TotalChunks)
		}
	})

	t.Run("valid state transitions succeed", func(t *testing.T) {
		j := job.NewIndexingJob("job-123", "doc-456", 100)

		if err := j.TransitionTo(job.StatusRunning); err != nil {
			t.Fatalf("unexpected error transitioning to Running: %v", err)
		}
		if j.Status != job.StatusRunning {
			t.Errorf("expected status Running, got %s", j.Status)
		}

		if err := j.TransitionTo(job.StatusCompleted); err != nil {
			t.Fatalf("unexpected error transitioning to Completed: %v", err)
		}
		if j.Status != job.StatusCompleted {
			t.Errorf("expected status Completed, got %s", j.Status)
		}
	})

	t.Run("invalid state transitions fail with error", func(t *testing.T) {
		j := job.NewIndexingJob("job-123", "doc-456", 100)
		_ = j.TransitionTo(job.StatusRunning)
		_ = j.TransitionTo(job.StatusCompleted)

		// Invalid: Completed -> Running
		if err := j.TransitionTo(job.StatusRunning); err == nil {
			t.Error("expected error transitioning from Completed to Running, got nil")
		}
	})

	t.Run("progress calculation correctly reflects indexed and skipped counts", func(t *testing.T) {
		j := job.NewIndexingJob("job-123", "doc-456", 100)
		j.UpdateProgress(40, 10)

		if j.IndexedChunks != 40 {
			t.Errorf("expected IndexedChunks 40, got %d", j.IndexedChunks)
		}
		if j.SkippedChunks != 10 {
			t.Errorf("expected SkippedChunks 10, got %d", j.SkippedChunks)
		}
		if j.ProgressPercentage() != 50.0 {
			t.Errorf("expected 50.0%% progress, got %.2f%%", j.ProgressPercentage())
		}
	})
}
