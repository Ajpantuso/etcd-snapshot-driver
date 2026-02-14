# Velero Integration with etcd-snapshot-driver

This guide explains how to configure Velero to use the etcd-snapshot-driver for crash-consistent backups of ETCD clusters using Volume Group Snapshots.

## Overview

Volume Group Snapshots (VGS) enable atomic snapshotting of multiple PersistentVolumeClaims, which is critical for ETCD clusters that span multiple volumes (e.g., data volume + logs volume). This ensures restore consistency and prevents data corruption that could result from non-coordinated snapshots.

### Key Features

- **Crash-Consistent Snapshots**: All volumes in the group snapshot at the same point in time
- **Atomic Operations**: Either all snapshots succeed or all fail - no partial states
- **Velero Integration**: Seamless backup and restore workflows
- **Multiple Cluster Support**: Works with HyperShift and standard ETCD deployments

## Prerequisites

### 1. Kubernetes Version

- **Kubernetes 1.27+**: VolumeGroupSnapshot is available as a beta feature
- **Kubernetes 1.32+**: Recommended for mature VolumeGroupSnapshot support

### 2. External Snapshotter CRDs

Install the VolumeGroupSnapshot Custom Resource Definitions:

```bash
# Install VolumeGroupSnapshot CRDs (from external-snapshotter project)
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/master/client/config/crd/groupsnapshot.storage.k8s.io_volumegroupsnapshotclasses.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/master/client/config/crd/groupsnapshot.storage.k8s.io_volumegroupsnapshots.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/master/client/config/crd/groupsnapshot.storage.k8s.io_volumegroupsnapshotcontents.yaml
```

Verify installation:

```bash
kubectl get crd | grep groupsnapshot
# Expected output:
# volumegroupsnapshotclasses.groupsnapshot.storage.k8s.io
# volumegroupsnapshots.groupsnapshot.storage.k8s.io
# volumegroupsnapshotcontents.groupsnapshot.storage.k8s.io
```

### 3. Snapshot Controller

Install the snapshot-controller (handles VolumeGroupSnapshot lifecycle):

```bash
# Using kustomize
kubectl apply -k github.com/kubernetes-csi/external-snapshotter/deploy/kubernetes/snapshot-controller?ref=master

# Or using Helm
helm repo add snapshot-controller https://kubernetes-csi.github.io/external-snapshotter
helm install snapshot-controller snapshot-controller/snapshot-controller
```

Verify installation:

```bash
kubectl get deployment -n kube-system snapshot-controller
# Expected output should show the snapshot-controller pod running
```

### 4. etcd-snapshot-driver

Deploy the etcd-snapshot-driver with GroupController support:

```bash
# Deploy the driver with GroupController enabled
kubectl apply -k deploy/

# Verify deployment
kubectl get deployment -n etcd-snapshot-driver
kubectl get csidriver
```

## Configuration

### 1. Create VolumeGroupSnapshotClass

Deploy the VolumeGroupSnapshotClass (usually done as part of driver deployment):

```bash
kubectl apply -f deploy/base/volume-group-snapshot-class.yaml
```

Verify:

```bash
kubectl get volumegroupsnapshotclass
# Expected output:
# NAME                          DRIVER                   AGE
# etcd-group-snapshot-class     etcd-snapshot-driver     1m
```

### 2. Label ETCD PVCs

To use group snapshots, ETCD PVCs must have labels that Velero can use for grouping:

```bash
# Example: Label PVCs in an ETCD cluster for backup grouping
kubectl label pvc etcd-data -n openshift-etcd app=etcd backup.velero.io/sync=true
kubectl label pvc etcd-wal -n openshift-etcd app=etcd backup.velero.io/sync=true
```

The labels act as selectors when creating VolumeGroupSnapshots. All PVCs with matching labels will be snapshotted atomically together.

### 3. Configure Velero

Configure Velero to use the etcd-snapshot-driver for group snapshots:

#### Option A: Using Helm Values

