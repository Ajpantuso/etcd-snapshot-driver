package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
	// Snapshot operations
	SnapshotOpsTotal    *prometheus.CounterVec
	SnapshotOpsDuration *prometheus.HistogramVec
	SnapshotSize        *prometheus.GaugeVec
	SnapshotsTotal      *prometheus.GaugeVec

	// RPC calls
	CSIRPCTotal   *prometheus.CounterVec
	CSIRPCLatency *prometheus.HistogramVec
	CSIRPCActive  *prometheus.GaugeVec

	// Storage
	SnapshotPVCUsage      *prometheus.GaugeVec
	SnapshotPVCAvailable  *prometheus.GaugeVec
	StorageErrors         *prometheus.CounterVec

	// ETCD health
	ETCDMembers   *prometheus.GaugeVec
	ETCDHasQuorum *prometheus.GaugeVec
}

func NewMetrics() *Metrics {
	return &Metrics{
		// Snapshot operations
		SnapshotOpsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "etcd_snapshot_operations_total",
				Help: "Total number of snapshot operations",
			},
			[]string{"operation", "status", "cluster"},
		),
		SnapshotOpsDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "etcd_snapshot_operation_duration_seconds",
				Help:    "Duration of snapshot operations",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"operation"},
		),
		SnapshotSize: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "etcd_snapshot_size_bytes",
				Help: "Size of snapshots in bytes",
			},
			[]string{"snapshot_id", "cluster"},
		),
		SnapshotsTotal: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "etcd_snapshots_total",
				Help: "Total number of snapshots",
			},
			[]string{"cluster", "status"},
		),

		// RPC calls
		CSIRPCTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "csi_rpc_total",
				Help: "Total number of CSI RPC calls",
			},
			[]string{"method", "status"},
		),
		CSIRPCLatency: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "csi_rpc_latency_seconds",
				Help:    "Latency of CSI RPC calls",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"method"},
		),
		CSIRPCActive: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "csi_rpc_active",
				Help: "Number of active CSI RPC calls",
			},
			[]string{"method"},
		),

		// Storage
		SnapshotPVCUsage: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "etcd_snapshot_pvc_usage_bytes",
				Help: "PVC usage for snapshots",
			},
			[]string{"pvc_name", "namespace"},
		),
		SnapshotPVCAvailable: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "etcd_snapshot_pvc_available_bytes",
				Help: "PVC available space for snapshots",
			},
			[]string{"pvc_name", "namespace"},
		),
		StorageErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "etcd_snapshot_storage_errors_total",
				Help: "Total number of storage errors",
			},
			[]string{"reason"},
		),

		// ETCD health
		ETCDMembers: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "etcd_cluster_members",
				Help: "Number of ETCD cluster members",
			},
			[]string{"cluster", "state"},
		),
		ETCDHasQuorum: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "etcd_cluster_has_quorum",
				Help: "Whether ETCD cluster has quorum",
			},
			[]string{"cluster"},
		),
	}
}
