package util

import "fmt"

// SnapshotNotFoundError is returned when a snapshot cannot be found
type SnapshotNotFoundError struct {
	SnapshotID string
}

func (e *SnapshotNotFoundError) Error() string {
	return fmt.Sprintf("snapshot not found: %s", e.SnapshotID)
}

// ETCDDiscoveryError is returned when ETCD cluster discovery fails
type ETCDDiscoveryError struct {
	Reason string
}

func (e *ETCDDiscoveryError) Error() string {
	return fmt.Sprintf("etcd discovery failed: %s", e.Reason)
}

// JobExecutionError is returned when a Job execution fails
type JobExecutionError struct {
	JobName string
	Reason  string
}

func (e *JobExecutionError) Error() string {
	return fmt.Sprintf("job execution failed: %s (%s)", e.JobName, e.Reason)
}
