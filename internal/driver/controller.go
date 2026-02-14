package driver

import (
	"fmt"
	"time"

	"go.uber.org/zap"
)

// parseVolumeID extracts namespace and PVC name from volume ID
// Expected format: "namespace/pvc-name"
func parseVolumeID(volumeID string) (namespace, pvcName string, err error) {
	parts := len(volumeID)
	if parts == 0 {
		return "", "", fmt.Errorf("empty volume ID")
	}

	// Find the last "/" to split namespace and name
	var i int
	for i = len(volumeID) - 1; i >= 0; i-- {
		if volumeID[i] == '/' {
			break
		}
	}

	if i < 0 {
		// No "/" found, assume it's just the PVC name in current namespace
		return "default", volumeID, nil
	}

	namespace = volumeID[:i]
	pvcName = volumeID[i+1:]

	if namespace == "" || pvcName == "" {
		return "", "", fmt.Errorf("invalid volume ID format: %s", volumeID)
	}

	return namespace, pvcName, nil
}

type ControllerConfig struct {
	Logger 				 *zap.SugaredLogger
	SnapshotTimeout          time.Duration
	ClusterLabelKey          string
	JobBackoffLimit          int32
	JobActiveDeadlineSeconds int64
	ETCDImage                string
	BusyboxImage             string
	ETCDTLSEnabled           bool
	ETCDTLSSecretName        string
	ETCDTLSSecretNamespace   string
	ETCDClientCertPath       string
	ETCDClientKeyPath        string
	ETCDCAPath               string
	DefaultStorageClass      string
	SnapshotPVCSize          string
}

func (c *ControllerConfig) Options(opts ...ControllerOption) {
	for _, opt := range opts {
		opt.ConfigureController(c)
	}
}

func (c *ControllerConfig) Default() {
	if c.Logger == nil {
		c.Logger = zap.NewNop().Sugar()
	}
}

type ControllerOption interface {
	ConfigureController(*ControllerConfig)
}
