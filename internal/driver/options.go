package driver

import (
	"time"

	"go.uber.org/zap"
)

type WithSnapShotTimeout time.Duration

func (w WithSnapShotTimeout) ConfigureController(c *ControllerConfig) {
	c.SnapshotTimeout = time.Duration(w)
}

type WithClusterLabelKey string

func (w WithClusterLabelKey) ConfigureController(c *ControllerConfig) {
	c.ClusterLabelKey = string(w)
}

type WithJobBackoffLimit int32

func (w WithJobBackoffLimit) ConfigureController(c *ControllerConfig) {
	c.JobBackoffLimit = int32(w)
}

type WithJobActiveDeadlineSeconds int64

func (w WithJobActiveDeadlineSeconds) ConfigureController(c *ControllerConfig) {
	c.JobActiveDeadlineSeconds = int64(w)
}

type WithETCDImage string

func (w WithETCDImage) ConfigureController(c *ControllerConfig) {
	c.ETCDImage = string(w)
}

type WithBusyboxImage string

func (w WithBusyboxImage) ConfigureController(c *ControllerConfig) {
	c.BusyboxImage = string(w)
}

type WithETCDTLSEnabled bool

func (w WithETCDTLSEnabled) ConfigureController(c *ControllerConfig) {
	c.ETCDTLSEnabled = bool(w)
}

type WithETCDTLSSecretName string

func (w WithETCDTLSSecretName) ConfigureController(c *ControllerConfig) {
	c.ETCDTLSSecretName = string(w)
}

type WithETCDTLSSecretNamespace string

func (w WithETCDTLSSecretNamespace) ConfigureController(c *ControllerConfig) {
	c.ETCDTLSSecretNamespace = string(w)
}

type WithETCDClientCertPath string

func (w WithETCDClientCertPath) ConfigureController(c *ControllerConfig) {
	c.ETCDClientCertPath = string(w)
}

type WithETCDClientKeyPath string

func (w WithETCDClientKeyPath) ConfigureController(c *ControllerConfig) {
	c.ETCDClientKeyPath = string(w)
}

type WithETCDCAPath string

func (w WithETCDCAPath) ConfigureController(c *ControllerConfig) {
	c.ETCDCAPath = string(w)
}

type WithDefaultStorageClass string

func (w WithDefaultStorageClass) ConfigureController(c *ControllerConfig) {
	c.DefaultStorageClass = string(w)
}

type WithSnapshotPVCSize string

func (w WithSnapshotPVCSize) ConfigureController(c *ControllerConfig) {
	c.SnapshotPVCSize = string(w)
}

type WithLogger struct {
	Logger *zap.SugaredLogger
}

func (w WithLogger) ConfigureDriver(c *DriverConfig) {
	c.Logger = w.Logger
}
func (w WithLogger) ConfigureIdentityServer(c *IdentityServerConfig) {
	c.Logger = w.Logger
}
func (w WithLogger) ConfigureController(c *ControllerConfig) {
	c.Logger = w.Logger
}

type WithEndPoint string

func (w WithEndPoint) ConfigureDriver(c *DriverConfig) {
	c.EndPoint = string(w)
}
