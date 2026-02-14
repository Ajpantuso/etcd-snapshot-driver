package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func SetupViper(cmd *cobra.Command) (*viper.Viper, error) {
	cmd.PersistentFlags().StringVar(&cfgFile, "config", ".etcd-snapshot-driver.yaml", "config file")

	flags := cmd.Flags()

	// CSI Configuration
	flags.String("csi-endpoint", "unix:///var/lib/kubelet/plugins/etcd-snapshot-driver/csi.sock", "CSI endpoint socket location")
	flags.String("driver-name", "etcd-snapshot-driver", "CSI driver name")

	// ETCD Configuration
	flags.String("etcd-namespace", "etcd", "Default namespace for ETCD resources")
	flags.String("cluster-label-key", "etcd.io/cluster", "Label key used to identify ETCD cluster membership")

	// Snapshot Configuration
	flags.Duration("snapshot-timeout", 5*time.Minute, "Timeout for snapshot operations")
	flags.Int32("job-backoff-limit", 3, "Kubernetes job backoff limit for snapshot jobs")
	flags.Int64("job-active-deadline", 600, "Kubernetes job active deadline in seconds")
	flags.String("default-storage-class", "standard", "Default storage class for snapshots")
	flags.String("snapshot-pvc-size", "10Gi", "Size of the dedicated snapshot PVC")

	// ETCD TLS Configuration
	flags.Bool("etcd-tls-enabled", true, "Enable TLS authentication for ETCD")
	flags.String("etcd-tls-secret-name", "etcd-client-tls", "Kubernetes secret name containing ETCD TLS certificates")
	flags.String("etcd-tls-secret-namespace", "", "Namespace for TLS secret (empty uses PVC namespace)")
	flags.String("etcd-client-cert-path", "/etc/etcd/tls/client/etcd-client.crt", "Path to ETCD client certificate in job pods")
	flags.String("etcd-client-key-path", "/etc/etcd/tls/client/etcd-client.key", "Path to ETCD client key in job pods")
	flags.String("etcd-ca-path", "/etc/etcd/tls/etcd-ca/ca.crt", "Path to ETCD CA certificate in job pods")

	// Container Images
	flags.String("etcd-image", "quay.io/coreos/etcd:v3.5.0", "ETCD container image for snapshot jobs")
	flags.String("busybox-image", "busybox:1.35", "Busybox container image for cleanup jobs")
	// Observability
	flags.String("metrics-bind-address", ":8080", "Address for metrics and health endpoints")
	flags.String("log-level", "info", "Log level (debug, info, warn, error)")
	flags.String("log-format", "json", "Log format (json, console)")

	// High Availability
	flags.Bool("leader-elect", false, "Enable leader election for high availability")

	viper := viper.New()

	if err := viper.BindPFlags(flags); err != nil {
		return nil, fmt.Errorf("binding flags: %w", err)
	}

	return viper, nil
}

func LoadOptions(viper *viper.Viper) {
	viper.SetConfigType("yaml")
	viper.SetConfigFile(cfgFile)

	// Set up automatic environment variable binding for all config keys
	// The EnvKeyReplacer converts kebab-case to SCREAMING_SNAKE_CASE
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	// Try to read config file if it exists, but don't fail if it doesn't
	if err := viper.ReadInConfig(); err == nil {
		viper.WatchConfig()
	}
}