```yaml
# values.yaml
configuration:
  csiSnapshotTimeout: 10m
  volumeSnapshotLocation:
    provider: csi
    config:
      snapshotClass: etcd-group-snapshot-class
  volumeGroupSnapshotLocation:
    provider: csi
    config:
      snapshotGroupClass: etcd-group-snapshot-class
```

Deploy with Helm:

```bash
helm install velero velero/velero \
  --namespace velero \
  --create-namespace \
  --values values.yaml
```

#### Option B: Using Velero CLI

```bash
velero install \
  --provider aws \
  --bucket my-bucket \
  --secret-file ./credentials-velero \
  --use-volume-snapshots \
  --volume-snapshot-location default=csi
```

Then patch the BackupStorageLocation:

```bash
kubectl patch volumesnapshotlocation default -n velero --type merge \
  -p '{"spec":{"config":{"snapshotClass":"etcd-group-snapshot-class"}}}'
```

## Usage

### Creating a Backup with Group Snapshots

#### 1. Create a Backup Resource

```yaml
apiVersion: velero.io/v1
kind: Backup
metadata:
  name: etcd-backup-group
  namespace: velero
spec:
  includedNamespaces:
    - openshift-etcd
  includedResources:
    - persistentvolumeclaims
    - persistentvolumes
  volumeSnapshotLocation: default
  hooks:
    resources:
      - name: pre-backup-hook
        pre:
          - exec:
              command:
                - "etcdctl"
                - "snapshot"
                - "status"
              # Optional: verify cluster health before snapshot
```

#### 2. Apply the Backup

```bash
kubectl apply -f backup.yaml

# Monitor backup progress
kubectl logs -n velero deployment/velero -f
kubectl get backup -n velero
kubectl describe backup etcd-backup-group -n velero
```

#### 3. Verify VolumeGroupSnapshot Creation

Velero will automatically create a VolumeGroupSnapshot for PVCs with matching labels:

```bash
kubectl get volumegroupsnapshot -A
# Expected output:
# NAMESPACE      NAME                         DRIVER                    STATE     READYTOUSE
# openshift-etcd etcd-backup-group-xxxxx      etcd-snapshot-driver      Ready     true
```

### Creating a Restore

```yaml
apiVersion: velero.io/v1
kind: Restore
metadata:
  name: etcd-restore-group
  namespace: velero
spec:
  backupName: etcd-backup-group
  # Velero will automatically restore all group snapshots
```

Apply and monitor:

```bash
kubectl apply -f restore.yaml
kubectl describe restore etcd-restore-group -n velero
```

## Advanced Configuration

### Snapshot Timeout

Configure the snapshot timeout for long-running operations:

```yaml
# Patch the etcd-snapshot-driver deployment
kubectl patch deployment etcd-snapshot-driver -n etcd-snapshot-driver \
  --type='json' \
  -p='[{"op": "replace", "path": "/spec/template/spec/containers/0/env", "value": [
    {"name": "SNAPSHOT_TIMEOUT", "value": "15m"}
  ]}]'
```

### Selective PVC Grouping

Control which PVCs are included in group snapshots using labels:

```bash
# Only these PVCs will be snapshotted together
kubectl label pvc etcd-data -n openshift-etcd velero.io/group=main-cluster
kubectl label pvc etcd-wal -n openshift-etcd velero.io/group=main-cluster

# Create VolumeGroupSnapshot with selector
apiVersion: groupsnapshot.storage.k8s.io/v1beta2
kind: VolumeGroupSnapshot
metadata:
  name: etcd-backup-main
spec:
  volumeGroupSnapshotClassName: etcd-group-snapshot-class
  source:
    selector:
      matchLabels:
        velero.io/group: main-cluster
```

### Error Handling and Recovery

#### PVC Validation Failures

If a PVC fails validation (not bound, insufficient permissions, etc.):

```bash
# Check events for details
kubectl describe pvc etcd-data -n openshift-etcd

# Common issues:
# 1. PVC not bound - wait for provisioner or debug storage
# 2. Access mode mismatch - verify storage class supports RWO/RWX
# 3. Network issues - check connectivity to ETCD endpoints
```

#### ETCD Cluster Health Failures

