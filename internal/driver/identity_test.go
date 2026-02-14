package driver_test

import (
	"context"
	"testing"

	"github.com/Ajpantuso/etcd-snapshot-driver/internal/driver"
	csi "github.com/container-storage-interface/spec/lib/go/csi"
)

func TestIdentityGetPluginInfo(t *testing.T) {
	server := driver.NewIdentityServer()

	resp, err := server.GetPluginInfo(context.Background(), &csi.GetPluginInfoRequest{})
	if err != nil {
		t.Fatalf("GetPluginInfo failed: %v", err)
	}

	if resp.Name != "etcd-snapshot-driver" {
		t.Errorf("Expected name 'etcd-snapshot-driver', got '%s'", resp.Name)
	}

	if resp.VendorVersion != "dev" {
		t.Errorf("Expected version 'dev', got '%s'", resp.VendorVersion)
	}
}

func TestIdentityGetPluginCapabilities(t *testing.T) {
	server := driver.NewIdentityServer()

	resp, err := server.GetPluginCapabilities(context.Background(), &csi.GetPluginCapabilitiesRequest{})
	if err != nil {
		t.Fatalf("GetPluginCapabilities failed: %v", err)
	}

	if len(resp.Capabilities) == 0 {
		t.Error("Expected capabilities, got none")
	}

	found := false
	for _, cap := range resp.Capabilities {
		if service := cap.GetService(); service != nil {
			if service.Type == csi.PluginCapability_Service_CONTROLLER_SERVICE {
				found = true
				break
			}
		}
	}

	if !found {
		t.Error("Expected CONTROLLER_SERVICE capability")
	}
}

func TestIdentityProbe(t *testing.T) {
	server := driver.NewIdentityServer()

	resp, err := server.Probe(context.Background(), &csi.ProbeRequest{})
	if err != nil {
		t.Fatalf("Probe failed: %v", err)
	}

	if resp.Ready == nil || !resp.Ready.Value {
		t.Error("Expected probe to return ready=true")
	}
}
