package main

import (
	"fmt"
	"net/http"

	"os"

	"github.com/Ajpantuso/etcd-snapshot-driver/internal/config"
	"github.com/Ajpantuso/etcd-snapshot-driver/internal/driver"
	"github.com/Ajpantuso/etcd-snapshot-driver/internal/health"
	"github.com/Ajpantuso/etcd-snapshot-driver/internal/metrics"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	cfgFile string
	logger  *zap.SugaredLogger
)

func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "etcd-snapshot-driver",
		Short: "CSI driver for ETCD cluster snapshots",
		Long: `etcd-snapshot-driver is a Kubernetes CSI driver that enables
	snapshot operations for ETCD clusters, particularly optimized
	for HyperShift managed ETCD deployments.`,
	}
	cmd.AddCommand(newVersionCommand())

	viper, err := SetupViper(cmd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "setting up configuration: %v\n", err)
		os.Exit(1)
	}

	cmd.PersistentPreRunE = initializeLogging(viper)
	cmd.RunE = run(viper)

	return cmd
}

// initializeLogging sets up the logger based on config
func initializeLogging(viper *viper.Viper) func(*cobra.Command, []string) error {
	return func(c *cobra.Command, s []string) error {
		level := viper.GetString("log-level")
		format := viper.GetString("log-format")

		var config zap.Config
		if format == "console" {
			config = zap.NewDevelopmentConfig()
		} else {
			config = zap.NewProductionConfig()
		}

		config.Level = zap.NewAtomicLevelAt(parseLogLevel(level))
		config.OutputPaths = []string{"stdout"}
		config.ErrorOutputPaths = []string{"stderr"}

		baseLogger, err := config.Build()
		if err != nil {
			return fmt.Errorf("failed to initialize logger: %w", err)
		}

		logger = baseLogger.Sugar()
		return nil
	}
}

func parseLogLevel(level string) zapcore.Level {
	switch level {
	case "debug":
		return zap.DebugLevel
	case "info":
		return zap.InfoLevel
	case "warn":
		return zap.WarnLevel
	case "error":
		return zap.ErrorLevel
	default:
		return zap.InfoLevel
	}
}

func run(viper *viper.Viper) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		LoadOptions(viper)

		logger.Infow("Starting etcd-snapshot-driver",
			"version", config.Version,
			"endpoint", viper.GetString("csi-endpoint"),
		)

		// Create Kubernetes client
		k8sConfig, err := rest.InClusterConfig()
		if err != nil {
			logger.Errorw("Failed to load in-cluster config", "error", err)
			return err
		}

		k8sClient, err := kubernetes.NewForConfig(k8sConfig)
		if err != nil {
			logger.Errorw("Failed to create Kubernetes client", "error", err)
			return err
		}

		logger.Infow("Kubernetes client initialized")

		// Initialize metrics and health checker
		m := metrics.NewMetrics()
		hc := health.NewHealthChecker(k8sClient, logger)

		// Start metrics and health check HTTP server
		go func() {
			mux := http.NewServeMux()
			mux.Handle("/metrics", promhttp.Handler())
			mux.HandleFunc("/healthz", hc.Liveness)
			mux.HandleFunc("/ready", hc.Readiness)

			logger.Infow("Starting metrics server", "address", viper.GetString("metrics-bind-address"))
			if err := http.ListenAndServe(viper.GetString("metrics-bind-address"), mux); err != nil {
				logger.Errorw("Metrics server error", "error", err)
			}
		}()

		// Mark as ready after initialization
		hc.SetReady(true)

		// Create and run driver
	groupControllerServer := driver.NewGroupControllerServer(k8sClient,
		driver.WithLogger{Logger: logger},
		driver.WithClusterLabelKey(viper.GetString("cluster-label-key")),
		driver.WithSnapShotTimeout(viper.GetDuration("snapshot-timeout")),
		driver.WithJobBackoffLimit(viper.GetInt32("job-backoff-limit")),
		driver.WithJobActiveDeadlineSeconds(viper.GetInt64("job-active-deadline")),
		driver.WithETCDImage(viper.GetString("etcd-image")),
		driver.WithBusyboxImage(viper.GetString("busybox-image")),
		driver.WithETCDTLSEnabled(viper.GetBool("etcd-tls-enabled")),
		driver.WithETCDTLSSecretName(viper.GetString("etcd-tls-secret-name")),
		driver.WithETCDTLSSecretNamespace(viper.GetString("etcd-tls-secret-namespace")),
		driver.WithETCDClientCertPath(viper.GetString("etcd-client-cert-path")),
		driver.WithETCDClientKeyPath(viper.GetString("etcd-client-key-path")),
		driver.WithETCDCAPath(viper.GetString("etcd-ca-path")),
		driver.WithDefaultStorageClass(viper.GetString("default-storage-class")),
		driver.WithSnapshotPVCSize(viper.GetString("snapshot-pvc-size")),
	)

			identityServer := driver.NewIdentityServer(driver.WithLogger{Logger: logger})

		driverInstance := driver.NewDriver(k8sClient, groupControllerServer, identityServer,
			driver.WithLogger{Logger: logger},
			driver.WithEndPoint(viper.GetString("csi-endpoint")),
		)

		_ = m // Keep metrics reference to avoid GC
		if err := driverInstance.Run(cmd.Context()); err != nil {
			logger.Errorw("Driver failed", "error", err)
			return err
		}

		return nil
	}
}
