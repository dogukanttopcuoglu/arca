package job

import (
	"fmt"
)

// QAJobStatus represents the formal state machine status for an asynchronous QAJob.
type QAJobStatus string

const (
	QAStatusPending    QAJobStatus = "Pending"
	QAStatusPlanning   QAJobStatus = "Planning"
	QAStatusRetrieving QAJobStatus = "Retrieving"
	QAStatusGenerating QAJobStatus = "Generating"
	QAStatusVerifying  QAJobStatus = "Verifying"
	QAStatusCompleted  QAJobStatus = "Completed"
	QAStatusFailed     QAJobStatus = "Failed"
	QAStatusCancelled  QAJobStatus = "Cancelled"
)

// IsValid checks if status is a recognized QAJobStatus.
func (s QAJobStatus) IsValid() bool {
	switch s {
	case QAStatusPending, QAStatusPlanning, QAStatusRetrieving, QAStatusGenerating, QAStatusVerifying, QAStatusCompleted, QAStatusFailed, QAStatusCancelled:
		return true
	default:
		return false
	}
}

// CanTransitionTo enforces valid state machine transitions for QAJob.
func CanTransitionTo(from, to QAJobStatus) error {
	if !from.IsValid() {
		return fmt.Errorf("invalid current QA status: %q", from)
	}
	if !to.IsValid() {
		return fmt.Errorf("invalid target QA status: %q", to)
	}

	if from == to {
		return nil
	}

	switch from {
	case QAStatusPending:
		if to == QAStatusPlanning || to == QAStatusCancelled {
			return nil
		}
	case QAStatusPlanning:
		if to == QAStatusRetrieving || to == QAStatusFailed || to == QAStatusCancelled {
			return nil
		}
	case QAStatusRetrieving:
		if to == QAStatusGenerating || to == QAStatusFailed || to == QAStatusCancelled {
			return nil
		}
	case QAStatusGenerating:
		if to == QAStatusVerifying || to == QAStatusFailed || to == QAStatusCancelled {
			return nil
		}
	case QAStatusVerifying:
		if to == QAStatusCompleted || to == QAStatusFailed || to == QAStatusCancelled {
			return nil
		}
	case QAStatusCompleted, QAStatusFailed, QAStatusCancelled:
		return fmt.Errorf("cannot transition from terminal QA status %q to %q", from, to)
	}

	return fmt.Errorf("invalid QA status transition from %q to %q", from, to)
}
