package driver

import (
	"context"
	"fmt"
	"time"

	"github.com/Ajpantuso/etcd-snapshot-driver/internal/etcd"
	"github.com/Ajpantuso/etcd-snapshot-driver/internal/job"
	"github.com/Ajpantuso/etcd-snapshot-driver/internal/snapshot"
	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type GroupControllerServer struct {
	csi.UnimplementedGroupControllerServer
	k8sClient              kubernetes.Interface
	discovery              *etcd.Discovery
	jobExecutor            *job.Executor
	snapshotManager        *snapshot.Manager
	snapshotPVCProvisioner *SnapshotPVCProvisioner
	cfg                    *ControllerConfig
	logger                 *zap.SugaredLogger
}

func NewGroupControllerServer(
	k8sClient kubernetes.Interface,
	opts ...ControllerOption,
) *GroupControllerServer {
	var cfg ControllerConfig
	cfg.Options(opts...)
	cfg.Default()

	return &GroupControllerServer{
		k8sClient:              k8sClient,
		discovery:              etcd.NewDiscovery(k8sClient, cfg.Logger, cfg.ClusterLabelKey),
		jobExecutor:            job.NewExecutor(k8sClient, cfg.Logger),
		snapshotManager:        snapshot.NewManager(k8sClient, cfg.Logger, "kube-system"),
		snapshotPVCProvisioner: NewSnapshotPVCProvisioner(k8sClient, cfg.DefaultStorageClass, cfg.SnapshotPVCSize, cfg.Logger),
		cfg:                    &cfg,
		logger:                 cfg.Logger,
	}
}

// GroupControllerGetCapabilities returns the capabilities of the group controller
func (g *GroupControllerServer) GroupControllerGetCapabilities(ctx context.Context, req *csi.GroupControllerGetCapabilitiesRequest) (*csi.GroupControllerGetCapabilitiesResponse, error) {
	g.logger.Debugw("GroupControllerGetCapabilities called")

	return &csi.GroupControllerGetCapabilitiesResponse{
		Capabilities: []*csi.GroupControllerServiceCapability{
			{
				Type: &csi.GroupControllerServiceCapability_Rpc{
					Rpc: &csi.GroupControllerServiceCapability_RPC{
						Type: csi.GroupControllerServiceCapability_RPC_CREATE_DELETE_GET_VOLUME_GROUP_SNAPSHOT,
					},
				},
			},
		},
	}, nil
}

// CreateVolumeGroupSnapshot creates a group snapshot of multiple ETCD cluster volumes
// Workflow:
// 1. Validate request (name, source_volume_ids)
// 2. Parse all volume IDs to extract namespace/pvc-name pairs
// 3. Validate all PVCs exist and are writable
// 4. Discover ETCD clusters from each PVC
// 5. Validate all cluster health
// 6. Generate snapshot jobs for each PVC
// 7. Execute jobs in parallel using errgroup
// 8. On error, cleanup successful snapshots and return error (all-or-nothing)
// 9. Store group snapshot metadata
// 10. Return response with all snapshot IDs
func (g *GroupControllerServer) CreateVolumeGroupSnapshot(ctx context.Context, req *csi.CreateVolumeGroupSnapshotRequest) (*csi.CreateVolumeGroupSnapshotResponse, error) {
	groupSnapshotID := fmt.Sprintf("%s-%d", req.GetName(), time.Now().Unix())
	sourceVolumeIDs := req.GetSourceVolumeIds()

	g.logger.Infow("CreateVolumeGroupSnapshot workflow starting",
		"group_snapshot_id", groupSnapshotID,
		"snapshot_name", req.GetName(),
		"source_volume_count", len(sourceVolumeIDs),
	)

	// Phase 1: Validate request
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "snapshot name required")
	}
	if len(sourceVolumeIDs) == 0 {
		return nil, status.Error(codes.InvalidArgument, "source_volume_ids required")
	}

	// Phase 2: Parse all volume IDs
	type volumeInfo struct {
		namespace  string
		name       string
		originalID string
	}
	volumes := make([]volumeInfo, len(sourceVolumeIDs))
	for i, volumeID := range sourceVolumeIDs {
		ns, name, err := parseVolumeID(volumeID)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid source_volume_id[%d]: %v", i, err)
		}
		volumes[i] = volumeInfo{namespace: ns, name: name, originalID: volumeID}
		g.logger.Debugw("Parsed volume ID",
			"index", i,
			"namespace", ns,
			"pvc_name", name,
		)
	}

	// Phase 3: Validate all PVCs
	for i, vol := range volumes {
		if err := g.validatePVCForGroup(ctx, vol.namespace, vol.name); err != nil {
			g.logger.Errorw("PVC validation failed",
				"index", i,
				"namespace", vol.namespace,
				"pvc_name", vol.name,
				"error", err,
			)
			return nil, status.Errorf(codes.FailedPrecondition, "PVC validation failed for volume %d: %v", i, err)
		}
		g.logger.Debugw("PVC validation passed",
			"index", i,
			"namespace", vol.namespace,
			"pvc_name", vol.name,
		)
	}

	// Phase 4 & 5: Discover ETCD clusters and validate health
	type clusterInfo struct {
		index      int
		info       *etcd.ClusterInfo
		volumeInfo volumeInfo
	}
	clusterInfos := make([]clusterInfo, 0, len(volumes))
	var firstClusterName string
	var firstClusterEndpoints []string

	for i, vol := range volumes {
		info, err := g.discovery.DiscoverCluster(ctx, vol.namespace, vol.name)
		if err != nil {
			g.logger.Errorw("ETCD cluster discovery failed",
				"index", i,
				"namespace", vol.namespace,
				"pvc_name", vol.name,
				"error", err,
			)
			return nil, status.Errorf(codes.Internal, "ETCD discovery failed for volume %d: %v", i, err)
		}

		// Validate cluster health
		if err := g.discovery.ValidateClusterHealth(ctx, info); err != nil {
			g.logger.Errorw("ETCD cluster health validation failed",
				"index", i,
				"cluster_name", info.Name,
				"error", err,
			)
			return nil, status.Errorf(codes.FailedPrecondition, "ETCD cluster health validation failed for volume %d: %v", i, err)
		}

		// Validate cluster consistency: all PVCs must belong to same cluster
		if i == 0 {
			firstClusterName = info.Name
			firstClusterEndpoints = info.Endpoints
		} else {
			if info.Name != firstClusterName {
				g.logger.Errorw("Cluster mismatch in VolumeGroupSnapshot",
					"volume_index", i,
					"expected_cluster", firstClusterName,
					"got_cluster", info.Name,
				)
				return nil, status.Errorf(codes.FailedPrecondition,
					"all PVCs must belong to the same ETCD cluster: expected %s, got %s",
					firstClusterName, info.Name)
			}
			// Compare endpoints to ensure it's the same cluster
			if len(info.Endpoints) != len(firstClusterEndpoints) {
				g.logger.Errorw("Cluster endpoints mismatch in VolumeGroupSnapshot",
					"volume_index", i,
					"expected_endpoints", firstClusterEndpoints,
					"got_endpoints", info.Endpoints,
				)
				return nil, status.Errorf(codes.FailedPrecondition,
					"all PVCs must belong to the same ETCD cluster: endpoints differ")
			}
		}

		clusterInfos = append(clusterInfos, clusterInfo{
			index:      i,
			info:       info,
			volumeInfo: vol,
		})

		g.logger.Debugw("ETCD cluster discovered and validated",
			"index", i,
			"cluster_name", info.Name,
			"endpoints", info.Endpoints,
		)
	}

	// Phase 6: Ensure dedicated snapshot PVC exists
	firstCluster := clusterInfos[0]
	snapshotPVCName, err := g.snapshotPVCProvisioner.EnsureSnapshotPVC(ctx, firstCluster.volumeInfo.namespace)
	if err != nil {
		g.logger.Errorw("Failed to ensure snapshot PVC", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to prepare snapshot storage: %v", err)
	}

	snapshotID := fmt.Sprintf("%s-%d", groupSnapshotID, time.Now().Unix())

	jobConfig := &job.JobConfig{
		SnapshotID:            snapshotID,
		Namespace:             firstCluster.volumeInfo.namespace,
		ETCDEndpoints:         firstCluster.info.Endpoints,
		SnapshotPVCName:       snapshotPVCName,
		SnapshotPVCNamespace:  firstCluster.volumeInfo.namespace,
		Timeout:               300,
		BackoffLimit:          g.cfg.JobBackoffLimit,
		ActiveDeadlineSeconds: g.cfg.JobActiveDeadlineSeconds,
		Operation:             "save",
		TLSEnabled:            g.cfg.ETCDTLSEnabled,
		TLSSecretName:         g.cfg.ETCDTLSSecretName,
		ClientCertPath:        g.cfg.ETCDClientCertPath,
		ClientKeyPath:         g.cfg.ETCDClientKeyPath,
		CAPath:                g.cfg.ETCDCAPath,
		ETCDImage:             g.cfg.ETCDImage,
		BusyboxImage:          g.cfg.BusyboxImage,
	}

	snapshotJob := job.GenerateSnapshotSaveJob(jobConfig)
	g.logger.Debugw("Generated snapshot job",
		"snapshot_id", snapshotID,
		"job_name", snapshotJob.Name,
		"pvc_name", firstCluster.volumeInfo.name,
	)

	// Phase 7: Execute the single snapshot job
	jobResult, err := g.jobExecutor.ExecuteSnapshotJob(ctx, snapshotJob, g.cfg.SnapshotTimeout)
	if err != nil {
		g.logger.Errorw("Snapshot job failed",
			"snapshot_id", snapshotID,
			"error", err,
		)
		return nil, status.Errorf(codes.Internal, "snapshot creation failed: %v", err)
	}

	g.logger.Infow("Snapshot job completed successfully",
		"snapshot_id", snapshotID,
		"duration", jobResult.Duration.String(),
	)

	// Phase 8: Store individual snapshot metadata (needed by cleanupSnapshot)
	snapMetadata := &snapshot.SnapshotMetadata{
		SnapshotID:     snapshotID,
		SourceVolumeID: sourceVolumeIDs[0],
		ClusterName:    firstClusterName,
		CreationTime:   time.Now(),
		ReadyToUse:     true,
		PVCName:        snapshotPVCName,
		Namespace:      firstCluster.volumeInfo.namespace,
	}
	if err := g.snapshotManager.StoreSnapshotMetadata(ctx, snapMetadata); err != nil {
		g.logger.Warnw("Failed to store snapshot metadata", "error", err)
	}

	// Store group snapshot metadata
	groupMetadata := &snapshot.GroupSnapshotMetadata{
		GroupSnapshotID:      groupSnapshotID,
		SnapshotID:           snapshotID,
		SourceVolumeIDs:      sourceVolumeIDs,
		ClusterName:          firstClusterName,
		SnapshotPVCName:      snapshotPVCName,
		SnapshotPVCNamespace: firstCluster.volumeInfo.namespace,
		CreationTime:         time.Now(),
		ReadyToUse:           true,
	}

	if err := g.snapshotManager.StoreGroupSnapshotMetadata(ctx, groupMetadata); err != nil {
		g.logger.Warnw("Failed to store group snapshot metadata", "error", err)
		// Don't fail the entire operation if metadata storage fails
	}

	g.logger.Infow("CreateVolumeGroupSnapshot workflow completed successfully",
		"group_snapshot_id", groupSnapshotID,
		"snapshot_id", snapshotID,
		"cluster_name", firstClusterName,
		"source_volumes_count", len(sourceVolumeIDs),
	)

	// Phase 9: Build response with single snapshot representing all volumes
	response := &csi.CreateVolumeGroupSnapshotResponse{
		GroupSnapshot: &csi.VolumeGroupSnapshot{
			GroupSnapshotId: groupSnapshotID,
			Snapshots: []*csi.Snapshot{
				{
					SnapshotId:     snapshotID,
					SourceVolumeId: sourceVolumeIDs[0],
					CreationTime:   timestamppb.Now(),
					ReadyToUse:     true,
				},
			},
			CreationTime: timestamppb.Now(),
			ReadyToUse:   true,
		},
	}

	return response, nil
}

