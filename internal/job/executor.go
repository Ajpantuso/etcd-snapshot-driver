package job

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type Executor struct {
	k8sClient kubernetes.Interface
	logger    *zap.SugaredLogger
}

type JobResult struct {
	Success      bool
	SnapshotID   string
	SnapshotSize int64
	Duration     time.Duration
	ErrorMessage string
}

func NewExecutor(k8sClient kubernetes.Interface, logger *zap.SugaredLogger) *Executor {
	return &Executor{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

// ExecuteSnapshotJob creates a Job and waits for completion
func (e *Executor) ExecuteSnapshotJob(ctx context.Context, job *batchv1.Job, timeout time.Duration) (*JobResult, error) {
	snapshotID := job.Labels["snapshot-id"]
	operation := job.Labels["operation"]

	e.logger.Infow("Executing snapshot job",
		"job_name", job.Name,
		"operation", operation,
		"snapshot_id", snapshotID,
	)

	startTime := time.Now()

	// Create the job
	createdJob, err := e.k8sClient.BatchV1().Jobs(job.Namespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			e.logger.Debugw("Job already exists", "job_name", job.Name)
			createdJob = job
		} else {
			return &JobResult{
				Success:    false,
				SnapshotID: snapshotID,
				ErrorMessage: fmt.Sprintf("failed to create job: %v", err),
			}, err
		}
	}

	// Wait for job completion with timeout
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := e.waitForJobCompletion(ctx, createdJob)
	if err != nil {
		result.Duration = time.Since(startTime)
		if err != context.DeadlineExceeded {
			// Try to get logs for debugging
			if logs, logErr := e.getJobLogs(context.Background(), createdJob); logErr == nil {
				e.logger.Warnw("Job failed, logs:", "logs", logs)
			}
		}
		return result, err
	}

	result.Duration = time.Since(startTime)
	e.logger.Infow("Snapshot job completed",
		"snapshot_id", snapshotID,
		"operation", operation,
		"duration", result.Duration.String(),
	)

	return result, nil
}

// waitForJobCompletion polls the job until completion
func (e *Executor) waitForJobCompletion(ctx context.Context, job *batchv1.Job) (*JobResult, error) {
	snapshotID := job.Labels["snapshot-id"]
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return &JobResult{
				Success:      false,
				SnapshotID:   snapshotID,
				ErrorMessage: "job execution timeout",
			}, ctx.Err()

		case <-ticker.C:
			// Get updated job status
			updatedJob, err := e.k8sClient.BatchV1().Jobs(job.Namespace).Get(ctx, job.Name, metav1.GetOptions{})
			if err != nil {
				e.logger.Warnw("Failed to get job status",
					"job_name", job.Name,
					"error", err,
				)
				continue
			}

			// Check job status
			if updatedJob.Status.Succeeded > 0 {
				e.logger.Infow("Job succeeded",
					"job_name", job.Name,
					"snapshot_id", snapshotID,
				)
				return &JobResult{
					Success:    true,
					SnapshotID: snapshotID,
				}, nil
			}

			if updatedJob.Status.Failed > 0 {
				msg := fmt.Sprintf("job failed after %d attempts", updatedJob.Status.Failed)
				e.logger.Errorw("Job failed",
					"job_name", job.Name,
					"snapshot_id", snapshotID,
					"failed_count", updatedJob.Status.Failed,
				)
				return &JobResult{
					Success:      false,
					SnapshotID:   snapshotID,
					ErrorMessage: msg,
				}, fmt.Errorf("%s", msg)
			}

			// Still running
			e.logger.Debugw("Job still running",
				"job_name", job.Name,
				"active_pods", updatedJob.Status.Active,
			)
		}
	}
}

// getJobLogs retrieves logs from the job's pod for debugging
func (e *Executor) getJobLogs(ctx context.Context, job *batchv1.Job) (string, error) {
	pods, err := e.k8sClient.CoreV1().Pods(job.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("job-name=%s", job.Name),
	})
	if err != nil || len(pods.Items) == 0 {
		return "", err
	}

	pod := pods.Items[0]
	if len(pod.Spec.Containers) == 0 {
		return "", fmt.Errorf("pod has no containers")
	}

	logReq := e.k8sClient.CoreV1().Pods(job.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{})
	logs, err := logReq.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer logs.Close()

	buf := make([]byte, 4096)
	n, _ := logs.Read(buf)
	return string(buf[:n]), nil
}
