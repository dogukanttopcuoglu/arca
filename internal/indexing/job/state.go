package job

import (
	"fmt"
)

// IndexingStatus represents the formal state machine status for an IndexingJob.
type IndexingStatus string

const (
	StatusPending   IndexingStatus = "Pending"
	StatusRunning   IndexingStatus = "Running"
	StatusCompleted IndexingStatus = "Completed"
	StatusFailed    IndexingStatus = "Failed"
	StatusRetrying  IndexingStatus = "Retrying"
	StatusCancelled IndexingStatus = "Cancelled"
)

// IsValid checks if the status string is a recognized IndexingStatus.
func (s IndexingStatus) IsValid() bool {
	switch s {
	case StatusPending, StatusRunning, StatusCompleted, StatusFailed, StatusRetrying, StatusCancelled:
		return true
	default:
		return false
	}
}

// CanTransitionTo enforces valid state machine transition rules.
func CanTransitionTo(from, to IndexingStatus) error {
	if !from.IsValid() {
		return fmt.Errorf("invalid current status: %q", from)
	}
	if !to.IsValid() {
		return fmt.Errorf("invalid target status: %q", to)
	}

	if from == to {
		return nil
	}

	switch from {
	case StatusPending:
		if to == StatusRunning || to == StatusCancelled {
			return nil
		}
	case StatusRunning:
		if to == StatusCompleted || to == StatusFailed || to == StatusRetrying || to == StatusCancelled {
			return nil
		}
	case StatusRetrying:
		if to == StatusRunning || to == StatusFailed || to == StatusCancelled {
			return nil
		}
	case StatusCompleted, StatusFailed, StatusCancelled:
		// Terminal states cannot transition to non-terminal active states
		return fmt.Errorf("cannot transition from terminal status %q to %q", from, to)
	}

	return fmt.Errorf("invalid status transition from %q to %q", from, to)
}