// DeleteVolumeGroupSnapshot deletes all snapshots in a group
// Workflow:
// 1. Validate request
// 2. Retrieve group metadata (idempotent - if not found, return success)
// 3. Delete each individual snapshot
// 4. Delete group metadata
// 5. Return success (always idempotent)
func (g *GroupControllerServer) DeleteVolumeGroupSnapshot(ctx context.Context, req *csi.DeleteVolumeGroupSnapshotRequest) (*csi.DeleteVolumeGroupSnapshotResponse, error) {
	groupSnapshotID := req.GetGroupSnapshotId()

	g.logger.Infow("DeleteVolumeGroupSnapshot workflow starting",
		"group_snapshot_id", groupSnapshotID,
	)

	// Phase 1: Validate request
	if groupSnapshotID == "" {
		return nil, status.Error(codes.InvalidArgument, "group_snapshot_id required")
	}

	// Phase 2: Retrieve group metadata (idempotent)
	metadata, err := g.snapshotManager.RetrieveGroupSnapshotMetadata(ctx, groupSnapshotID)
	if err != nil {
		g.logger.Infow("Group snapshot metadata not found (idempotent deletion)",
			"group_snapshot_id", groupSnapshotID,
		)
		return &csi.DeleteVolumeGroupSnapshotResponse{}, nil
	}

	g.logger.Debugw("Retrieved group snapshot metadata",
		"group_snapshot_id", groupSnapshotID,
		"snapshot_id", metadata.SnapshotID,
	)

	// Phase 3: Delete the single snapshot
	if err := g.cleanupSnapshot(ctx, metadata.SnapshotID); err != nil {
		g.logger.Warnw("Failed to cleanup snapshot",
			"snapshot_id", metadata.SnapshotID,
			"error", err,
		)
		// Continue with metadata cleanup even if snapshot cleanup fails
	}

	// Phase 4: Delete group metadata
	if err := g.snapshotManager.DeleteGroupSnapshotMetadata(ctx, groupSnapshotID); err != nil {
		g.logger.Warnw("Failed to delete group snapshot metadata",
			"group_snapshot_id", groupSnapshotID,
			"error", err,
		)
		// Still return success
	}

	// Phase 5: Return success (always idempotent)
	g.logger.Infow("DeleteVolumeGroupSnapshot workflow completed",
		"group_snapshot_id", groupSnapshotID,
	)

	return &csi.DeleteVolumeGroupSnapshotResponse{}, nil
}

