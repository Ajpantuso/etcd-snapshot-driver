package health

import (
	"context"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type HealthChecker struct {
	k8sClient kubernetes.Interface
	logger    *zap.SugaredLogger
	mu        sync.RWMutex
	ready     bool
}

func NewHealthChecker(k8sClient kubernetes.Interface, logger *zap.SugaredLogger) *HealthChecker {
	return &HealthChecker{
		k8sClient: k8sClient,
		logger:    logger,
		ready:     false,
	}
}

// SetReady marks the driver as ready
func (h *HealthChecker) SetReady(ready bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ready = ready
}

// Liveness checks if the driver process is alive
func (h *HealthChecker) Liveness(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// Readiness checks if the driver is ready to serve requests
func (h *HealthChecker) Readiness(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if !h.ready {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("Not ready"))
		return
	}

	// Try to list namespaces as a basic API server check
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	_, err := h.k8sClient.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		h.logger.Warnw("Readiness check failed: API server not accessible", "error", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("API server not accessible"))
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Ready"))
}

const (
	timeout = 5 * time.Second
)
