# etcd-snapshot-driver Refactoring

## Single Snapshot VolumeGroupSnapshot Implementation

**Status**: ✅ Completed

### Key Insight
For ETCD clusters, a single snapshot from any member contains all data needed to restore the entire cluster. Multiple PVCs in a VolumeGroupSnapshot represent different aspects of the same cluster (data dir, WAL dir, etc.), not different data.

### Changes Made

1. **internal/snapshot/metadata.go** - GroupSnapshotMetadata structure
   - `SnapshotIDs []string` → `SnapshotID string` (singular)
   - `ClusterNames []string` → `ClusterName string` (singular)
   - Removed `Namespace string`
   - Added `SnapshotPVCName string` (tracks which PVC stores the snapshot)
   - Added `SnapshotPVCNamespace string`

2. **internal/driver/groupcontroller.go** - GroupControllerServer methods
   - **CreateVolumeGroupSnapshot**:
     * Validates all PVCs exist and are bound
     * Discovers ETCD cluster from each PVC
     * Validates cluster health for each PVC
     * Validates all PVCs belong to SAME cluster (new validation)
     * Creates ONE snapshot job (for first PVC only)
     * Executes single job directly (no errgroup needed)
     * Stores single snapshot ID in group metadata
   - **DeleteVolumeGroupSnapshot**: Deletes single snapshot (was iterating over multiple)
   - **GetVolumeGroupSnapshot**: Returns single snapshot (was iterating over multiple)
   - Removed unused imports: `golang.org/x/sync/errgroup`, `k8s.io/api/batch/v1`

3. **internal/driver/groupcontroller_test.go**
   - Added `TestCreateVolumeGroupSnapshotMixedClusters` test
   - All existing validation tests still pass

### Testing
- All 8 tests pass (including new mixed clusters test)
- Project builds successfully with `go build ./...`
- No breaking changes to existing tests
