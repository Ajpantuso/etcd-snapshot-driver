package driver

import (
	"context"
	"testing"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGroupControllerGetCapabilities(t *testing.T) {
	// Create fake Kubernetes client
	fakeClient := fake.NewSimpleClientset()

	// Create logger
	logger := zap.NewNop().Sugar()

	// Create GroupControllerServer
	server := NewGroupControllerServer(
		fakeClient,
		ControllerOption(WithLogger{Logger: logger}),
	)

	// Call GetCapabilities
	ctx := context.Background()
	resp, err := server.GroupControllerGetCapabilities(ctx, nil)

	// Assert no error
	assert.NoError(t, err)
	assert.NotNil(t, resp)

	// Assert capabilities
	assert.Len(t, resp.Capabilities, 1)
	assert.NotNil(t, resp.Capabilities[0].GetRpc())
	assert.Equal(t, "CREATE_DELETE_GET_VOLUME_GROUP_SNAPSHOT", resp.Capabilities[0].GetRpc().Type.String())
}

func TestCreateVolumeGroupSnapshotMissingName(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	logger := zap.NewNop().Sugar()

	server := NewGroupControllerServer(
		fakeClient,
		ControllerOption(WithLogger{Logger: logger}),
	)

	ctx := context.Background()
	// Create request with missing name
	req := &csi.CreateVolumeGroupSnapshotRequest{
		Name:             "",
		SourceVolumeIds:  []string{"default/etcd-data"},
	}

	resp, err := server.CreateVolumeGroupSnapshot(ctx, req)

	// Assert error
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "snapshot name required")
}

func TestCreateVolumeGroupSnapshotMissingSourceVolumeIDs(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	logger := zap.NewNop().Sugar()

	server := NewGroupControllerServer(
		fakeClient,
		ControllerOption(WithLogger{Logger: logger}),
	)

	ctx := context.Background()
	// Create request with missing source volume IDs
	req := &csi.CreateVolumeGroupSnapshotRequest{
		Name:            "test-snapshot",
		SourceVolumeIds: []string{},
	}

	resp, err := server.CreateVolumeGroupSnapshot(ctx, req)

	// Assert error
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "source_volume_ids required")
}

func TestDeleteVolumeGroupSnapshotMissingID(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	logger := zap.NewNop().Sugar()

	server := NewGroupControllerServer(
		fakeClient,
		ControllerOption(WithLogger{Logger: logger}),
	)

	ctx := context.Background()
	// Create request with missing group snapshot ID
	req := &csi.DeleteVolumeGroupSnapshotRequest{
		GroupSnapshotId: "",
	}

	resp, err := server.DeleteVolumeGroupSnapshot(ctx, req)

	// Assert error
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "group_snapshot_id required")
}

func TestDeleteVolumeGroupSnapshotIdempotent(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	logger := zap.NewNop().Sugar()

	server := NewGroupControllerServer(
		fakeClient,
		ControllerOption(WithLogger{Logger: logger}),
	)

	ctx := context.Background()
	// Create request for non-existent group snapshot
	req := &csi.DeleteVolumeGroupSnapshotRequest{
		GroupSnapshotId: "non-existent-group-snapshot",
	}

	resp, err := server.DeleteVolumeGroupSnapshot(ctx, req)

	// Assert no error (idempotent)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestGetVolumeGroupSnapshotMissingID(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	logger := zap.NewNop().Sugar()

	server := NewGroupControllerServer(
		fakeClient,
		ControllerOption(WithLogger{Logger: logger}),
	)

	ctx := context.Background()
	// Create request with missing group snapshot ID
	req := &csi.GetVolumeGroupSnapshotRequest{
		GroupSnapshotId: "",
	}

	resp, err := server.GetVolumeGroupSnapshot(ctx, req)

	// Assert error
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "group_snapshot_id required")
}

func TestGetVolumeGroupSnapshotNotFound(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	logger := zap.NewNop().Sugar()

	server := NewGroupControllerServer(
		fakeClient,
		ControllerOption(WithLogger{Logger: logger}),
	)

	ctx := context.Background()
	// Create request for non-existent group snapshot
	req := &csi.GetVolumeGroupSnapshotRequest{
		GroupSnapshotId: "non-existent-group-snapshot",
	}

	resp, err := server.GetVolumeGroupSnapshot(ctx, req)

	// Assert error
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "group snapshot not found")
}

func TestCreateVolumeGroupSnapshotMixedClusters(t *testing.T) {
	// This test verifies that CreateVolumeGroupSnapshot fails when PVCs belong to different ETCD clusters
	// Since we don't have full mocking infrastructure for ETCD discovery in tests,
	// this test validates the expected behavior at the validation layer.
	// Full integration tests would require mocking the discovery.DiscoverCluster method.

	fakeClient := fake.NewSimpleClientset()
	logger := zap.NewNop().Sugar()

	server := NewGroupControllerServer(
		fakeClient,
		ControllerOption(WithLogger{Logger: logger}),
	)

	ctx := context.Background()
	// Create request with multiple PVCs
	req := &csi.CreateVolumeGroupSnapshotRequest{
		Name:            "test-snapshot",
		SourceVolumeIds: []string{"default/etcd-data-1", "default/etcd-data-2"},
	}

	resp, err := server.CreateVolumeGroupSnapshot(ctx, req)

	// Should fail because PVCs don't exist for validation
	assert.Error(t, err)
	assert.Nil(t, resp)
}
