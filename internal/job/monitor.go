package job

import (
	"context"
	"time"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type Monitor struct {
	k8sClient kubernetes.Interface
	logger    *zap.SugaredLogger
}

type JobStatus struct {
	Name      string
	Namespace string
	Succeeded bool
	Failed    bool
	Active    bool
	Message   string
}

func NewMonitor(k8sClient kubernetes.Interface, logger *zap.SugaredLogger) *Monitor {
	return &Monitor{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

// PollJobStatus periodically checks job status
func (m *Monitor) PollJobStatus(ctx context.Context, namespace, jobName string, interval time.Duration) (*JobStatus, error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()

		case <-ticker.C:
			job, err := m.k8sClient.BatchV1().Jobs(namespace).Get(ctx, jobName, metav1.GetOptions{})
			if err != nil {
				m.logger.Warnw("Failed to get job status",
					"job_name", jobName,
					"error", err,
				)
				continue
			}

			status := &JobStatus{
				Name:      job.Name,
				Namespace: job.Namespace,
				Succeeded: job.Status.Succeeded > 0,
				Failed:    job.Status.Failed > 0,
				Active:    job.Status.Active > 0,
			}

			if job.Status.Succeeded > 0 {
				status.Message = "Job completed successfully"
				return status, nil
			}

			if job.Status.Failed > 0 {
				status.Message = "Job failed"
				return status, nil
			}

			if job.Status.Active == 0 && job.Status.Succeeded == 0 && job.Status.Failed == 0 {
				status.Message = "Job is pending"
			} else {
				status.Message = "Job is running"
			}

			m.logger.Debugw("Job status",
				"job_name", jobName,
				"message", status.Message,
				"active", job.Status.Active,
			)

			return status, nil
		}
	}
}