If ETCD cluster health check fails:

```bash
# Verify ETCD cluster is accessible
kubectl exec -it -n openshift-etcd etcd-0 -- etcdctl endpoint health

# Check cluster quorum
kubectl exec -it -n openshift-etcd etcd-0 -- etcdctl member list

# Review driver logs
kubectl logs -n etcd-snapshot-driver deployment/etcd-snapshot-driver
```

#### Snapshot Cleanup

If a group snapshot creation fails, cleanup happens automatically:

```bash
# Monitor cleanup in driver logs
kubectl logs -n etcd-snapshot-driver deployment/etcd-snapshot-driver | grep "group snapshot"

# Manual cleanup if needed
kubectl delete volumegroupsnapshot <failed-snapshot> -n <namespace>
```

## Troubleshooting

### Verify Driver Capabilities

```bash
# Check that GroupController is registered
kubectl logs -n etcd-snapshot-driver deployment/etcd-snapshot-driver | grep -i "group\|capability"
```

### Check VolumeGroupSnapshot Status

```bash
# Describe failed group snapshot
kubectl describe volumegroupsnapshot <name> -n <namespace>

# Check for errors in group snapshot content
kubectl get volumegroupsnapshotcontent -A
kubectl describe volumegroupsnapshotcontent <content-name>
```

### Monitor Individual Snapshots

Even though group snapshots are atomic, individual snapshots are created:

```bash
# List all snapshots from a group snapshot
kubectl get volumesnapshot -A -l backup-name=<group-snapshot-id>

# Check individual snapshot status
kubectl describe volumesnapshot <snapshot-name> -n <namespace>
```

### Enable Debug Logging

Increase verbosity for troubleshooting:

```bash
# Patch driver to enable debug logging
kubectl set env deployment/etcd-snapshot-driver -n etcd-snapshot-driver LOG_LEVEL=debug

# Watch logs
kubectl logs -n etcd-snapshot-driver deployment/etcd-snapshot-driver -f
```

## Best Practices

1. **Label Strategy**: Use consistent labels across all ETCD PVCs
   - Example: `app=etcd`, `cluster=production`, `backup=enabled`

2. **Snapshot Frequency**: Balance between backup frequency and resource usage
   - Daily snapshots: typical for non-critical systems
   - Hourly snapshots: for critical ETCD clusters

3. **Retention Policies**: Set appropriate retention on snapshots
   ```bash
   kubectl patch volumegroupsnapshotclass etcd-group-snapshot-class \
     --type merge -p '{"metadata":{"annotations":{"snapshot.storage.k8s.io/ttl":"30d"}}}'
   ```

4. **Testing**: Regularly test restores to verify backup integrity
   ```bash
   # Schedule test restores in isolated clusters
   # Verify ETCD cluster quorum after restore
   ```

5. **Monitoring**: Track snapshot operations
   ```bash
   # Create Prometheus alerts for failed snapshots
   # Alert on slow snapshot creation
   ```

## API Reference

### VolumeGroupSnapshot API

```yaml
apiVersion: groupsnapshot.storage.k8s.io/v1beta2
kind: VolumeGroupSnapshot
metadata:
  name: my-backup
spec:
  volumeGroupSnapshotClassName: etcd-group-snapshot-class
  source:
    # Option 1: Selector-based grouping
    selector:
      matchLabels:
        app: etcd
    
    # Option 2: Direct volume reference (future)
    # volumeIds:
    #   - pvc-1-id
    #   - pvc-2-id
status:
  # Status fields (set by snapshot-controller)
  readyToUse: true
  groupSnapshotContentName: vgsc-xxxxx
  snapshots:
    - snapshotHandle: snapshot-1
      readyToUse: true
    - snapshotHandle: snapshot-2
      readyToUse: true
```

## See Also

- [Velero Documentation](https://velero.io/)
- [Kubernetes CSI Snapshotter](https://github.com/kubernetes-csi/external-snapshotter)
- [etcd-snapshot-driver README](../README.md)
- [HyperShift etcd Management](./hypershift-etcd.md)