// GetVolumeGroupSnapshot retrieves the status of a group snapshot
// Workflow:
// 1. Validate request
// 2. Retrieve group metadata
// 3. Build response with current status
func (g *GroupControllerServer) GetVolumeGroupSnapshot(ctx context.Context, req *csi.GetVolumeGroupSnapshotRequest) (*csi.GetVolumeGroupSnapshotResponse, error) {
	groupSnapshotID := req.GetGroupSnapshotId()

	g.logger.Debugw("GetVolumeGroupSnapshot called",
		"group_snapshot_id", groupSnapshotID,
	)

	// Phase 1: Validate request
	if groupSnapshotID == "" {
		return nil, status.Error(codes.InvalidArgument, "group_snapshot_id required")
	}

	// Phase 2: Retrieve group metadata
	metadata, err := g.snapshotManager.RetrieveGroupSnapshotMetadata(ctx, groupSnapshotID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "group snapshot not found: %s", groupSnapshotID)
	}

	g.logger.Debugw("Retrieved group snapshot metadata",
		"group_snapshot_id", groupSnapshotID,
		"snapshot_id", metadata.SnapshotID,
		"ready_to_use", metadata.ReadyToUse,
	)

	// Phase 3: Retrieve snapshot metadata for details
	snapMetadata, err := g.snapshotManager.RetrieveSnapshotMetadata(ctx, metadata.SnapshotID)
	creationTime := timestamppb.New(metadata.CreationTime)
	if err == nil && snapMetadata != nil {
		creationTime = timestamppb.New(snapMetadata.CreationTime)
	}

	// Phase 4: Build response with single snapshot
	response := &csi.GetVolumeGroupSnapshotResponse{
		GroupSnapshot: &csi.VolumeGroupSnapshot{
			GroupSnapshotId: groupSnapshotID,
			Snapshots: []*csi.Snapshot{
				{
					SnapshotId:     metadata.SnapshotID,
					SourceVolumeId: metadata.SourceVolumeIDs[0],
					CreationTime:   creationTime,
					ReadyToUse:     metadata.ReadyToUse,
				},
			},
			CreationTime: creationTime,
			ReadyToUse:   metadata.ReadyToUse,
		},
	}

	return response, nil
}

