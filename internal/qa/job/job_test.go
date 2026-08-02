package job_test

import (
	"testing"

	qajob "arca/internal/qa/job"
)

func TestQAJobStateMachine(t *testing.T) {
	t.Run("creates new QAJob in Pending status", func(t *testing.T) {
		j := qajob.NewQAJob("job-qa-1", "What is creativity?")
		if j.Status != qajob.QAStatusPending {
			t.Errorf("expected status Pending, got %s", j.Status)
		}
		if j.QueryText != "What is creativity?" {
			t.Errorf("expected query matching input, got %q", j.QueryText)
		}
	})

	t.Run("valid QAJob state transitions succeed", func(t *testing.T) {
		j := qajob.NewQAJob("job-qa-1", "Query")

		if err := j.TransitionTo(qajob.QAStatusPlanning); err != nil {
			t.Fatalf("unexpected error transitioning to Planning: %v", err)
		}
		if err := j.TransitionTo(qajob.QAStatusRetrieving); err != nil {
			t.Fatalf("unexpected error transitioning to Retrieving: %v", err)
		}
		if err := j.TransitionTo(qajob.QAStatusGenerating); err != nil {
			t.Fatalf("unexpected error transitioning to Generating: %v", err)
		}
		if err := j.TransitionTo(qajob.QAStatusVerifying); err != nil {
			t.Fatalf("unexpected error transitioning to Verifying: %v", err)
		}
		if err := j.TransitionTo(qajob.QAStatusCompleted); err != nil {
			t.Fatalf("unexpected error transitioning to Completed: %v", err)
		}
	})

	t.Run("invalid QAJob state transitions fail with error", func(t *testing.T) {
		j := qajob.NewQAJob("job-qa-1", "Query")
		_ = j.TransitionTo(qajob.QAStatusPlanning)
		_ = j.TransitionTo(qajob.QAStatusRetrieving)
		_ = j.TransitionTo(qajob.QAStatusGenerating)
		_ = j.TransitionTo(qajob.QAStatusVerifying)
		_ = j.TransitionTo(qajob.QAStatusCompleted)

		if err := j.TransitionTo(qajob.QAStatusRetrieving); err == nil {
			t.Error("expected error transitioning from Completed to Retrieving, got nil")
		}
	})
}
