package driver

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/Ajpantuso/etcd-snapshot-driver/internal/config"
	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"k8s.io/client-go/kubernetes"
)

type Driver struct {
	version                string
	k8sClient              kubernetes.Interface
	identityServer        *IdentityServer
	groupControllerServer *GroupControllerServer
	server                *grpc.Server
	cfg                   *DriverConfig
}

func NewDriver(client kubernetes.Interface, groupControllerServer *GroupControllerServer, identityServer *IdentityServer, opts ...DriverOption) *Driver {
	var cfg DriverConfig
	cfg.Options(opts...)

	return &Driver{
		version:                config.Version,
		k8sClient:              client,
		groupControllerServer:  groupControllerServer,
		identityServer:         identityServer,
		cfg:                    &cfg,
	}
}

func (d *Driver) Run(ctx context.Context) error {
	d.cfg.Logger.Infow("Starting etcd-snapshot-driver",
		"version", d.version,
		"endpoint", d.cfg.EndPoint,
	)

	// Create gRPC server
	opts := []grpc.ServerOption{
		grpc.UnaryInterceptor(d.loggingInterceptor),
	}
	d.server = grpc.NewServer(opts...)

	// Register services
	csi.RegisterIdentityServer(d.server, d.identityServer)
	csi.RegisterGroupControllerServer(d.server, d.groupControllerServer)

	// Setup listener
	listener, err := d.setupListener()
	if err != nil {
		return fmt.Errorf("failed to setup listener: %w", err)
	}
	defer listener.Close()

	// Start gRPC server in a goroutine
	go func() {
		d.cfg.Logger.Infow("gRPC server listening",
			"address", d.cfg.EndPoint,
		)
		if err := d.server.Serve(listener); err != nil && err != grpc.ErrServerStopped {
			d.cfg.Logger.Errorw("gRPC server error", "error", err)
		}
	}()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	select {
	case <-ctx.Done():
		d.cfg.Logger.Info("Context cancelled, shutting down")
	case sig := <-sigChan:
		d.cfg.Logger.Infow("Received signal", "signal", sig)
	}

	d.cfg.Logger.Info("Gracefully shutting down gRPC server")
	d.server.GracefulStop()
	return nil
}

func (d *Driver) setupListener() (net.Listener, error) {
	if strings.HasPrefix(d.cfg.EndPoint, "unix://") {
		address := strings.TrimPrefix(d.cfg.EndPoint, "unix://")
		// Remove existing socket file
		if err := os.Remove(address); err != nil && !os.IsNotExist(err) {
			d.cfg.Logger.Warnw("Failed to remove existing socket",
				"address", address,
				"error", err,
			)
		}

		// Create directory if needed
		dir := address[:strings.LastIndex(address, "/")]
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create socket directory: %w", err)
		}

		listener, err := net.Listen("unix", address)
		if err != nil {
			return nil, fmt.Errorf("failed to listen on unix socket: %w", err)
		}
		return listener, nil
	}

	return nil, fmt.Errorf("unsupported endpoint format: %s", d.cfg.EndPoint)
}

func (d *Driver) loggingInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	d.cfg.Logger.Debugw("CSI RPC received",
		"method", info.FullMethod,
	)

	resp, err := handler(ctx, req)
	if err != nil {
		d.cfg.Logger.Warnw("CSI RPC failed",
			"method", info.FullMethod,
			"error", err,
		)
	}
	return resp, err
}

type DriverConfig struct {
	EndPoint string
	Logger   *zap.SugaredLogger
}

func (c *DriverConfig) Options(opts ...DriverOption) {
	for _, opt := range opts {
		opt.ConfigureDriver(c)
	}
}

func (c *DriverConfig) Default() {
	if c.Logger == nil {
		c.Logger = zap.NewNop().Sugar()
	}
}

type DriverOption interface {
	ConfigureDriver(*DriverConfig)
}