// Helper function to validate PVC for group snapshots
func (g *GroupControllerServer) validatePVCForGroup(ctx context.Context, namespace, name string) error {
	pvc, err := g.k8sClient.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("PVC not found: %w", err)
	}

	// Check PVC is bound
	if pvc.Status.Phase != "Bound" {
		return fmt.Errorf("PVC not bound, current phase: %s", pvc.Status.Phase)
	}

	// Check access mode supports writing
	hasWriteAccess := false
	for _, mode := range pvc.Spec.AccessModes {
		if mode == "ReadWriteOnce" || mode == "ReadWriteMany" {
			hasWriteAccess = true
			break
		}
	}
	if !hasWriteAccess {
		return fmt.Errorf("PVC does not have write access mode")
	}

	return nil
}

// Helper function to cleanup a single snapshot (used for error handling in CreateVolumeGroupSnapshot)
func (g *GroupControllerServer) cleanupSnapshot(ctx context.Context, snapshotID string) error {
	// Retrieve metadata to find the PVC details
	metadata, err := g.snapshotManager.RetrieveSnapshotMetadata(ctx, snapshotID)
	if err != nil {
		g.logger.Debugw("Snapshot metadata not found for cleanup (already deleted)",
			"snapshot_id", snapshotID,
		)
		return nil
	}

	// Create and execute cleanup job
	jobConfig := &job.JobConfig{
		SnapshotID:            snapshotID,
		Namespace:             metadata.Namespace,
		SnapshotPVCName:       metadata.PVCName,
		SnapshotPVCNamespace:  metadata.Namespace,
		Timeout:               60,
		BackoffLimit:          1,
		ActiveDeadlineSeconds: 120,
		Operation:             "delete",
	}

	deleteJob := job.GenerateSnapshotDeleteJob(jobConfig)
	g.logger.Debugw("Generated cleanup job",
		"snapshot_id", snapshotID,
		"job_name", deleteJob.Name,
	)

	// Create cleanup job
	createdJob, err := g.k8sClient.BatchV1().Jobs(metadata.Namespace).Create(ctx, deleteJob, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create cleanup job: %w", err)
	}

	// Wait for job with short timeout
	_, err = g.jobExecutor.ExecuteSnapshotJob(ctx, createdJob, time.Minute)
	if err != nil {
		return fmt.Errorf("cleanup job failed: %w", err)
	}

	// Delete metadata
	if err := g.snapshotManager.DeleteSnapshotMetadata(ctx, snapshotID); err != nil {
		return fmt.Errorf("failed to delete metadata: %w", err)
	}

	g.logger.Infow("Snapshot cleanup completed",
		"snapshot_id", snapshotID,
	)

	return nil
}
