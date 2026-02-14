# Usage Guide

## Overview

The etcd-snapshot-driver CSI driver enables Kubernetes to create snapshots of ETCD clusters using VolumeGroupSnapshot objects. Volume group snapshots ensure crash-consistent, atomic snapshots across all PVCs in an ETCD cluster.

## Creating Snapshots

### Volume Group Snapshot

```yaml
apiVersion: groupsnapshot.storage.k8s.io/v1beta2
kind: VolumeGroupSnapshot
metadata:
  name: etcd-group-snapshot
spec:
  volumeGroupSnapshotClassName: etcd-group-snapshot-class
  source:
    selector:
      matchLabels:
        app: etcd
```

Create the snapshot:

```bash
kubectl apply -f group-snapshot.yaml
kubectl get volumegroupsnapshot
kubectl describe volumegroupsnapshot etcd-group-snapshot
```

### Authentication

For ETCD clusters using TLS certificates:

```bash
# Create secret with ETCD client certificates
kubectl create secret tls etcd-client-certs \
  --cert=client.crt \
  --key=client.key \
  --ca=ca.crt \
  -n default
```

Then create the snapshot as usual.

## Monitoring Snapshots

### List Group Snapshots

```bash
kubectl get volumegroupsnapshot -A
```

### Check Group Snapshot Status

```bash
kubectl get volumegroupsnapshotcontent
kubectl describe volumegroupsnapshotcontent <content-name>
```

## Deleting Snapshots

```bash
kubectl delete volumegroupsnapshot etcd-group-snapshot
```

The snapshot data will be automatically cleaned up.

## Snapshot Discovery

The driver supports multiple methods for discovering ETCD clusters:

### 1. Label-based Discovery (Recommended)

Label your PVC:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  labels:
    etcd.io/cluster: my-etcd-cluster
```

### 2. Annotation-based Discovery

Add annotations to your PVC:

```yaml
annotations:
  etcd.io/endpoints: "https://etcd-0.etcd.svc.cluster.local:2379,https://etcd-1.etcd.svc.cluster.local:2379"
```

### 3. StatefulSet-based Discovery

Automatic discovery from ETCD StatefulSet (in development).

## Troubleshooting

### Check Driver Logs

```bash
kubectl logs -f deployment/etcd-snapshot-driver -n etcd-snapshot-driver
```

### Check Job Logs

```bash
kubectl logs -f job/etcd-snapshot-save-<snapshot-id> -n <namespace>
```

### Health Checks

```bash
kubectl exec -it pod/etcd-snapshot-driver-xxx -n etcd-snapshot-driver -- \
  curl http://localhost:8080/ready
```

### Common Issues

1. **Discovery Failed**: Ensure PVC has proper labels or annotations
2. **Authentication Failed**: Verify ETCD credentials secret exists
3. **Storage Full**: Check PVC capacity and available space
4. **Job Timeout**: Increase SNAPSHOT_TIMEOUT environment variable
