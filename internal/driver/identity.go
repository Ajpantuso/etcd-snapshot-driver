package driver

import (
	"context"

	"github.com/Ajpantuso/etcd-snapshot-driver/internal/config"
	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	DriverName = "etcd-snapshot-driver"
)

type IdentityServer struct {
	csi.UnimplementedIdentityServer
	cfg *IdentityServerConfig
}

func NewIdentityServer(opts ...IdentityServerOption) *IdentityServer {
	cfg := &IdentityServerConfig{}
	cfg.Options(opts...)
	cfg.Default()

	return &IdentityServer{
		cfg: cfg,
	}
}

// GetPluginInfo returns the name and version of the plugin
func (s *IdentityServer) GetPluginInfo(ctx context.Context, req *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
	s.cfg.Logger.Debugw("GetPluginInfo called")

	return &csi.GetPluginInfoResponse{
		Name:          DriverName,
		VendorVersion: config.Version,
	}, nil
}

// GetPluginCapabilities returns the capabilities of the plugin
func (s *IdentityServer) GetPluginCapabilities(ctx context.Context, req *csi.GetPluginCapabilitiesRequest) (*csi.GetPluginCapabilitiesResponse, error) {
	s.cfg.Logger.Debugw("GetPluginCapabilities called")

	return &csi.GetPluginCapabilitiesResponse{
		Capabilities: []*csi.PluginCapability{
			{
				Type: &csi.PluginCapability_Service_{
					Service: &csi.PluginCapability_Service{
						Type: csi.PluginCapability_Service_CONTROLLER_SERVICE,
					},
				},
			},
		},
	}, nil
}

// Probe checks if the plugin is ready
func (s *IdentityServer) Probe(ctx context.Context, req *csi.ProbeRequest) (*csi.ProbeResponse, error) {
	s.cfg.Logger.Debugw("Probe called")

	return &csi.ProbeResponse{
		Ready: &wrapperspb.BoolValue{Value: true},
	}, nil
}

type IdentityServerConfig struct {
	Logger *zap.SugaredLogger
}

func (c *IdentityServerConfig) Options(opts ...IdentityServerOption) {
	for _, opt := range opts {
		opt.ConfigureIdentityServer(c)
	}
}

func (c *IdentityServerConfig) Default() {
	if c.Logger == nil {
		c.Logger = zap.NewNop().Sugar()
	}
}

type IdentityServerOption interface {
	ConfigureIdentityServer(*IdentityServerConfig)
}
