# Architecture Overview

## System Design

The etcd-snapshot-driver is a Kubernetes CSI (Container Storage Interface) driver that enables creation and management of ETCD cluster snapshots.

### Components

1. **CSI Driver Server**
   - Implements Identity and GroupController services
   - Listens on Unix socket for gRPC requests
   - Coordinates snapshot operations

2. **ETCD Discovery**
   - Discovers ETCD clusters via label selectors, annotations, or StatefulSets
   - Validates cluster health and quorum
   - Resolves authentication credentials

3. **Job Executor**
   - Creates and monitors Kubernetes Jobs for snapshot operations
   - Manages job lifecycle and timeouts
   - Retrieves job logs on failure

4. **Snapshot Manager**
   - Stores and retrieves snapshot metadata
   - Manages ConfigMaps as metadata storage
   - Handles snapshot lifecycle

5. **Metrics & Health Checks**
   - Exposes Prometheus metrics on HTTP port 8080
   - Provides /healthz and /ready endpoints
   - Structured JSON logging

## Data Flow

### CreateVolumeGroupSnapshot Flow

```
VolumeGroupSnapshot CRD
        ↓
CSI CreateVolumeGroupSnapshot RPC
        ↓
Validate Request (name, source volume IDs)
        ↓
For each volume in group:
  ├─ Discover PVC from volume ID
  ├─ Discover ETCD Cluster (via label/annotation/StatefulSet)
  ├─ Validate PVC and cluster health
  ├─ Prepare Snapshot Storage (PVC)
  ├─ Generate and execute snapshot Job
  └─ Store Metadata (ConfigMap)
        ↓
Return CreateVolumeGroupSnapshotResponse
```

### DeleteVolumeGroupSnapshot Flow

```
VolumeGroupSnapshot Deletion
        ↓
CSI DeleteVolumeGroupSnapshot RPC
        ↓
Validate Request
        ↓
Clean up each snapshot in group
        ↓
Return DeleteVolumeGroupSnapshotResponse (always success)
```

## Deployment Architecture

```
┌─────────────────────────────────────────────┐
│        etcd-snapshot-driver Pod             │
├─────────────────────────────────────────────┤
│ Main Driver Container (gRPC Server)         │
│  ├─ Identity Service                        │
│  ├─ GroupController Service                  │
│  │  ├─ ETCD Discovery                       │
│  │  ├─ Credential Resolution                │
│  │  ├─ Job Execution                        │
│  │  └─ Snapshot Validation                  │
│  └─ HTTP Metrics & Health Server            │
├─────────────────────────────────────────────┤
│ CSI Snapshotter Sidecar                     │
│  └─ Manages VolumeGroupSnapshot CRDs        │
└─────────────────────────────────────────────┘
```

## RBAC & Permissions

- **Driver SA**: Broad permissions (jobs, PVCs, secrets, configmaps)
- **Executor SA**: Minimal permissions (specific ETCD credential secrets)
- **Principle of Least Privilege**: Job pods run as non-root, read-only filesystem

## Security Considerations

1. **Authentication**: Supports TLS certificates, basic auth, and no-auth modes
2. **Encryption**: ETCD credentials stored as Kubernetes Secrets
3. **Isolation**: Executor pods isolated with non-root, read-only filesystem
4. **RBAC**: Fine-grained permissions for different components

## Metadata Storage

Snapshot metadata stored in ConfigMap `etcd-snapshot-metadata` in kube-system namespace:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: etcd-snapshot-metadata
  namespace: kube-system
data:
  <snapshot-id>: |
    {
      "snapshot_id": "etcd-cluster-1234567890",
      "source_volume_id": "default/etcd-pvc",
      "cluster_name": "etcd-cluster",
      "creation_time": "2024-01-15T10:30:00Z",
      "size": 1073741824,
      "checksum_sha256": "abc123...",
      "ready_to_use": true,
      "pvc_name": "etcd-pvc",
      "namespace": "default"
    }
```

## Scalability

- **Single-replica deployment** (MVP): Suitable for development/testing
- **HA with leader election** (future): Multiple replicas with leader election for production
- **Concurrent snapshots**: Supports multiple parallel snapshot operations
- **Large clusters**: Configurable timeouts for large ETCD clusters

## Failure Handling

- **Idempotent operations**: DeleteVolumeGroupSnapshot always succeeds
- **Job retry logic**: Configurable backoff limit and retry attempts
- **Timeout handling**: Configurable snapshot timeout (default 5 minutes)
- **Metadata cleanup**: Automatic cleanup on failure
- **Logging**: Structured logging for debugging
