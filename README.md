# etcd-snapshot-Driver: CSI-Based ETCD Cluster Snapshot Management

A Kubernetes Container Storage Interface (CSI) driver implementation enabling point-in-time snapshot creation, management, and restoration of ETCD clusters running on Kubernetes. This specification document provides comprehensive technical guidance for deployment, integration, and operation of the etcd-snapshot-driver.

---

## 1. Project Overview

### Mission Statement

The etcd-snapshot-driver solves the problem of creating consistent, point-in-time snapshots of ETCD clusters running within Kubernetes environments. It provides a Kubernetes-native interface for ETCD cluster backup and disaster recovery, integrating seamlessly with the Container Storage Interface (CSI) specification and Velero backup orchestration.

### Problem Definition

ETCD clusters store critical Kubernetes state data. Traditional backup approaches require:
- Manual intervention or custom scripts
- Out-of-band backup mechanisms disconnected from Kubernetes resources
- Manual coordination between multiple ETCD instances
- Complex integration with backup solutions like Velero

This driver eliminates these challenges by:
- Providing a standard CSI-compatible interface for ETCD snapshots
- Automating cluster member discovery and state coordination
- Integrating with Kubernetes Volume Group Snapshot objects
- Supporting Velero DataMover for transparent backup movement

### Target Audience and Use Cases

**Primary Users:**
- Kubernetes platform teams managing self-hosted ETCD clusters
- Organizations using Velero for cluster backup and disaster recovery
- Cloud providers offering managed Kubernetes with ETCD backup capabilities

**Key Use Cases:**
- Scheduled cluster snapshots via Velero backup policies
- Pre-upgrade cluster snapshots for rollback capability
- Disaster recovery and cluster restoration
- Multi-cluster backup consolidation
- Compliance-driven snapshot retention

### Key Differentiators

1. **CSI-Native Integration**: Uses standard Kubernetes CSI and Volume Group Snapshot APIs
2. **Cluster-Wide Awareness**: Single snapshot captures entire multi-member ETCD cluster
3. **Zero Downtime**: Snapshots do not interrupt ETCD service
4. **Kubernetes-First Design**: All operations through Kubernetes Jobs and Resources
5. **Velero Ready**: Direct integration with Velero DataMover for backup management

### Project Status and Maturity

**Status**: Alpha/Beta (version 1.0)
- Core snapshot creation, deletion, and restoration implemented
- CSI specification v1.8.0 compliance
- Production-ready RBAC and security controls
- Comprehensive error handling and recovery

---

## 2. Architecture Overview

### System Components

```
┌─────────────────────────────────────────────────────────────┐
│                    Kubernetes Cluster                        │
├─────────────────────────────────────────────────────────────┤
│                                                               │
│  ┌────────────────────────────────────────────────────────┐ │
│  │          CSI Controller Pod (etcd-snapshot-driver)     │ │
│  │  ┌──────────────────┬──────────────────────────────┐   │ │
│  │  │ gRPC Server      │ Kubernetes Client            │   │ │
│  │  │ - GroupCtrl Svc  │ - Job creation               │   │ │
│  │  │ - Identity Svc   │ - ETCD resource discovery    │   │ │
│  │  │ - Probe          │ - VolumeGroupSnapshot mgmt   │   │ │
│  │  └──────────────────┴──────────────────────────────┘   │ │
│  └────────────────────────────────────────────────────────┘ │
│         ▲                                        ▼            │
│         │                        ┌────────────────────────┐  │
│         │                        │  Snapshot Job          │  │
│         │                        │  (etcdutl container)   │  │
│         │                        │  - snapshot save       │  │
│         │                        │  - snapshot restore    │  │
│         │                        │  - snapshot delete     │  │
│         │                        └────────────────────────┘  │
│         │                                      │              │
│         └──────────────────────────────────────┘              │
│                                                               │
│  ┌────────────────────────────────────────────────────────┐ │
│  │            ETCD Cluster (Backup Target)                │ │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐             │ │
│  │  │ ETCD Pod │  │ ETCD Pod │  │ ETCD Pod │             │ │
│  │  │(Leader)  │  │(Follower)│  │(Follower)│             │ │
│  │  └──────────┘  └──────────┘  └──────────┘             │ │
│  └────────────────────────────────────────────────────────┘ │
│                                                               │
└─────────────────────────────────────────────────────────────┘
         ▲                                        ▼
         │                        ┌────────────────────────┐
         │                        │ Velero (Optional)      │
         │                        │ DataMover integration  │
         └────────────────────────┘
```

### Component Descriptions

#### CSI GroupController Service
- **Role**: Primary component implementing CSI Volume Group Snapshot specification
- **Responsibilities**:
  - Respond to Volume Group Snapshot creation requests
  - Discover ETCD clusters associated with PVCs
  - Create Kubernetes Jobs for snapshot operations
  - Track snapshot metadata and status
  - Handle deletion requests with atomic cleanup
- **Deployment**: Single Pod (or HA with leader election)
- **Interface**: gRPC server on port 50051

#### Node Service
- **Role**: Not implemented (snapshots operate cluster-wide, not per-node)
- **Rationale**: ETCD snapshots are cluster-level operations, not node-local
- **Note**: Node Service identity will report empty capability

#### External Snapshotter Sidecar
- **Role**: Official Kubernetes component (not part of this project)
- **Responsibilities**:
  - Monitor VolumeGroupSnapshot objects
  - Invoke CSI GroupController Service RPC methods
  - Update VolumeGroupSnapshot status from driver responses
- **Reference**: kubernetes-csi/external-snapshotter

#### Job Executor
- **Role**: Kubernetes Job running etcdutl operations
- **Responsibilities**:
  - Execute snapshot save/restore/delete operations
  - Handle ETCD cluster authentication
  - Stream snapshot data to storage
  - Report job completion status
- **Image**: Custom image with etcdutl and supporting tools
- **Execution Context**: Namespace-scoped with minimal RBAC

### Data Flow Diagrams

#### Volume Group Snapshot Creation Flow
```
User/Velero
    │
    └─→ Create VolumeGroupSnapshot resource
            │
            ├─→ external-snapshotter watches
            │
            └─→ CSI GroupController.CreateVolumeGroupSnapshot() RPC
                    │
                    ├─→ For each volume in group:
                    │   ├─→ Discover ETCD cluster (labels/annotations)
                    │   ├─→ Retrieve ETCD credentials (secrets)
                    │   ├─→ Create Kubernetes Job
                    │   │   ├─→ Job runs: etcdutl snapshot save --endpoints=...
                    │   │   ├─→ Snapshot saved to mounted PVC
                    │   │   └─→ Job completes with status
                    │   └─→ Store snapshot metadata
                    │
                    └─→ Return CreateVolumeGroupSnapshotResponse
                            │
                            └─→ external-snapshotter updates VolumeGroupSnapshot.status
                                    │
                                    └─→ Group snapshot ready for use
```

#### Volume Group Snapshot Deletion Flow
```
User/Velero deletes VolumeGroupSnapshot
    │
    ├─→ external-snapshotter detects deletion
    │
    └─→ CSI GroupController.DeleteVolumeGroupSnapshot() RPC
            │
            ├─→ Clean up each snapshot in group
            │   └─→ Remove snapshot files and metadata
            │
            └─→ Return DeleteVolumeGroupSnapshotResponse (idempotent)
```
                    │
                    └─→ Return restoration status
```

### Design Decisions and Rationale

#### Decision: Job-Based Execution
**Choice**: Operations (save/restore/delete) run as Kubernetes Jobs rather than in-process

**Rationale**:
- Enables resource isolation and limits (memory, CPU, timeout)
- Provides audit trail through Job history
- Supports RBAC separation of concerns
- Allows container image updates without driver restart
- Provides job failure recovery and retry mechanisms

#### Decision: One Backup Per Cluster
**Choice**: Single snapshot captures entire multi-member ETCD cluster

**Rationale**:
- ETCD clusters maintain consistency internally; single member snapshot suffices
- Eliminates coordination complexity across multiple snapshots
- Reduces storage overhead
- Simplifies restoration workflow
- Matches ETCD backup best practices

#### Decision: Namespace Isolation
**Choice**: Snapshot Jobs run in same namespace as PVC

**Rationale**:
- Maintains relationship between snapshot and workload
- Enables per-namespace quotas and limits
- Simplifies RBAC (namespace-scoped roles)
- Follows Kubernetes best practices for multi-tenancy

#### Decision: Velero Integration via DataMover
**Choice**: Backup movement handled by external mechanisms

**Rationale**:
- Decouples snapshot creation from backup storage
- Enables flexibility in backup destinations
- Leverages proven Velero backup orchestration
- Reduces driver complexity

---

## 3. Prerequisites and Dependencies

### Kubernetes Environment Requirements

| Requirement | Minimum Version | Notes |
|---|---|---|
| **Kubernetes** | 1.20 | CSI v1.3.0 support required |
| **API Server** | 1.20 | VolumeSnapshot API (snapshot.storage.k8s.io/v1) |
| **Feature Gates** | N/A | No special feature gates required |
| **RBAC** | Enabled | Required for Job execution |
| **Storage** | Any CSI | PVC for snapshot storage required |

### External Dependencies

| Dependency | Version | Purpose |
|---|---|---|
| **CSI Specification** | 1.8.0 | API contract for driver implementation |
| **external-snapshotter** | 6.0+ | Kubernetes sidecar for Volume Snapshot handling |
| **ETCD** | 3.4+ | Backup target cluster |
| **etcdutl** | 3.4+ | Snapshot save/restore operations |
| **Velero** | 1.11+ | (Optional) Backup orchestration |

### Build and Development Dependencies

| Dependency | Version | Purpose |
|---|---|---|
| **Go** | 1.20+ | Project language |
| **Go Modules** | Latest | Dependency management |
| **Docker** | 20.10+ | Container image building |
| **Kubernetes** | 1.20+ | Integration testing |
| **CSI Sanity** | Latest | CSI compliance testing |

### Runtime Dependencies

| Dependency | Version | Purpose |
|---|---|---|
| **etcdutl Image** | Matches ETCD | Snapshot operations container |
| **PVC Storage** | N/A | Snapshot data storage (configurable) |
| **Velero** | 1.11+ | (Optional) Backup management |

---

## 4. CSI Driver Interface Specification

### Overview
The driver implements the CSI Specification v1.8.0 gRPC interface over Unix domain socket and/or TCP.

### Identity Service

#### GetPluginInfo
```go
rpc GetPluginInfo(GetPluginInfoRequest) returns (GetPluginInfoResponse) {}

message GetPluginInfoResponse {
  string name = 1;              // "etcd-snapshot-driver"
  string vendor_version = 2;    // "1.0.0"
  map<string, string> manifest = 3;
}
```

**Implementation**:
```bash
Name: etcd-snapshot-driver
Vendor Version: 1.0.0
```

#### GetPluginCapabilities
```go
rpc GetPluginCapabilities(GetPluginCapabilitiesRequest)
    returns (GetPluginCapabilitiesResponse) {}

message GetPluginCapabilitiesResponse {
  repeated PluginCapability capabilities = 1;
}

message PluginCapability {
  oneof type {
    Service service = 1;
    VolumeExpansion volume_expansion = 2;
  }
}

message Service {
  enum Type {
    UNKNOWN = 0;
    CONTROLLER_SERVICE = 1;
    VOLUME_ACCESSIBILITY_CONSTRAINTS = 2;
  }
  Type type = 1;
}
```

**Supported Capabilities**:
- CONTROLLER_SERVICE (required for snapshot operations)

#### Probe
```go
rpc Probe(ProbeRequest) returns (ProbeResponse) {}

message ProbeResponse {
  bool ready = 1;  // true if driver ready to serve requests
}
```

**Implementation**: Returns ready=true when Kubernetes client initialized

### GroupController Service

#### CreateVolumeGroupSnapshot

Creates an atomic group snapshot of multiple ETCD volumes.

**Parameters Accepted**:
- `name`: Group snapshot name (required)
- `source_volume_ids`: List of volume IDs in `namespace/pvc-name` format (required)

**Implementation**:
1. Validate request (name, at least one source volume)
2. For each volume: discover PVC, discover ETCD cluster, validate health
3. Create snapshot Jobs and monitor execution
4. Store metadata and return response
5. On partial failure: clean up all completed snapshots (atomic)

**Error Handling**:
- INVALID_ARGUMENT: Missing name or empty volume list
- FAILED_PRECONDITION: PVC not bound or ETCD cluster unhealthy
- INTERNAL: Job failure or ETCD communication error

#### DeleteVolumeGroupSnapshot

Deletes a group snapshot and all associated individual snapshots.

**Implementation**:
1. Validate snapshot ID
2. Clean up each snapshot in the group
3. **Idempotency**: Return success even if snapshot doesn't exist

#### GetVolumeGroupSnapshot

Retrieves status of a group snapshot.

#### GroupControllerGetCapabilities

**Supported Capabilities**:
- CREATE_DELETE_GET_VOLUME_GROUP_SNAPSHOT (required)

### Node Service

**Status**: Not Implemented
**Rationale**: Snapshot operations are cluster-level, not node-local

```go
rpc NodeGetCapabilities(NodeGetCapabilitiesRequest)
    returns (NodeGetCapabilitiesResponse) {
  // Returns empty capabilities
}
```

### gRPC Server Configuration

**Binding Address**: `unix:///var/lib/kubelet/plugins/etcd-snapshot-driver/csi.sock` (default)

**Port**: 50051 (for optional TCP listener)

**Timeout**: 300 seconds per operation (configurable)

**Connection Pool**: Kubernetes client connection reuse

---

## 5. ETCD Integration Specification

### Cluster Discovery Methods

The driver discovers ETCD clusters using multiple methods (in priority order):

#### Method 1: Label-Based Discovery (Recommended)
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: etcd-0
  labels:
    etcd.io/cluster: my-etcd-cluster
spec:
  containers:
  - name: etcd
    image: quay.io/coreos/etcd:v3.5.0
```

**Discovery Process**:
1. List Pods with label `etcd.io/cluster=<cluster-name>`
2. Filter by running status
3. Extract endpoints from pod DNS names

#### Method 2: Annotation-Based Discovery
```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: etcd-data
  annotations:
    etcd.io/cluster: my-etcd-cluster
    etcd.io/endpoints: etcd-0.etcd-headless:2379,etcd-1.etcd-headless:2379
spec:
  accessModes: [ReadWriteOnce]
  storageClassName: fast-ssd
  resources:
    requests:
      storage: 10Gi
```

**Discovery Process**:
1. Read annotations from PVC
2. Extract cluster name and endpoints
3. Validate endpoints are reachable

#### Method 3: CR-Based Discovery (StatefulSet)
```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: etcd
  labels:
    etcd.io/cluster: my-etcd-cluster
spec:
  serviceName: etcd-headless
  replicas: 3
  selector:
    matchLabels:
      etcd.io/cluster: my-etcd-cluster
  template:
    metadata:
      labels:
        etcd.io/cluster: my-etcd-cluster
    spec:
      containers:
      - name: etcd
        image: quay.io/coreos/etcd:v3.5.0
        ports:
        - containerPort: 2379
          name: client
```

### Multi-Instance Handling Strategy

**Single Snapshot Represents Entire Cluster**:
- Driver creates one snapshot per ETCD volume in a group snapshot request
- Snapshot contains complete ETCD cluster state
- Single member snapshot captures all data via internal replication
- Deletion removes snapshot (not ETCD data)

**Implementation**:
```go
type SnapshotMetadata struct {
  SnapshotID       string
  ClusterID        string
  ClusterName      string
  Endpoints        []string        // All cluster members
  CreationTime     time.Time
  Size             int64
  ChecksumSHA256   string
  SourceMember     string          // Which member created snapshot
  EtcdVersion      string
  ReadyToUse       bool
}
```

### Authentication Methods

#### Method 1: Client Certificates (Recommended)
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: etcd-client-certs
type: kubernetes.io/tls
data:
  tls.crt: <base64-client-cert>
  tls.key: <base64-client-key>
  ca.crt: <base64-ca-cert>
```

**etcdutl Usage**:
```bash
etcdutl \
  --endpoints=https://etcd-0.etcd-headless:2379 \
  --cert=/etc/etcd/certs/tls.crt \
  --key=/etc/etcd/certs/tls.key \
  --cacert=/etc/etcd/certs/ca.crt \
  snapshot save /snapshots/backup.db
```

#### Method 2: Basic Authentication
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: etcd-basic-auth
type: Opaque
data:
  username: <base64-username>
  password: <base64-password>
```

**etcdutl Usage**:
```bash
etcdutl \
  --endpoints=http://etcd-0.etcd-headless:2379 \
  --user=username:password \
  snapshot save /snapshots/backup.db
```

#### Method 3: No Authentication (Development Only)
```bash
etcdutl \
  --endpoints=http://etcd-0.etcd-headless:2379 \
  snapshot save /snapshots/backup.db
```

### Credential Access and RBAC

**ServiceAccount for Snapshot Jobs**:
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: etcd-snapshot-executor
  namespace: default  # Or target namespace
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: etcd-secret-reader
  namespace: default
rules:
- apiGroups: [""]
  resources: ["secrets"]
  verbs: ["get"]
  resourceNames: ["etcd-client-certs", "etcd-basic-auth"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: etcd-secret-reader
  namespace: default
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: etcd-secret-reader
subjects:
- kind: ServiceAccount
  name: etcd-snapshot-executor
  namespace: default
```

### etcdutl Command Specifications

#### Snapshot Save Command
```bash
etcdutl \
  --endpoints=https://etcd-0.etcd-headless:2379,https://etcd-1.etcd-headless:2379 \
  --cert=/etc/etcd/certs/tls.crt \
  --key=/etc/etcd/certs/tls.key \
  --cacert=/etc/etcd/certs/ca.crt \
  --dial-timeout=10s \
  snapshot save /snapshots/backup-$(date +%s).db
```

**Parameters**:
- `--endpoints`: Comma-separated list of ETCD cluster members
- `--cert`: Path to client certificate
- `--key`: Path to client private key
- `--cacert`: Path to CA certificate
- `--dial-timeout`: Connection timeout (default: 5s)
- Target path: Destination for snapshot file

**Output**: Snapshot file (binary format)

**Validation**:
```bash
etcdutl snapshot status /snapshots/backup.db
# Output:
# hash:        <hash>
# revision:    <rev>
# total key:   <count>
# total size:  <bytes>
```

#### Snapshot Restore Command
```bash
etcdutl snapshot restore /snapshots/backup.db \
  --name=etcd-0 \
  --initial-cluster=etcd-0=https://etcd-0.etcd-headless:2379,etcd-1=https://etcd-1.etcd-headless:2379 \
  --initial-cluster-token=etcd-cluster-$(date +%s) \
  --initial-advertise-peer-urls=https://etcd-0.etcd-headless:2380
```

**Parameters**:
- `--name`: Node name in cluster
- `--initial-cluster`: Cluster membership (name=url format)
- `--initial-cluster-token`: Unique token for new cluster
- `--initial-advertise-peer-urls`: Peer communication URL
- `--skip-hash-check`: Skip integrity check (not recommended)
- Target path: Output directory for restored data

#### Snapshot Delete Command
```bash
rm /snapshots/backup.db
```

**Note**: No etcdutl command needed; simple file deletion

### Container Image Requirements

**Recommended Image**: `quay.io/coreos/etcd:v3.5.0`

**Image Must Include**:
- `etcdutl` binary (available since ETCD v3.4)
- `etcdctl` binary (optional, for verification)
- `bash` or `sh` shell
- `curl` or `wget` (optional, for health checks)

**Custom Image Example**:
```dockerfile
FROM quay.io/coreos/etcd:v3.5.0
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
```

---

## 6. Kubernetes Resources Specification

### Job Templates for Snapshot Operations

#### Snapshot Save Job Template
```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: snapshot-save-{{ .SnapshotID }}
  namespace: {{ .Namespace }}
  labels:
    app: etcd-snapshot-driver
    snapshot-id: {{ .SnapshotID }}
    operation: save
spec:
  backoffLimit: 2
  ttlSecondsAfterFinished: 3600  # Clean up after 1 hour
  activeDeadlineSeconds: 600      # 10 minute timeout
  template:
    metadata:
      labels:
        app: etcd-snapshot-driver
        snapshot-id: {{ .SnapshotID }}
    spec:
      serviceAccountName: etcd-snapshot-executor
      restartPolicy: Never
      containers:
      - name: snapshot
        image: quay.io/coreos/etcd:v3.5.0
        imagePullPolicy: IfNotPresent
        command:
        - /bin/bash
        - -c
        - |
          set -e
          etcdutl \
            --endpoints={{ .EtcdEndpoints }} \
            {{ if .CertPath }}--cert={{ .CertPath }} {{ end }} \
            {{ if .KeyPath }}--key={{ .KeyPath }} {{ end }} \
            {{ if .CacertPath }}--cacert={{ .CacertPath }} {{ end }} \
            snapshot save /snapshots/backup.db

          # Validate snapshot
          etcdutl snapshot status /snapshots/backup.db

          echo "Snapshot completed successfully"
        volumeMounts:
        - name: snapshots
          mountPath: /snapshots
        - name: certs
          mountPath: /etc/etcd/certs
          readOnly: true
        resources:
          requests:
            memory: "256Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        securityContext:
          runAsNonRoot: true
          runAsUser: 1000
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          capabilities:
            drop: [ALL]
      volumes:
      - name: snapshots
        persistentVolumeClaim:
          claimName: {{ .SnapshotPVCName }}
      - name: certs
        secret:
          secretName: {{ .CertSecretName }}
          optional: true
```

#### Snapshot Delete Job Template
```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: snapshot-delete-{{ .SnapshotID }}
  namespace: {{ .Namespace }}
  labels:
    app: etcd-snapshot-driver
    snapshot-id: {{ .SnapshotID }}
    operation: delete
spec:
  backoffLimit: 1
  ttlSecondsAfterFinished: 600
  activeDeadlineSeconds: 300
  template:
    metadata:
      labels:
        app: etcd-snapshot-driver
        snapshot-id: {{ .SnapshotID }}
    spec:
      serviceAccountName: etcd-snapshot-executor
      restartPolicy: Never
      containers:
      - name: cleanup
        image: busybox:1.35
        command:
        - /bin/sh
        - -c
        - |
          rm -f /snapshots/backup.db
          echo "Snapshot deleted successfully"
        volumeMounts:
        - name: snapshots
          mountPath: /snapshots
        resources:
          requests:
            memory: "64Mi"
            cpu: "50m"
          limits:
            memory: "128Mi"
            cpu: "100m"
        securityContext:
          runAsNonRoot: true
          runAsUser: 1000
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          capabilities:
            drop: [ALL]
      volumes:
      - name: snapshots
        persistentVolumeClaim:
          claimName: {{ .SnapshotPVCName }}
```

#### Snapshot Restore Job Template
```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: snapshot-restore-{{ .SnapshotID }}
  namespace: {{ .Namespace }}
  labels:
    app: etcd-snapshot-driver
    snapshot-id: {{ .SnapshotID }}
    operation: restore
spec:
  backoffLimit: 1
  ttlSecondsAfterFinished: 3600
  activeDeadlineSeconds: 900      # 15 minute timeout
  template:
    metadata:
      labels:
        app: etcd-snapshot-driver
        snapshot-id: {{ .SnapshotID }}
    spec:
      serviceAccountName: etcd-snapshot-executor
      restartPolicy: Never
      containers:
      - name: restore
        image: quay.io/coreos/etcd:v3.5.0
        imagePullPolicy: IfNotPresent
        command:
        - /bin/bash
        - -c
        - |
          set -e
          CLUSTER_TOKEN="restore-$(date +%s)"

          etcdutl snapshot restore /snapshots/backup.db \
            --name={{ .EtcdNodeName }} \
            --initial-cluster={{ .InitialCluster }} \
            --initial-cluster-token=$CLUSTER_TOKEN \
            --initial-advertise-peer-urls={{ .PeerURL }} \
            --data-dir=/var/lib/etcd

          echo "Snapshot restored successfully"
        volumeMounts:
        - name: snapshots
          mountPath: /snapshots
        - name: etcd-data
          mountPath: /var/lib/etcd
        resources:
          requests:
            memory: "512Mi"
            cpu: "200m"
          limits:
            memory: "1Gi"
            cpu: "1000m"
        securityContext:
          runAsNonRoot: false  # etcdutl may need permissions
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: false
          capabilities:
            drop: [ALL]
      volumes:
      - name: snapshots
        persistentVolumeClaim:
          claimName: {{ .SnapshotPVCName }}
      - name: etcd-data
        emptyDir: {}
```

### RBAC Resources

#### Driver ServiceAccount and RBAC
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: etcd-snapshot-driver
  namespace: kube-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: etcd-snapshot-driver
rules:
# CSI driver requirements
- apiGroups: [""]
  resources: ["persistentvolumeclaims"]
  verbs: ["get", "list", "watch"]
- apiGroups: [""]
  resources: ["persistentvolumes"]
  verbs: ["get", "list"]
- apiGroups: ["snapshot.storage.k8s.io"]
  resources: ["volumesnapshots"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["snapshot.storage.k8s.io"]
  resources: ["volumesnapshotcontents"]
  verbs: ["get", "list", "watch"]
# Job creation and management
- apiGroups: ["batch"]
  resources: ["jobs"]
  verbs: ["create", "get", "list", "watch", "delete"]
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "list", "watch", "logs"]
# Pod discovery for ETCD clusters
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["list", "get"]
- apiGroups: [""]
  resources: ["services"]
  verbs: ["get", "list"]
# Secrets for ETCD credentials
- apiGroups: [""]
  resources: ["secrets"]
  verbs: ["get", "list"]
  resourceNames: ["etcd-client-certs", "etcd-basic-auth"]
# Events for debugging
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: etcd-snapshot-driver
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: etcd-snapshot-driver
subjects:
- kind: ServiceAccount
  name: etcd-snapshot-driver
  namespace: kube-system
```

#### Job Executor ServiceAccount
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: etcd-snapshot-executor
  namespace: default  # Namespace where Jobs run
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: etcd-snapshot-executor
  namespace: default
rules:
# Minimal permissions for snapshot Jobs
- apiGroups: [""]
  resources: ["secrets"]
  verbs: ["get"]
  resourceNames: ["etcd-client-certs", "etcd-basic-auth"]
- apiGroups: [""]
  resources: ["pods/log"]
  verbs: ["get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: etcd-snapshot-executor
  namespace: default
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: etcd-snapshot-executor
subjects:
- kind: ServiceAccount
  name: etcd-snapshot-executor
  namespace: default
```

### VolumeGroupSnapshotClass Example
```yaml
apiVersion: groupsnapshot.storage.k8s.io/v1beta2
kind: VolumeGroupSnapshotClass
metadata:
  name: etcd-group-snapshot-class
driver: etcd-snapshot-driver
deletionPolicy: Delete
parameters:
  snapshot-timeout: "300"
  storage-class: fast-ssd
---
apiVersion: groupsnapshot.storage.k8s.io/v1beta2
kind: VolumeGroupSnapshot
metadata:
  name: etcd-backup
  namespace: default
spec:
  volumeGroupSnapshotClassName: etcd-group-snapshot-class
  source:
    selector:
      matchLabels:
        app: etcd
```

### ConfigMap for Driver Configuration
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: etcd-snapshot-driver-config
  namespace: kube-system
data:
  config.yaml: |
    # Default snapshot timeout in seconds
    snapshot_timeout: 300

    # Default storage class for snapshots
    storage_class: standard

    # ETCD discovery configuration
    discovery:
      # Methods: label, annotation, statefulset
      methods:
      - label
      - annotation
      - statefulset

      # Label selector for ETCD cluster discovery
      label_selector:
        etcd.io/cluster: ""

    # Logging configuration
    logging:
      level: info
      format: json

    # Metrics configuration
    metrics:
      enabled: true
      port: 8080
```

---

## 7. Configuration Options and Parameters

### Driver-Level Configuration

| Flag | Env Variable | Default | Type | Description |
|---|---|---|---|---|
| `--csi-endpoint` | `CSI_ENDPOINT` | `unix:///var/lib/kubelet/plugins/etcd-snapshot-driver/csi.sock` | string | CSI gRPC endpoint |
| `--snapshot-timeout` | `SNAPSHOT_TIMEOUT` | `300` | int | Snapshot operation timeout (seconds) |
| `--storage-class` | `STORAGE_CLASS` | `standard` | string | Default storage class for snapshot PVCs |
| `--etcd-namespace` | `ETCD_NAMESPACE` | `default` | string | Default namespace for ETCD discovery |
| `--metrics-bind-address` | `METRICS_BIND_ADDRESS` | `:8080` | string | Metrics server bind address |
| `--log-level` | `LOG_LEVEL` | `info` | string | Log level (debug/info/warn/error) |
| `--kubeconfig` | `KUBECONFIG` | In-cluster | string | Path to kubeconfig file |
| `--leader-elect` | `LEADER_ELECT` | `false` | bool | Enable leader election for HA |
| `--job-backoff-limit` | `JOB_BACKOFF_LIMIT` | `2` | int | Job retry limit |
| `--job-active-deadline` | `JOB_ACTIVE_DEADLINE` | `600` | int | Job active deadline (seconds) |

### VolumeGroupSnapshotClass Parameters

| Parameter | Required | Type | Default | Description |
|---|---|---|---|---|
| `etcd-namespace` | No | string | `default` | ETCD cluster namespace |
| `etcd-cluster-name` | No | string | Auto-discovered | ETCD cluster name |
| `snapshot-timeout` | No | int | `300` | Operation timeout (seconds) |
| `storage-class` | No | string | `standard` | PVC storage class |
| `snapshot-label` | No | string | `""` | Additional label selector |
| `retention-days` | No | int | `30` | Snapshot retention period |

### Environment Variables

```bash
# CSI Configuration
export CSI_ENDPOINT=unix:///var/lib/kubelet/plugins/etcd-snapshot-driver/csi.sock
export CSI_LISTEN_TIMEOUT=30s

# ETCD Discovery
export ETCD_NAMESPACE=default
export ETCD_DISCOVERY_METHOD=label

# Job Configuration
export SNAPSHOT_TIMEOUT=300
export JOB_BACKOFF_LIMIT=2
export JOB_ACTIVE_DEADLINE=600

# Monitoring
export METRICS_ENABLED=true
export METRICS_PORT=8080

# Logging
export LOG_LEVEL=info
export LOG_FORMAT=json
```

### PVC Annotations for Discovery

```yaml
metadata:
  annotations:
    # ETCD cluster association
    etcd.io/cluster: my-etcd-cluster

    # Direct endpoint specification
    etcd.io/endpoints: etcd-0.etcd-headless:2379,etcd-1.etcd-headless:2379

    # Credentials secret name
    etcd.io/credentials-secret: etcd-client-certs

    # Snapshot prefix
    etcd.io/snapshot-prefix: backup-
```

---

## 8. Deployment Architecture

### Controller Deployment Configuration

#### Single-Replica Deployment
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: etcd-snapshot-driver
  namespace: kube-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: etcd-snapshot-driver
  template:
    metadata:
      labels:
        app: etcd-snapshot-driver
    spec:
      serviceAccountName: etcd-snapshot-driver
      priorityClassName: system-cluster-critical
      containers:
      - name: driver
        image: my-registry/etcd-snapshot-driver:v1.0.0
        imagePullPolicy: IfNotPresent
        args:
        - --csi-endpoint=unix:///csi/csi.sock
        - --v=4
        ports:
        - containerPort: 50051
          name: grpc
          protocol: TCP
        - containerPort: 8080
          name: metrics
          protocol: TCP
        env:
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        volumeMounts:
        - name: csi-socket
          mountPath: /csi
        - name: config
          mountPath: /etc/etcd-snapshot-driver
          readOnly: true
        resources:
          requests:
            memory: "256Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 10
          timeoutSeconds: 5
          failureThreshold: 3
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
          timeoutSeconds: 3
          failureThreshold: 2
        securityContext:
          runAsNonRoot: true
          runAsUser: 1000
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          capabilities:
            drop: [ALL]

      - name: csi-provisioner
        image: registry.k8s.io/sig-storage/csi-provisioner:v3.5.0
        imagePullPolicy: IfNotPresent
        args:
        - --csi-address=/csi/csi.sock
        - --v=4
        volumeMounts:
        - name: csi-socket
          mountPath: /csi
        resources:
          requests:
            memory: "128Mi"
            cpu: "50m"
          limits:
            memory: "256Mi"
            cpu: "200m"
        securityContext:
          runAsNonRoot: true
          runAsUser: 1000
          allowPrivilegeEscalation: false
          capabilities:
            drop: [ALL]

      - name: csi-snapshotter
        image: registry.k8s.io/sig-storage/csi-snapshotter:v6.2.1
        imagePullPolicy: IfNotPresent
        args:
        - --csi-address=/csi/csi.sock
        - --v=4
        volumeMounts:
        - name: csi-socket
          mountPath: /csi
        resources:
          requests:
            memory: "128Mi"
            cpu: "50m"
          limits:
            memory: "256Mi"
            cpu: "200m"
        securityContext:
          runAsNonRoot: true
          runAsUser: 1000
          allowPrivilegeEscalation: false
          capabilities:
            drop: [ALL]

      volumes:
      - name: csi-socket
        emptyDir: {}
      - name: config
        configMap:
          name: etcd-snapshot-driver-config
          optional: true
```

#### HA Deployment with Leader Election
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: etcd-snapshot-driver
  namespace: kube-system
spec:
  replicas: 3
  selector:
    matchLabels:
      app: etcd-snapshot-driver
  template:
    metadata:
      labels:
        app: etcd-snapshot-driver
    spec:
      serviceAccountName: etcd-snapshot-driver
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                - key: app
                  operator: In
                  values:
                  - etcd-snapshot-driver
              topologyKey: kubernetes.io/hostname
      containers:
      - name: driver
        image: my-registry/etcd-snapshot-driver:v1.0.0
        imagePullPolicy: IfNotPresent
        args:
        - --csi-endpoint=unix:///csi/csi.sock
        - --leader-elect
        - --leader-elect-namespace=kube-system
        - --v=4
        env:
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        volumeMounts:
        - name: csi-socket
          mountPath: /csi
        resources:
          requests:
            memory: "256Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "500m"

      # Additional sidecar containers...
      volumes:
      - name: csi-socket
        emptyDir: {}
```

### Container Specifications

#### Driver Container
- **Image**: `my-registry/etcd-snapshot-driver:v1.0.0`
- **Resource Requests**: 100m CPU, 256Mi memory
- **Resource Limits**: 500m CPU, 512Mi memory
- **Port**: 50051 (gRPC)
- **Health Check**: HTTP /healthz and /ready endpoints

#### External Snapshotter Sidecar
- **Image**: `registry.k8s.io/sig-storage/csi-snapshotter:v6.2.1`
- **Resource Requests**: 50m CPU, 128Mi memory
- **Resource Limits**: 200m CPU, 256Mi memory
- **Shared Volume**: CSI socket

### Network Requirements

**Egress**:
- ETCD cluster endpoints (port 2379 for client, 2380 for peer communication)
- Kubernetes API server (port 6443)
- Image registries (443 for container pulls)

**Ingress**:
- None required for CSI driver (gRPC over local socket)

**Internal Communication**:
- CSI driver ↔ external-snapshotter (unix socket)
- CSI driver ↔ Kubernetes API (authenticated)
- Snapshot Jobs ↔ ETCD cluster (network or local)

### Storage Requirements

| Component | Storage | Type | Purpose |
|---|---|---|---|
| **Snapshot PVC** | Configurable | Persistent | Stores snapshot backup files |
| **Driver State** | 100Mi | Temporary | Job metadata and configuration |
| **Logs** | 500Mi | Temporary | Driver and sidecar logs |

### High Availability Considerations

**Leader Election**:
- Use `--leader-elect` flag for single leader pattern
- Distributes snapshot requests across cluster
- Prevents concurrent Job creation for same cluster

**Pod Disruption Budget**:
```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: etcd-snapshot-driver
  namespace: kube-system
spec:
  minAvailable: 1
  selector:
    matchLabels:
      app: etcd-snapshot-driver
```

**Node Affinity**:
- Anti-affinity to spread replicas across nodes
- Ensures availability during node maintenance

---

## 9. API Contracts and Data Models

### Internal Data Models

#### SnapshotMetadata Structure
```go
type SnapshotMetadata struct {
  // Identifier for this snapshot
  SnapshotID        string    `json:"snapshot_id"`

  // Source volume information
  SourceVolumeID    string    `json:"source_volume_id"`
  SourceNamespace   string    `json:"source_namespace"`
  SourcePVCName     string    `json:"source_pvc_name"`

  // ETCD cluster information
  ClusterID         string    `json:"cluster_id"`
  ClusterName       string    `json:"cluster_name"`
  ClusterNamespace  string    `json:"cluster_namespace"`
  ClusterEndpoints  []string  `json:"cluster_endpoints"`
  SourceMember      string    `json:"source_member"`
  EtcdVersion       string    `json:"etcd_version"`

  // Snapshot state
  CreationTime      time.Time `json:"creation_time"`
  Size              int64     `json:"size_bytes"`
  ChecksumSHA256    string    `json:"checksum_sha256"`
  ReadyToUse        bool      `json:"ready_to_use"`
  Status            string    `json:"status"` // creating|ready|failed|deleting

  // Backup location
  BackupPath        string    `json:"backup_path"`
  StorageClassName  string    `json:"storage_class_name"`

  // Additional metadata
  CreatedBy         string    `json:"created_by"`
  Labels            map[string]string `json:"labels"`
  Annotations       map[string]string `json:"annotations"`
}
```

#### JobConfiguration Structure
```go
type JobConfiguration struct {
  // Job metadata
  JobName           string                  `json:"job_name"`
  Namespace         string                  `json:"namespace"`
  SnapshotID        string                  `json:"snapshot_id"`

  // Operation type
  Operation         string                  `json:"operation"` // save|delete|restore

  // ETCD configuration
  EtcdEndpoints     []string                `json:"etcd_endpoints"`
  EtcdAuth          EtcdAuthConfig          `json:"etcd_auth"`

  // Snapshot configuration
  SnapshotPath      string                  `json:"snapshot_path"`

  // Restore configuration (for restore operations)
  RestoreConfig     *RestoreConfiguration  `json:"restore_config,omitempty"`

  // Job execution settings
  Timeout           int                     `json:"timeout_seconds"`
  BackoffLimit      int                     `json:"backoff_limit"`
  ActiveDeadline    int                     `json:"active_deadline_seconds"`
}

type EtcdAuthConfig struct {
  Method            string                  `json:"method"` // none|basic|cert
  BasicUsername     string                  `json:"basic_username,omitempty"`
  BasicPassword     string                  `json:"basic_password,omitempty"`
  CertSecretName    string                  `json:"cert_secret_name,omitempty"`
  CertPath          string                  `json:"cert_path,omitempty"`
  KeyPath           string                  `json:"key_path,omitempty"`
  CacertPath        string                  `json:"cacert_path,omitempty"`
}

type RestoreConfiguration struct {
  NodeName          string                  `json:"node_name"`
  InitialCluster    string                  `json:"initial_cluster"`
  InitialClusterToken string                `json:"initial_cluster_token"`
  PeerURL           string                  `json:"peer_url"`
  DataDir           string                  `json:"data_dir"`
}
```

### Configuration Schemas

#### VolumeGroupSnapshotClass Parameters Schema
```yaml
type: object
properties:
  etcd-namespace:
    type: string
    description: "Kubernetes namespace containing ETCD cluster"
    default: "default"

  etcd-cluster-name:
    type: string
    description: "Name of ETCD cluster (auto-discovered if not set)"

  snapshot-timeout:
    type: integer
    description: "Snapshot operation timeout in seconds"
    minimum: 60
    maximum: 3600
    default: 300

  storage-class:
    type: string
    description: "Storage class for snapshot storage"
    default: "standard"

  snapshot-label:
    type: string
    description: "Additional label selector for cluster discovery"

  retention-days:
    type: integer
    description: "Retention period for snapshots in days"
    minimum: 1
    maximum: 3650
    default: 30
```

### Kubernetes API Interactions

#### Watch VolumeSnapshot Resources
```go
// CSI driver watches VolumeSnapshot objects
volumeSnapshotInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
  AddFunc: func(obj interface{}) {
    snapshot := obj.(*snapshotv1.VolumeSnapshot)
    driver.handleSnapshotCreation(snapshot)
  },
  DeleteFunc: func(obj interface{}) {
    snapshot := obj.(*snapshotv1.VolumeSnapshot)
    driver.handleSnapshotDeletion(snapshot)
  },
})
```

#### State Transitions
```
VolumeSnapshot.Status:
  Conditions:
  - Type: ReadyToUse
    Status: False → True (after snapshot creation completes)
  - Type: Progressing
    Status: True → False
  - Type: Error
    Status: False → True (if creation fails)

VolumeSnapshot.Status.BoundVolumeSnapshotContentName:
  Reference to VolumeSnapshotContent object containing snapshot ID
```

### CSI Error Code Mappings

| Scenario | CSI Error Code | gRPC Status | Description |
|---|---|---|---|
| PVC not found | INVALID_ARGUMENT | InvalidArgument | Source volume ID is invalid |
| ETCD cluster not found | NOT_FOUND | NotFound | ETCD cluster cannot be discovered |
| ETCD unreachable | FAILED_PRECONDITION | FailedPrecondition | Cannot connect to ETCD |
| Authentication failed | FAILED_PRECONDITION | FailedPrecondition | Invalid credentials or certificates |
| Job creation failed | INTERNAL | Internal | Kubernetes Job creation error |
| Snapshot storage full | RESOURCE_EXHAUSTED | ResourceExhausted | PVC storage quota exceeded |
| Operation timeout | DEADLINE_EXCEEDED | DeadlineExceeded | Snapshot operation timed out |
| Permission denied | PERMISSION_DENIED | PermissionDenied | RBAC check failed |

---

## 10. Snapshot Lifecycle Workflows

### Creation Workflow (10 Phases)

**Phase 1: Request Validation**
```
Input: CreateSnapshotRequest
├─ Validate source_volume_id format
├─ Check snapshot_name is unique
├─ Verify parameters are well-formed
└─ Output: Validation passed or INVALID_ARGUMENT error
```

**Phase 2: PVC Discovery**
```
Action: Locate PVC by source_volume_id
├─ Query Kubernetes PVC by name
├─ Validate PVC exists and is bound
├─ Extract namespace and storage class
└─ Output: PVC object or NOT_FOUND error
```

**Phase 3: ETCD Cluster Discovery**
```
Action: Identify ETCD cluster related to PVC
├─ Check PVC annotations for cluster reference
├─ Query Pods with label selector (etcd.io/cluster=...)
├─ Validate cluster is healthy (quorum check)
└─ Output: Cluster endpoints and version or NOT_FOUND error
```

**Phase 4: Credential Resolution**
```
Action: Obtain ETCD authentication credentials
├─ Check for explicit credential reference in parameters
├─ Query Secret (etcd-client-certs, etcd-basic-auth)
├─ Validate credentials are present and valid
└─ Output: Authentication config or FAILED_PRECONDITION error
```

**Phase 5: Snapshot Storage Preparation**
```
Action: Prepare storage for snapshot backup
├─ Create or verify snapshot PVC exists
├─ Check available storage space
├─ Ensure storage class supports RWO
└─ Output: Storage ready or RESOURCE_EXHAUSTED error
```

**Phase 6: Job Template Generation**
```
Action: Generate Kubernetes Job manifest
├─ Fill template with ETCD endpoints
├─ Include credential mounts
├─ Set timeout and resource limits
├─ Assign unique job name with snapshot ID
└─ Output: Job manifest YAML
```

**Phase 7: Job Creation**
```
Action: Submit Job to Kubernetes API
├─ Create Job resource in PVC namespace
├─ Verify Job accepted by API server
├─ Record Job UID for tracking
└─ Output: Job created or INTERNAL error
```

**Phase 8: Job Execution Monitoring**
```
Action: Poll Job status until completion
├─ Check Job.Status.Active (waiting for execution)
├─ Monitor Pod logs for progress
├─ Apply timeout mechanism (default 300s)
├─ Handle Job failure and retry
└─ Output: Job succeeded or failed with status
```

**Phase 9: Snapshot Validation**
```
Action: Validate snapshot integrity
├─ Run: etcdutl snapshot status /snapshots/backup.db
├─ Verify hash and revision
├─ Validate file size and permissions
├─ Compute checksums
└─ Output: Snapshot valid or INTERNAL error
```

**Phase 10: Response Generation**
```
Action: Return CSI CreateSnapshotResponse
├─ Populate snapshot_id (unique identifier)
├─ Set creation_time to current timestamp
├─ Set size_bytes from snapshot file
├─ Set ready_to_use=true if successful
└─ Output: CreateSnapshotResponse with snapshot metadata
```

### Deletion Workflow (5 Phases)

**Phase 1: Request Validation**
```
Input: DeleteSnapshotRequest
├─ Validate snapshot_id format
└─ Output: Validation passed or INVALID_ARGUMENT error
```

**Phase 2: Snapshot Lookup**
```
Action: Locate snapshot by ID
├─ Query snapshot metadata from storage
├─ Determine snapshot storage location
├─ Verify ownership and permissions
└─ Output: Snapshot info or proceed with idempotent deletion
```

**Phase 3: Cleanup Job Creation**
```
Action: Create Job to remove snapshot files
├─ Generate Job manifest
├─ Command: rm /snapshots/backup.db
├─ Submit Job to Kubernetes
└─ Output: Job UID or proceed direct deletion
```

**Phase 4: Cleanup Verification**
```
Action: Wait for cleanup completion
├─ Poll Job status (short timeout)
├─ Log any deletion errors (non-fatal)
├─ Verify snapshot file removed
└─ Output: Cleanup completed (best effort)
```

**Phase 5: Response Return**
```
Action: Return DeleteSnapshotResponse
├─ Always return success (idempotent)
├─ Log any errors that occurred
├─ Update snapshot status to deleted
└─ Output: Empty DeleteSnapshotResponse
```

**Idempotency Guarantee**: If snapshot already deleted, return success

### Restoration Workflow (4 Complex Phases)

**Phase 1: Restoration Request Preparation**
```
Input: User provision PVC from snapshot
├─ Kubernetes detects CreateSnapshot with dataSource
├─ CSI driver receives restore parameters
├─ Validate snapshot exists and is ready
└─ Output: Restoration job configuration
```

**Phase 2: Target ETCD Cluster Preparation**
```
Action: Prepare ETCD cluster for restoration
├─ Identify target cluster (same or different)
├─ Generate unique cluster token
├─ Configure cluster membership
├─ Prepare data directory
└─ Output: Cluster configuration or FAILED_PRECONDITION error
```

**Phase 3: Restore Job Execution**
```
Action: Execute restoration via Kubernetes Job
├─ Create Job with restore parameters:
│  ├─ snapshot_id=<source-snapshot>
│  ├─ etcdutl snapshot restore /snapshots/backup.db
│  ├─ --initial-cluster=<new-membership>
│  ├─ --initial-cluster-token=restore-<timestamp>
│  └─ --data-dir=/var/lib/etcd
├─ Execute in target namespace
├─ Monitor execution with timeout (900s)
└─ Output: Restoration completed or INTERNAL error
```

**Phase 4: Cluster Verification and Rollout**
```
Action: Verify restored cluster health
├─ Check restored ETCD cluster responds to requests
├─ Validate data integrity (key counts)
├─ Trigger ETCD Pod restart with new data
├─ Wait for cluster to reach quorum
└─ Output: Restoration verified or restoration failed
```

---

## 11. Velero Integration Specification

### Integration Architecture Overview

```
Velero Backup Controller
    │
    ├─→ Create Backup resource
    │
    ├─→ Discover PVCs in namespaces
    │
    ├─→ For each ETCD-backed PVC:
    │   ├─→ Create VolumeSnapshot via CSI
    │   │   └─→ CSI driver creates snapshot Job
    │   │
    │   └─→ Wait for VolumeSnapshotContent ready
    │
    ├─→ DataMover plugin transfers snapshot to backup storage
    │   └─→ Upload backup.db to S3/GCS/Azure/etc
    │
    └─→ Update Backup status
        └─→ Backup complete
```

### Required Velero Components

#### Velero Core Installation
```bash
velero install \
  --bucket=my-backup-bucket \
  --secret-file=./credentials-aws \
  --use-volume-snapshots=true
```

#### CSI Snapshotter Integration
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: velero
  namespace: velero
data:
  # Enable CSI snapshots
  features: |
    EnableCSI=true
    EnableCSISnapshots=true
```

#### DataMover Configuration
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: velero-datamovement
  namespace: velero
data:
  aws_access_key_id: <base64-key>
  aws_secret_access_key: <base64-secret>
  aws_endpoint: https://s3.example.com
```

### Backup Workflow with DataMover (6 Steps)

**Step 1: Backup Policy Triggers**
```
User creates backup via:
velero backup create my-backup --include-namespaces default
```

**Step 2: VolumeSnapshot Creation**
```
Velero discovers ETCD-backed PVCs
├─→ For each PVC: kubectl apply VolumeSnapshot
├─→ CSI driver receives CreateSnapshot RPC
├─→ Creates snapshot Job
└─→ Waits for VolumeSnapshotContent with snapshot_id
```

**Step 3: Snapshot Ready Check**
```
Poll VolumeSnapshot.Status.ReadyToUse
├─→ Continues when ReadyToUse=true
└─→ Records snapshot ID for later
```

**Step 4: DataMover Transfer**
```
Velero DataMover plugin:
├─→ Lists VolumeSnapshotContents
├─→ For each snapshot:
│   ├─→ Mount snapshot (if provider supports)
│   ├─→ Or retrieve snapshot data via CSI
│   ├─→ Stream data to backup destination
│   └─→ Verify checksum
└─→ Records snapshot location in backup metadata
```

**Step 5: Backup Metadata Recording**
```
Velero updates backup status:
├─→ Records all snapshot IDs
├─→ Records backup destination paths
├─→ Stores ETCD cluster metadata
└─→ Marks backup complete
```

**Step 6: Cleanup (Retention Policy)**
```
Velero retention controller:
├─→ Monitors backup age
├─→ After retention period expires
├─→ Deletes VolumeSnapshots
├─→ CSI driver cleanup (via DeleteSnapshot RPC)
└─→ Removes snapshot from backup storage
```

### Restore Workflow (4 Steps)

**Step 1: Restore Initiation**
```
User or backup schedule triggers restore:
velero restore create --from-backup=my-backup
```

**Step 2: PVC Recreation from Snapshot**
```
Velero creates PVCs:
├─→ For each ETCD PVC in backup:
│   ├─→ Create new PVC with dataSource pointing to snapshot
│   └─→ Kubernetes provision PVC from snapshot
└─→ CSI driver handles restoration via Job
```

**Step 3: ETCD Cluster Restoration**
```
CSI driver executes restore Job:
├─→ Download snapshot from backup storage
├─→ Run etcdutl snapshot restore
├─→ Reconfigure cluster membership
├─→ Restart ETCD pods with new data
└─→ Wait for cluster to reach quorum
```

**Step 4: Application Reconciliation**
```
Velero completes restore:
├─→ Recreate Pods/Deployments
├─→ Pods reconnect to restored ETCD
├─→ Service discovery updated
└─→ Restore complete
```

### Integration Requirements and Limitations

**Requirements**:
- Velero 1.11+ with CSI support enabled
- CSI snapshotter sidecar installed
- etcd-snapshot-driver deployed and configured
- Snapshot PVCs have persistent backing storage
- Network connectivity from Jobs to backup destination (for DataMover)

**Limitations**:
- Snapshots are cluster-wide (cannot snapshot individual keys/namespaces)
- Restoration is cluster-level (affects entire ETCD cluster, not individual PVCs)
- No incremental snapshots (each snapshot is full backup)
- No point-in-time recovery to arbitrary timestamps (only to snapshot boundaries)
- Requires application downtime during ETCD restoration (cluster members restart)
- Velero DataMover must have credentials for snapshot storage provider

---

## 12. Testing and Validation Requirements

### Unit Testing Requirements

**Coverage Target**: >80% code coverage for critical paths

**Critical Areas to Test**:
1. **ETCD Cluster Discovery**
   - Label-based discovery
   - Annotation-based discovery
   - Statefulset-based discovery
   - Error handling (cluster not found)

2. **CSI RPC Handlers**
   - CreateSnapshot (happy path, error cases)
   - DeleteSnapshot (idempotency)
   - GetPluginInfo
   - GetPluginCapabilities
   - Probe

3. **Job Generation and Execution**
   - Job manifest templating
   - Job status polling
   - Timeout handling
   - Failure recovery

4. **Credential Handling**
   - Secret retrieval
   - Certificate validation
   - Basic auth encoding

5. **Snapshot Validation**
   - Checksum computation
   - Size verification
   - Status checks

### Integration Testing Scenarios

**Test Case 1: End-to-End Snapshot Creation**
```
Setup:
- Deploy ETCD cluster with 3 members
- Deploy etcd-snapshot-driver
- Create snapshot PVC

Test:
1. Create VolumeSnapshot resource
2. Verify CSI driver receives CreateSnapshot RPC
3. Job created and executed
4. Snapshot file exists and validated
5. VolumeSnapshotContent created with snapshot_id

Expected Result: Snapshot ready within 5 minutes
```

**Test Case 2: Snapshot Deletion with Cleanup**
```
Setup:
- Existing snapshot from Test Case 1

Test:
1. Delete VolumeSnapshot resource
2. Verify DeleteSnapshot RPC called
3. Cleanup job executes
4. Snapshot file removed

Expected Result: Snapshot deleted within 2 minutes, idempotent
```

**Test Case 3: Snapshot Restoration**
```
Setup:
- Existing snapshot from Test Case 1
- New ETCD cluster deployed

Test:
1. Create new PVC with dataSource pointing to snapshot
2. CSI driver receives restore request
3. Restore Job executes
4. New ETCD cluster initialized from backup
5. Data integrity verified

Expected Result: Restoration completes in 10 minutes, data matches original
```

**Test Case 4: High Availability Snapshot**
```
Setup:
- ETCD cluster with 5 members
- 2 etcd-snapshot-driver replicas with leader election

Test:
1. Create snapshot (routed to leader)
2. Kill leader mid-snapshot
3. Observe failover to new leader
4. Snapshot completes successfully

Expected Result: Single snapshot created, completes after failover
```

**Test Case 5: Error Recovery**
```
Setup:
- ETCD cluster with authentication enabled
- Invalid credentials in secret

Test:
1. Attempt snapshot creation
2. Job fails due to auth error
3. Driver retries with backoff
4. Driver reports error to external-snapshotter

Expected Result: Job retried, error logged, VolumeSnapshot marked failed
```

**Test Case 6: Velero Integration**
```
Setup:
- Velero with CSI support
- etcd-snapshot-driver deployed
- ETCD cluster running

Test:
1. Create Velero backup
2. Velero discovers ETCD PVC
3. CSI driver creates snapshot
4. DataMover transfers snapshot to backup storage
5. Velero delete/recreate workflow
6. Restore from snapshot

Expected Result: Full backup/restore cycle completes successfully
```

### End-to-End Testing

**Full Lifecycle Test**:
```bash
# Deploy ETCD
kubectl apply -f etcd-cluster.yaml
kubectl rollout status statefulset/etcd

# Deploy CSI driver
kubectl apply -f csi-driver-deployment.yaml
kubectl rollout status deployment/etcd-snapshot-driver

# Create snapshot
kubectl apply -f volume-snapshot.yaml
kubectl wait --for=condition=ready volumesnapshot/etcd-backup

# Verify snapshot
kubectl get volumesnapshotcontent
etcdctl snapshot status /snapshots/backup.db

# Test deletion
kubectl delete volumesnapshot etcd-backup
kubectl wait --for=delete volumesnapshot/etcd-backup

# Test restoration
kubectl apply -f volume-snapshot-restore.yaml
kubectl wait --for=condition=ready volumesnapshot/etcd-restore
```

### Performance and Scalability Testing

**Metrics to Measure**:
- Snapshot creation time (target: <5 minutes for 1GB database)
- Snapshot size overhead (target: <110% of ETCD data size)
- Deletion time (target: <2 minutes)
- Restoration time (target: <15 minutes for 1GB database)
- Resource usage (CPU, memory under load)
- Concurrent snapshot handling (test 5, 10, 20 concurrent snapshots)

### CSI Conformance Testing

**Use csi-sanity**:
```bash
csi-sanity \
  --csi.endpoint=unix:///var/lib/kubelet/plugins/etcd-snapshot-driver/csi.sock \
  --ginkgo.focus="snapshot" \
  -csi.testvolumeparameters=driver-config.yaml
```

**Expected Results**: All tests pass without errors or warnings

### Pre-Release Validation Checklist

- [ ] All unit tests pass with >80% coverage
- [ ] All integration tests pass
- [ ] End-to-end lifecycle tested
- [ ] Performance tests pass with acceptable metrics
- [ ] CSI conformance tests pass (csi-sanity snapshot tests)
- [ ] Velero integration tested (backup/restore cycle)
- [ ] Security audit completed
- [ ] Documentation reviewed and updated
- [ ] Container image scanned for vulnerabilities
- [ ] HA failover tested
- [ ] Error scenarios tested
- [ ] Upgrade procedure tested (if applicable)
- [ ] Rollback procedure tested

---

## 13. Error Handling and Edge Cases

### Error Catalog

#### ETCD Cluster Discovery Failures

| Error | Cause | Recovery |
|---|---|---|
| ETCD_CLUSTER_NOT_FOUND | No pods matching label selector | Check cluster labels, verify pods running |
| ETCD_ENDPOINTS_UNREACHABLE | Cannot connect to any endpoint | Check network policy, firewall, service DNS |
| ETCD_QUORUM_LOST | Less than half members responding | Restore quorum before snapshot |
| ETCD_NO_LEADER | Cluster has no elected leader | Wait for leader election, retry |

#### Authentication/Authorization Failures

| Error | Cause | Recovery |
|---|---|---|
| AUTH_CERT_INVALID | Certificate expired or invalid | Regenerate and update secret |
| AUTH_CERT_NOT_FOUND | Certificate secret doesn't exist | Create secret with valid credentials |
| AUTH_PERMISSION_DENIED | Credentials lack permission | Update ETCD auth roles |
| AUTH_BASIC_INVALID | Username/password incorrect | Verify credentials in secret |

#### Storage Failures

| Error | Cause | Recovery |
|---|---|---|
| STORAGE_PVC_NOT_FOUND | Snapshot PVC doesn't exist | Create PVC with required size |
| STORAGE_QUOTA_EXCEEDED | PVC filled to capacity | Increase PVC size, clean old snapshots |
| STORAGE_WRITE_FAILED | Cannot write to mount | Check PVC health, permissions |

#### Job Execution Failures

| Error | Cause | Recovery |
|---|---|---|
| JOB_CREATION_FAILED | Kubernetes API error | Check API server, RBAC permissions |
| JOB_TIMEOUT | Job exceeded deadline | Increase timeout, check ETCD size |
| JOB_IMAGE_PULL_FAILED | Container image unavailable | Check image registry, credentials |
| JOB_POD_EVICTED | Node out of resources | Add resources, reduce concurrent jobs |

#### Snapshot Data Integrity Issues

| Error | Cause | Recovery |
|---|---|---|
| CHECKSUM_MISMATCH | Snapshot data corrupted | Delete snapshot, retry |
| SNAPSHOT_STATUS_FAILED | etcdutl validation failed | Verify ETCD cluster health |
| SNAPSHOT_INCOMPLETE | Snapshot file truncated | Check storage, retry save |

#### Concurrent Operation Conflicts

| Error | Cause | Recovery |
|---|---|---|
| SNAPSHOT_CREATION_IN_PROGRESS | Multiple create requests | Wait for first to complete |
| SNAPSHOT_DELETION_IN_PROGRESS | Delete while creating | Let current operation finish |
| CLUSTER_MODIFICATION_IN_PROGRESS | ETCD membership changing | Wait for membership stable |

### Edge Case Handling

#### Empty ETCD Cluster (No Data)
```
Behavior: Still creates valid snapshot
- Snapshot file created (minimal size)
- Checksum computed correctly
- Restoration results in empty cluster

Implementation:
if etcd_data_size == 0:
  return CreateSnapshotResponse(
    snapshot_id=generate_id(),
    ready_to_use=true,
    size_bytes=0
  )
```

#### Single Member Cluster
```
Behavior: Takes snapshot from sole member
- No quorum concerns
- Snapshot contains complete state

Implementation:
if len(cluster_endpoints) == 1:
  # Connect to single member
  snapshot_from_member(endpoints[0])
```

#### Leader Election During Snapshot
```
Behavior: Continue with new leader
- ETCD internal replication ensures consistency
- Job can connect to any member

Implementation:
connect_etcd(endpoints):
  for endpoint in shuffle(endpoints):
    try:
      return connect(endpoint)
    except ConnectionError:
      continue
```

#### Version Mismatch Between Members
```
Behavior: Use snapshot from any member
- ETCD handles version compatibility
- All members have same data

Implementation:
# Accept snapshot from any member
# Version extracted from snapshot metadata
if versions_differ:
  log.warn("ETCD members have different versions")
  # Proceed - replication ensures data consistency
```

#### Network Partition During Snapshot
```
Behavior: Snapshot fails, Job retries
- Kubernetes Job backoff handles retry
- User can retry snapshot manually

Implementation:
on_network_error():
  Job.retries++
  if Job.retries < backoff_limit:
    Job.retry_after(exponential_backoff())
  else:
    report_error(INTERNAL)
```

#### Node Failure During Snapshot Job
```
Behavior: Kubernetes reschedules Job
- Snapshot creation restarts on new node
- May create duplicate if original partially succeeded

Implementation:
# Ensure idempotency of snapshot creation
# Overwrite existing snapshot file if present
# Validate final result regardless of retries
```

#### Snapshot PVC Full During Save
```
Behavior: Job fails, error reported
- Storage exhaustion caught during execution
- User must expand PVC and retry

Implementation:
if snapshot_size > pvc_available:
  report_error(RESOURCE_EXHAUSTED)
  suggest_action("Expand PVC to at least " + required_size)
```

#### Simultaneous Snapshot Creation (Multiple VolumeSnapshots)
```
Behavior: HA-mode driver ensures single per cluster
- Leader election ensures one active creator
- Other drivers wait for completion

Implementation:
with leader_lock():
  if snapshot_in_progress(cluster_id):
    wait_for_completion()
  else:
    create_snapshot()
```

#### Restored Cluster Member Replacement
```
Behavior: Gradual member replacement post-restore
- Start with restored member only
- Add new members using etcdctl member commands
- Grow cluster to desired size

Implementation:
# Provide operational guidance
warn("Restored cluster has single member")
suggest("Use: etcdctl member add <name> --peer-urls=...")
```

---

## 14. Security Considerations

### Threat Model

**Assets**:
- ETCD cluster state (Kubernetes cluster secrets, config)
- Snapshot backup files (copies of cluster state)
- Encryption keys and certificates
- Credentials for ETCD authentication

**Threat Actors**:
- Compromised application containers
- Rogue cluster users with pod execution capability
- External attackers with network access
- Malicious cluster admins

**Threats**:
- Unauthorized snapshot creation/deletion
- Snapshot data theft (backup files exposed)
- Cluster state modification during restore
- Credential theft (certificates, passwords)
- Denial of service (snapshot quota exhaustion)

### Authentication and Authorization Approach

**CSI Driver Authentication**:
- Uses Kubernetes ServiceAccount bound to RBAC role
- Communicates with API server over secure channel (mTLS by default)
- Limited to resources required for operation

**ETCD Cluster Authentication**:
- Client certificates (recommended)
- Basic authentication (supported)
- No authentication (dev only)

**Snapshot Job Authorization**:
- ServiceAccount with minimal permissions
- Namespace-scoped role limiting secret access
- No cluster-admin privileges required

**RBAC Configuration** (see section 6):
```yaml
# CSI Driver - Cluster-level permissions
- List PVCs and Pods
- Create/manage Jobs
- Read cluster labels and annotations

# Snapshot Job - Namespace-level permissions
- Read only specific secret (credentials)
- No PVC creation/deletion
- No cross-namespace access
```

### Data Protection

#### In Transit
- **ETCD Communication**: TLS 1.3 encryption for client protocol (2379)
- **Peer Communication**: TLS for internal cluster replication (2380)
- **Snapshot Transfer**: Use TLS when uploading to backup storage
- **API Communication**: Kubernetes API server enforces TLS

**Implementation**:
```yaml
# Enable ETCD TLS
--cert-file=/etc/etcd/certs/etcd.crt
--key-file=/etc/etcd/certs/etcd.key
--client-cert-auth=true
--trusted-ca-file=/etc/etcd/certs/ca.crt
```

#### At Rest
- **Snapshots on PVC**: Should use encrypted storage volumes (e.g., EBS encryption)
- **Backup Storage**: Use provider encryption (S3 SSE, GCS encryption)
- **Secrets in etcd**: Use encryption at rest (--encryption-provider-config)

**Implementation**:
```yaml
# PVC with encrypted storage
storageClassName: encrypted-ssd
# Provider: AWS EBS encryption, GCP persistent disk encryption
```

#### Snapshot Isolation
- **Namespace Separation**: Jobs run in application namespace, isolated from other apps
- **RBAC Boundaries**: No cross-namespace secret access
- **PVC Isolation**: Each snapshot in separate PVC (or directory)

### RBAC Best Practices

```yaml
# Principle of Least Privilege
# Driver role includes only: list PVCs, create Jobs, read Pods
# Does not include: delete PVCs, modify RBAC, access secrets

# Namespace Isolation
# Snapshot Jobs in application namespace (not kube-system)
# Executor ServiceAccount scoped to single namespace

# Resource Quotas
# Snapshot operations bounded by namespace quotas
# Prevents single user from exhausting cluster resources
```

### Container Security

**Non-Root Execution**:
```yaml
securityContext:
  runAsNonRoot: true
  runAsUser: 1000      # Non-root user
  fsGroup: 2000
```

**Privilege Dropping**:
```yaml
securityContext:
  allowPrivilegeEscalation: false
  capabilities:
    drop: [ALL]
```

**Read-Only Filesystem**:
```yaml
securityContext:
  readOnlyRootFilesystem: true
```

**Resource Limits**:
```yaml
resources:
  requests:
    memory: 256Mi
    cpu: 100m
  limits:
    memory: 512Mi
    cpu: 500m
```

**Image Security**:
- Use image tags (not latest)
- Sign container images (Notary, Cosign)
- Scan for vulnerabilities (Trivy, Clair)
- Use private registries with authentication

### Audit and Compliance Logging

**Kubernetes Audit Events**:
```json
{
  "level": "RequestResponse",
  "auditID": "...",
  "stage": "ResponseComplete",
  "requestObject": {
    "apiVersion": "snapshot.storage.k8s.io/v1",
    "kind": "VolumeSnapshot",
    "metadata": {
      "name": "etcd-backup",
      "namespace": "default"
    }
  },
  "responseStatus": {
    "code": 201
  },
  "user": {
    "username": "system:serviceaccount:kube-system:etcd-snapshot-driver"
  }
}
```

**Driver Logging** (structured format):
```json
{
  "timestamp": "2024-01-15T10:30:45Z",
  "level": "INFO",
  "component": "csi-controller",
  "operation": "CreateSnapshot",
  "snapshot_id": "snap-12345",
  "cluster_name": "my-etcd",
  "user": "system:serviceaccount:default:etcd-snapshot-executor",
  "status": "success",
  "duration_seconds": 45
}
```

**Compliance Requirements**:
- All snapshot operations logged with timestamps
- Logs include user/service account performing operation
- Logs archived for compliance retention period
- Alerts on unauthorized access attempts

### Velero Backup Security

**Backup Encryption**:
```bash
velero backup create my-backup \
  --encryption-key-arn=arn:aws:kms:...
```

**Access Control**:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: velero-credentials
data:
  # Restricted IAM credentials
  # Only allow: s3:PutObject, s3:GetObject, s3:DeleteObject
```

**Backup Integrity**:
- Checksums computed and verified
- Encryption keys managed by key management service
- Access logs maintained for audit

### Security Checklist

- [ ] RBAC configured with least privilege principle
- [ ] TLS 1.3 enabled for all ETCD communication
- [ ] Client certificates deployed and validated
- [ ] Snapshots stored on encrypted volumes
- [ ] Backup storage uses encryption at rest
- [ ] Container images scanned for vulnerabilities
- [ ] Non-root execution enforced
- [ ] Privilege escalation disabled
- [ ] Read-only filesystem enabled
- [ ] Resource limits enforced
- [ ] Audit logging configured
- [ ] Secret credentials rotated regularly
- [ ] Backup encryption enabled
- [ ] Access controls documented
- [ ] Incident response procedures in place

---

## 15. Monitoring and Observability

### Prometheus Metrics

#### Snapshot Operation Metrics
```
# Counter: Total snapshot operations by type and status
etcd_snapshot_operations_total{
  operation="save|delete|restore",
  status="success|failure",
  cluster="my-etcd"
}

# Histogram: Snapshot operation duration in seconds
etcd_snapshot_operation_duration_seconds_bucket{
  operation="save|delete|restore",
  le="0.1|0.5|1|5|10|30|60|300|..."
}

# Gauge: Current snapshot size in bytes
etcd_snapshot_size_bytes{
  snapshot_id="snap-12345",
  cluster="my-etcd"
}

# Counter: Total snapshots by cluster
etcd_snapshots_total{
  cluster="my-etcd",
  status="ready|failed|deleting"
}
```

#### RPC Call Metrics
```
# Counter: CSI RPC calls by method
csi_rpc_calls_total{
  method="CreateSnapshot|DeleteSnapshot|GetPluginInfo",
  status="success|error"
}

# Histogram: CSI RPC latency
csi_rpc_latency_seconds_bucket{
  method="CreateSnapshot|DeleteSnapshot|...",
  le="0.1|0.5|1|5|10|30|..."
}

# Gauge: Active RPC calls
csi_rpc_active_calls{
  method="CreateSnapshot|DeleteSnapshot|..."
}
```

#### Storage Metrics
```
# Gauge: Snapshot PVC usage
etcd_snapshot_pvc_usage_bytes{
  pvc_name="etcd-snapshots",
  namespace="default"
}

# Gauge: Snapshot PVC available space
etcd_snapshot_pvc_available_bytes{
  pvc_name="etcd-snapshots",
  namespace="default"
}

# Counter: Storage errors
etcd_snapshot_storage_errors_total{
  error_type="quota_exceeded|write_failed|..."
}
```

#### ETCD Health Metrics
```
# Gauge: Connected ETCD members
etcd_cluster_members{
  cluster="my-etcd",
  state="up|down"
}

# Gauge: ETCD cluster has quorum
etcd_cluster_has_quorum{
  cluster="my-etcd"
}

# Histogram: ETCD operation latency
etcd_operation_latency_seconds{
  operation="get|put|delete",
  le="0.01|0.05|0.1|0.5|1|..."
}
```

### Structured Logging Format

**Log Entry Example**:
```json
{
  "timestamp": "2024-01-15T10:30:45.123Z",
  "level": "INFO",
  "component": "csi-controller",
  "operation": "CreateSnapshot",
  "snapshot_id": "snap-12345",
  "source_volume_id": "pvc-67890",
  "cluster_name": "my-etcd",
  "cluster_members": 3,
  "duration_seconds": 45.2,
  "status": "success",
  "message": "Snapshot created successfully",
  "trace_id": "d8f1e2c3-4567-890a-bcde-f1234567890a"
}
```

**Critical Events to Log**:
- Snapshot creation started/completed/failed
- ETCD cluster discovery
- Authentication failures
- Job creation/completion
- Storage errors
- Timeout events
- Recovery actions

### Health Checks

**Liveness Probe** (HTTP GET /healthz):
```go
// Returns 200 OK if driver process is running
// Returns 503 if unresponsive
healthz:
  httpGet:
    path: /healthz
    port: 8080
  initialDelaySeconds: 10
  periodSeconds: 10
  timeoutSeconds: 5
  failureThreshold: 3
```

**Readiness Probe** (HTTP GET /ready):
```go
// Returns 200 OK if:
// - Kubernetes client initialized
// - Snapshot PVC accessible
// - ETCD cluster discoverable
// Returns 503 if any check fails
readinessProbe:
  httpGet:
    path: /ready
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 5
  timeoutSeconds: 3
  failureThreshold: 2
```

### Alerting Rules

**Critical Alerts** (page on-call):
```yaml
alert: SnapshotCreationFailure
expr: rate(etcd_snapshot_operations_total{status="failure"}[5m]) > 0
for: 5m
annotations:
  summary: "ETCD snapshot creation failing"

alert: SnapshotPVCFull
expr: (etcd_snapshot_pvc_usage_bytes / etcd_snapshot_pvc_size_bytes) > 0.9
for: 10m
annotations:
  summary: "Snapshot PVC {{ $labels.pvc_name }} {{ $value | humanizePercentage }} full"

alert: ETCDClusterUnhealthy
expr: etcd_cluster_has_quorum == 0
for: 2m
annotations:
  summary: "ETCD cluster {{ $labels.cluster }} lost quorum"

alert: SnapshotJobTimeout
expr: increase(etcd_snapshot_job_timeout_total[1h]) > 0
for: 1m
annotations:
  summary: "Snapshot jobs timing out, investigate performance"
```

**Warning Alerts** (log and notify):
```yaml
alert: SnapshotSlowCreation
expr: histogram_quantile(0.95, etcd_snapshot_operation_duration_seconds_bucket{operation="save"}) > 300
for: 15m
annotations:
  summary: "95th percentile snapshot creation > 5 minutes"

alert: ETCDClusterLargeSnapshot
expr: etcd_snapshot_size_bytes > 10 * 1024 * 1024 * 1024  # 10GB
annotations:
  summary: "ETCD snapshot {{ $labels.snapshot_id }} exceeds 10GB"

alert: SnapshotJobHighErrorRate
expr: (rate(etcd_snapshot_operations_total{status="failure"}[15m]) / rate(etcd_snapshot_operations_total[15m])) > 0.1
for: 15m
annotations:
  summary: "Snapshot operation error rate > 10%"
```

### Grafana Dashboard Specifications

**Dashboard: ETCD Snapshot Driver Overview**
```
Row 1: Key Metrics
- Snapshots created (today, 7-day, 30-day counts)
- Total snapshots by status (ready, failed, deleting)
- Snapshot storage usage (PVC util %, available space)
- Average snapshot size (last 10 snapshots)

Row 2: Operation Performance
- Snapshot creation latency (50th, 95th, 99th percentile)
- Snapshot creation duration timeline (last 24h)
- Operation success rate (timeline)
- Operation error rate by type

Row 3: ETCD Cluster Health
- ETCD members up/down count by cluster
- ETCD cluster quorum status
- Failed API calls to ETCD (by cluster)
- ETCD operation latency

Row 4: Kubernetes Resources
- CSI driver pod CPU/memory usage
- CSI driver pod restart count
- Job execution count (running, completed, failed)
- PVC storage utilization

Row 5: Alerts and Events
- Active alerts
- Recent snapshot operation failures
- Recent storage errors
- Configuration changes
```

### OpenTelemetry Tracing (Optional)

**Trace Context Propagation**:
```go
// Include trace ID in all logs
logEntry.SetString("trace_id", span.SpanContext().TraceID().String())

// Span for CreateSnapshot operation
span := tracer.Start(ctx, "CreateSnapshot",
  attribute.String("snapshot_id", req.SnapshotName),
  attribute.String("source_volume_id", req.SourceVolumeId),
)
defer span.End()

// Child spans for sub-operations
discoverSpan := tracer.Start(ctx, "DiscoverCluster")
jobSpan := tracer.Start(ctx, "ExecuteJob")
validationSpan := tracer.Start(ctx, "ValidateSnapshot")
```

**Trace Sampling**:
- Always sample error traces
- Sample 5% of successful traces (configurable)
- Sample 100% of slow operations (>100s)

---

## 16. Operational Procedures

### Installation Steps

**Prerequisites Check**:
```bash
# 1. Verify Kubernetes version
kubectl version --short  # Must be 1.20+

# 2. Check API server for VolumeSnapshot API
kubectl api-resources | grep volumesnapshot

# 3. Verify external-snapshotter is running
kubectl get deployment -n kube-system | grep csi-snapshotter

# 4. Check for ETCD cluster
kubectl get pods -A -l etcd.io/cluster

# 5. Verify RBAC enabled
kubectl auth can-i create clusterroles --as=system:serviceaccount:kube-system:default
```

**Installation Procedure**:
```bash
# 1. Create namespace
kubectl create namespace csi-drivers

# 2. Deploy external-snapshotter (if not present)
kubectl apply -f external-snapshotter.yaml

# 3. Create RBAC
kubectl apply -f csi-rbac.yaml

# 4. Deploy etcd-snapshot-driver
kubectl apply -f csi-driver-deployment.yaml

# 5. Create VolumeGroupSnapshotClass
kubectl apply -f deploy/base/volume-group-snapshot-class.yaml

# 6. Verify deployment
kubectl rollout status deployment/etcd-snapshot-driver -n kube-system
kubectl get csidriver
kubectl get volumegroupsnapshotclass

# 7. Test driver
kubectl apply -f test-snapshot.yaml
kubectl wait --for=condition=ready volumesnapshot/test-snapshot --timeout=5m
```

### Upgrade and Rollback Procedures

**Upgrade Procedure**:
```bash
# 1. Drain in-progress snapshots
kubectl patch deployment etcd-snapshot-driver -p \
  '{"spec":{"template":{"spec":{"terminationGracePeriodSeconds":300}}}}'

# 2. Update image in deployment
kubectl set image deployment/etcd-snapshot-driver \
  driver=my-registry/etcd-snapshot-driver:v1.1.0

# 3. Wait for rollout
kubectl rollout status deployment/etcd-snapshot-driver

# 4. Verify new version
kubectl logs -f deployment/etcd-snapshot-driver | grep "version"

# 5. Run validation
kubectl apply -f test-snapshot-upgrade.yaml
```

**Rollback Procedure**:
```bash
# 1. Detect failure (alert triggered or manual check)

# 2. Roll back deployment
kubectl rollout undo deployment/etcd-snapshot-driver

# 3. Wait for rollout
kubectl rollout status deployment/etcd-snapshot-driver

# 4. Verify previous version
kubectl logs deployment/etcd-snapshot-driver | grep "version"

# 5. Investigate cause
kubectl describe deployment etcd-snapshot-driver
kubectl logs deployment/etcd-snapshot-driver --tail=200
```

### Backup and Disaster Recovery Scenarios

**Scenario 1: Backup ETCD with CSI Snapshot**
```bash
# 1. Create VolumeSnapshot
kubectl apply -f backup-snapshot.yaml

# 2. Wait for ready
kubectl wait --for=condition=ready volumesnapshot/etcd-backup --timeout=10m

# 3. Verify snapshot
kubectl get volumesnapshotcontent
etcdctl snapshot status /snapshots/backup.db

# 4. Copy to external storage (manual or Velero)
kubectl exec -it job/snapshot-save-* -- \
  gsutil cp /snapshots/backup.db gs://my-backups/

# 5. Verify backup
gsutil ls -h gs://my-backups/backup.db
```

**Scenario 2: Restore ETCD from Snapshot**
```bash
# 1. Identify snapshot to restore
kubectl get volumesnapshotcontent -o json | grep snapshotHandle

# 2. Create restore PVC
kubectl apply -f restore-pvc.yaml

# 3. Create restore VolumeSnapshot
kubectl apply -f restore-snapshot.yaml

# 4. Wait for restoration
kubectl wait --for=condition=ready volumesnapshot/etcd-restore --timeout=15m

# 5. Verify restored cluster
kubectl exec -it deployment/etcd -- \
  etcdctl --endpoints=localhost:2379 endpoint health

# 6. Verify data
etcdctl get --prefix ""  # List all keys
```

**Scenario 3: ETCD Data Corruption Recovery**
```bash
# 1. Detect corruption
kubectl logs deployment/etcd | grep -i corrupt

# 2. Scale down ETCD
kubectl scale statefulset etcd --replicas=0

# 3. Delete PVC data
kubectl delete pvc etcd-data-etcd-0
kubectl delete pvc etcd-data-etcd-1
kubectl delete pvc etcd-data-etcd-2

# 4. Restore from snapshot
kubectl apply -f restore-from-backup.yaml

# 5. Scale up ETCD
kubectl scale statefulset etcd --replicas=3

# 6. Verify recovery
kubectl rollout status statefulset/etcd
kubectl exec deployment/etcd -- etcdctl member list
```

### Routine Maintenance Schedule

| Task | Frequency | Owner | Duration |
|---|---|---|---|
| Verify snapshot creation success rate | Daily | Ops | 5 min |
| Check snapshot PVC usage trend | Weekly | Ops | 10 min |
| Rotate ETCD client certificates | Quarterly | Security | 30 min |
| Test snapshot restoration | Monthly | Ops | 1 hour |
| Update etcdutl container image | Quarterly | Ops | 30 min |
| Review audit logs | Weekly | Security | 20 min |
| Capacity planning review | Quarterly | Ops | 1 hour |
| Disaster recovery drill | Quarterly | Ops | 4 hours |

### Troubleshooting Guide

#### Problem: VolumeSnapshot stuck in "Pending"
```
Diagnosis:
1. Check CSI driver pod status
   kubectl describe deployment etcd-snapshot-driver

2. Check external-snapshotter logs
   kubectl logs -l app=csi-snapshotter | grep -i error

3. Verify snapshot class exists
   kubectl get volumesnapshotclass

4. Check CSI socket accessible
   kubectl exec deployment/etcd-snapshot-driver -- \
     ls -la /var/lib/kubelet/plugins/etcd-snapshot-driver/

Resolution:
- Restart CSI driver pod
- Verify RBAC permissions
- Check cluster resources (CPU, memory)
- Increase snapshot timeout
```

#### Problem: Snapshot creation times out
```
Diagnosis:
1. Check ETCD cluster health
   kubectl exec deployment/etcd -- etcdctl endpoint health

2. Check ETCD size
   kubectl exec deployment/etcd -- du -sh /var/lib/etcd

3. Check snapshot job logs
   kubectl logs job/snapshot-save-*

4. Monitor network latency
   kubectl exec job/snapshot-save-* -- ping -c 5 etcd-0

Resolution:
- Increase snapshot-timeout parameter
- Check network performance
- Reduce ETCD size (compaction, cleanup)
- Verify storage performance
```

#### Problem: Snapshot deletion fails
```
Diagnosis:
1. Check cleanup job status
   kubectl describe job snapshot-delete-*

2. Check PVC mount
   kubectl exec job/snapshot-delete-* -- df -h

3. Check file permissions
   kubectl exec job/snapshot-delete-* -- ls -la /snapshots/

Resolution:
- Manually delete snapshot: kubectl delete pvc etcd-snapshots
- Check for locked files
- Verify job RBAC permissions
- Force pod deletion if stuck
```

#### Problem: ETCD cluster not discovered
```
Diagnosis:
1. Verify cluster labels
   kubectl get pods -A --show-labels | grep etcd

2. Check PVC annotations
   kubectl get pvc -A -o yaml | grep etcd.io

3. Check driver logs
   kubectl logs deployment/etcd-snapshot-driver | grep -i discover

Resolution:
- Add labels to ETCD pods
- Add annotations to PVC
- Check namespace permissions
- Verify ServiceAccount RBAC
```

---

## 17. Known Limitations and Future Enhancements

### Current Limitations

#### Functional Limitations
- **No Incremental Snapshots**: Each snapshot is full backup (no delta)
- **No Point-in-Time Recovery**: Can only restore to snapshot boundaries
- **Single Snapshot Per Request**: Cannot create multiple snapshots in parallel
- **Cluster-Level Only**: Cannot snapshot individual keys/databases
- **No Selective Restore**: Restore is all-or-nothing

#### Performance Limitations
- **Snapshot Time**: Linear with ETCD database size (approximately 1 min per GB)
- **Concurrent Limit**: Driver can handle ~10 concurrent snapshots per replica
- **Storage Overhead**: Snapshots require 1.5x-2x ETCD data size for safe margin
- **Network Latency Sensitivity**: Performance degrades with high latency to ETCD

#### Compatibility Limitations
- **ETCD Versions**: Only ETCD 3.4+ supported (requires etcdutl)
- **Kubernetes Versions**: Minimum 1.20 (CSI v1.3.0)
- **Storage Classes**: Must support RWO access mode
- **Authentication**: No mTLS mutual auth (only client certs)

#### Security Limitations
- **Backup Encryption**: No built-in encryption (rely on storage provider)
- **Key Rotation**: Manual certificate rotation required
- **Audit Trail**: Limited to Kubernetes audit logs
- **Multi-Tenancy**: No namespace-level snapshot isolation

### Future Enhancements

#### Version 1.1 (Near-term, 3-6 months)
- **Incremental Snapshots**: Delta backup support via ETCD revision tracking
- **Snapshot Scheduling**: Built-in retention policies and automatic cleanup
- **Metrics Enhancements**: Additional Prometheus metrics and dashboards
- **Performance**: Parallel member snapshots with aggregation
- **CLI Tool**: kubectl plugin for snapshot management

**Implementation Approach**: Add retention controller, enhance metrics, optimize parallelization

#### Version 1.2 (Mid-term, 6-12 months)
- **Advanced Scheduling**: Cron-based snapshot scheduling in-driver
- **Point-in-Time Recovery**: Leverage ETCD revision tracking for recovery
- **Encryption Integration**: Support for KMS-backed snapshot encryption
- **Multi-Cluster**: Snapshots across federated ETCD clusters
- **UI Dashboard**: Web interface for snapshot management

**Implementation Approach**: Add scheduling service, implement revision recovery, add crypto providers

#### Version 2.0 (Long-term, 12+ months)
- **Native Backup Format**: Custom optimized backup format vs raw db
- **Snapshot Deduplication**: Content-addressable storage for snapshots
- **Streaming Restore**: Incremental restore from backup stream
- **Database Optimization**: Per-database snapshots (if future ETCD supports)
- **Advanced Security**: mTLS mutual authentication, audit signing

**Implementation Approach**: Design new backup format, implement deduplication, enhance restore workflow

### Community Feature Requests

- Prometheus/OpenTelemetry native instrumentation
- Support for ETCD snapshots with embedded secrets encryption
- Integration with popular backup solutions (Ark, Kasten)
- Windows container support
- ARM64 container image builds
- High-availability active-active (not just active-passive)

### Explicit Non-Goals

- **Real-Time Replication**: Not a streaming replication backup
- **Bi-Directional Sync**: One-way backup only
- **Schema Evolution**: No support for backward/forward compatibility
- **Compression**: Assume storage provider handles compression
- **Cloud Provider Integrations**: Generic Kubernetes approach
- **GUI/Dashboard**: Focus on programmatic and Kubernetes native interfaces
- **Data Transformation**: No data migration or transformation
- **Cost Optimization**: Assume user optimizes for their infrastructure

---

## 18. References and Resources

### Specifications and Standards

**Container Storage Interface**
- [CSI Specification v1.8.0](https://github.com/container-storage-interface/spec/releases/tag/v1.8.0)
- [CSI Snapshot API](https://kubernetes.io/docs/concepts/storage/volume-snapshots/)
- [Kubernetes Volume Snapshot Controller](https://github.com/kubernetes-csi/external-snapshotter)

**Kubernetes Documentation**
- [Kubernetes Storage Classes](https://kubernetes.io/docs/concepts/storage/storage-classes/)
- [Persistent Volumes](https://kubernetes.io/docs/concepts/storage/persistent-volumes/)
- [RBAC Authorization](https://kubernetes.io/docs/reference/access-authn-authz/rbac/)
- [Pod Security Context](https://kubernetes.io/docs/tasks/configure-pod-container/security-context/)

**ETCD Documentation**
- [ETCD Official Documentation](https://etcd.io/docs/)
- [ETCD v3.5 API](https://etcd.io/docs/v3.5/api/)
- [ETCD Backup and Restore](https://etcd.io/docs/v3.5/op-guide/maintenance/)
- [etcdutl Command](https://etcd.io/docs/v3.5/dev-guide/interacting_v3/#snapshot)

### Related Projects

**External Snapshotter**
- [kubernetes-csi/external-snapshotter](https://github.com/kubernetes-csi/external-snapshotter)
- [Snapshotter Sidecar Design](https://github.com/kubernetes-csi/external-snapshotter/blob/master/README.md)

**Velero**
- [Velero Project](https://velero.io/)
- [Velero CSI Plugin](https://velero.io/docs/latest/csi/)
- [DataMover Integration](https://velero.io/blog/velero-v1.11-data-mover/)

**Reference Implementations**
- [AWS EBS CSI Driver](https://github.com/kubernetes-sigs/aws-ebs-csi-driver)
- [OpenStack Cinder CSI](https://github.com/kubernetes/cloud-provider-openstack/tree/master/pkg/csi/cinder)
- [GCP PD CSI Driver](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver)

### Tools

**Testing and Validation**
- [csi-sanity](https://github.com/kubernetes-csi/csi-test/blob/master/cmd/csi-sanity/README.md): CSI driver compliance testing
- [kubectl-snapshot](https://github.com/volumesnapshot/kubectl-snapshot): VolumeSnapshot kubectl plugin
- [etcdctl](https://github.com/etcd-io/etcd/tree/main/tools/etcdctl): ETCD command-line client

**Monitoring and Debugging**
- [Prometheus](https://prometheus.io/docs/): Metrics collection
- [Grafana](https://grafana.com/docs/): Metrics visualization
- [Jaeger](https://www.jaegertracing.io/): Distributed tracing
- [kubectl-logs](https://kubernetes.io/docs/reference/kubectl/cheatsheet/): Log management

**Development**
- [Go 1.20+ Installation](https://golang.org/doc/install)
- [Docker Desktop](https://www.docker.com/products/docker-desktop): Local Kubernetes
- [KinD (Kubernetes in Docker)](https://kind.sigs.k8s.io/): Multi-node testing
- [Kubebuilder](https://book.kubebuilder.io/): Kubernetes controller framework

### Community and Support

- [Kubernetes Storage SIG](https://github.com/kubernetes/community/tree/master/sig-storage)
- [ETCD Community](https://etcd.io/community/)
- [Container Runtime Interface Community](https://github.com/kubernetes/community/blob/master/sig-node/)
- [CNCF Slack Channels](https://cloud-native.slack.com/) (#storage, #etcd, #kubernetes)

### License

Apache License 2.0 - See LICENSE file for details

---

## 19. Appendices

### Glossary of Terms

| Term | Definition |
|---|---|
| **CSI** | Container Storage Interface - standardized interface for storage drivers |
| **VolumeSnapshot** | Kubernetes resource representing point-in-time snapshot of PVC |
| **VolumeSnapshotContent** | Cluster-level resource containing actual snapshot data reference |
| **External Snapshotter** | Kubernetes controller managing VolumeSnapshot lifecycle |
| **etcdutl** | Command-line tool for ETCD backup/restore operations |
| **PVC** | PersistentVolumeClaim - request for storage |
| **PV** | PersistentVolume - actual storage resource |
| **ETCD** | Consistent, distributed key-value store used by Kubernetes |
| **RBAC** | Role-Based Access Control - Kubernetes authorization mechanism |
| **ServiceAccount** | Kubernetes identity for processes in pods |
| **Leader Election** | Mechanism for selecting single active instance in cluster |
| **Quorum** | Majority of cluster members for consensus |
| **Velero** | Open source backup/restore solution for Kubernetes |
| **DataMover** | Velero plugin for snapshot data movement |
| **Snapshot Metadata** | Information about snapshot (ID, size, time, status) |
| **Checksum** | Hash value for integrity verification |

### Sample Configurations

#### Complete Working Example: ETCD Cluster with Snapshots
```yaml
---
# 1. ETCD StatefulSet
apiVersion: v1
kind: ServiceAccount
metadata:
  name: etcd
  namespace: default
---
apiVersion: v1
kind: Service
metadata:
  name: etcd-headless
  namespace: default
spec:
  serviceName: etcd
  clusterIP: None
  selector:
    etcd.io/cluster: my-etcd
  ports:
  - name: client
    port: 2379
    targetPort: 2379
  - name: peer
    port: 2380
    targetPort: 2380
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: etcd
  namespace: default
spec:
  serviceName: etcd-headless
  replicas: 3
  selector:
    matchLabels:
      etcd.io/cluster: my-etcd
  template:
    metadata:
      labels:
        etcd.io/cluster: my-etcd
    spec:
      serviceAccountName: etcd
      containers:
      - name: etcd
        image: quay.io/coreos/etcd:v3.5.0
        command:
        - /usr/local/bin/etcd
        - --name=$(HOSTNAME)
        - --initial-advertise-peer-urls=https://$(HOSTNAME).etcd-headless:2380
        - --listen-peer-urls=https://0.0.0.0:2380
        - --listen-client-urls=https://0.0.0.0:2379
        - --advertise-client-urls=https://$(HOSTNAME).etcd-headless:2379
        - --initial-cluster=etcd-0=https://etcd-0.etcd-headless:2380,etcd-1=https://etcd-1.etcd-headless:2380,etcd-2=https://etcd-2.etcd-headless:2380
        - --initial-cluster-state=new
        - --initial-cluster-token=etcd-cluster
        env:
        - name: HOSTNAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        ports:
        - containerPort: 2379
          name: client
        - containerPort: 2380
          name: peer
        volumeMounts:
        - name: etcd-data
          mountPath: /var/lib/etcd
  volumeClaimTemplates:
  - metadata:
      name: etcd-data
    spec:
      accessModes: [ReadWriteOnce]
      storageClassName: standard
      resources:
        requests:
          storage: 10Gi
---
# 2. Snapshot PVC
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: etcd-snapshots
  namespace: default
  annotations:
    etcd.io/cluster: my-etcd
spec:
  accessModes: [ReadWriteOnce]
  storageClassName: standard
  resources:
    requests:
      storage: 30Gi
---
# 3. CSI Driver and Sidecar Deployment (summarized, see section 8)
apiVersion: apps/v1
kind: Deployment
metadata:
  name: etcd-snapshot-driver
  namespace: kube-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: etcd-snapshot-driver
  template:
    metadata:
      labels:
        app: etcd-snapshot-driver
    spec:
      serviceAccountName: etcd-snapshot-driver
      containers:
      - name: driver
        image: my-registry/etcd-snapshot-driver:v1.0.0
        args:
        - --csi-endpoint=unix:///csi/csi.sock
        volumeMounts:
        - name: csi-socket
          mountPath: /csi
      - name: csi-snapshotter
        image: registry.k8s.io/sig-storage/csi-snapshotter:v6.2.1
        args:
        - --csi-address=/csi/csi.sock
        volumeMounts:
        - name: csi-socket
          mountPath: /csi
      volumes:
      - name: csi-socket
        emptyDir: {}
---
# 4. VolumeGroupSnapshotClass
apiVersion: groupsnapshot.storage.k8s.io/v1beta2
kind: VolumeGroupSnapshotClass
metadata:
  name: etcd-group-snapshot-class
driver: etcd-snapshot-driver
deletionPolicy: Delete
parameters:
  snapshot-timeout: "300"
---
# 5. Create Group Snapshot
apiVersion: groupsnapshot.storage.k8s.io/v1beta2
kind: VolumeGroupSnapshot
metadata:
  name: etcd-backup
  namespace: default
spec:
  volumeGroupSnapshotClassName: etcd-group-snapshot-class
  source:
    selector:
      matchLabels:
        app: etcd
```

### Code Templates

#### Go Driver Structure
```go
package main

import (
  "context"
  "github.com/container-storage-interface/spec/lib/go/csi"
)

// Driver implements CSI driver
type Driver struct {
  k8sClient           kubernetes.Interface
  groupControllerSvc  csi.GroupControllerServer
  logger              logging.Logger
}

// NewDriver creates driver instance
func NewDriver(k8sClient kubernetes.Interface) *Driver {
  return &Driver{
    k8sClient: k8sClient,
  }
}

// CreateVolumeGroupSnapshot implements CSI CreateVolumeGroupSnapshot
func (d *Driver) CreateVolumeGroupSnapshot(ctx context.Context, req *csi.CreateVolumeGroupSnapshotRequest) (*csi.CreateVolumeGroupSnapshotResponse, error) {
  // 1. Validate request
  // 2. For each volume: discover ETCD cluster, create snapshot job
  // 3. Monitor job completion
  // 4. Return group snapshot response
  return nil, nil // placeholder
}

// DeleteVolumeGroupSnapshot implements CSI DeleteVolumeGroupSnapshot
func (d *Driver) DeleteVolumeGroupSnapshot(ctx context.Context, req *csi.DeleteVolumeGroupSnapshotRequest) (*csi.DeleteVolumeGroupSnapshotResponse, error) {
  // Implementation with idempotency
  return &csi.DeleteVolumeGroupSnapshotResponse{}, nil
}

// GetPluginInfo implements CSI GetPluginInfo
func (d *Driver) GetPluginInfo(ctx context.Context, req *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
  return &csi.GetPluginInfoResponse{
    Name:           "etcd-snapshot-driver",
    VendorVersion:  "1.0.0",
  }, nil
}
```

### Troubleshooting Decision Tree

```
Symptom: Snapshot not created
│
├─ Check: CSI driver pod running?
│  ├─ No → kubectl describe deployment/etcd-snapshot-driver
│  │        → Check resource limits, node availability
│  │
│  └─ Yes → Check: external-snapshotter running?
│     ├─ No → Deploy external-snapshotter
│     │
│     └─ Yes → Check: VolumeGroupSnapshotClass exists?
│        ├─ No → Create VolumeGroupSnapshotClass
│        │
│        └─ Yes → Check: CSI socket accessible?
│           ├─ No → Check: volume mount in driver pod
│           │
│           └─ Yes → Check: ETCD cluster discoverable?
│              ├─ No → Verify labels/annotations on ETCD pods
│              │
│              └─ Yes → Check: Snapshot job created?
│                 ├─ No → Check: driver pod logs for errors
│                 │
│                 └─ Yes → Check: Job successful?
│                    ├─ No → Describe job, check pod logs
│                    │        → Likely ETCD connection/auth issue
│                    │
│                    └─ Yes → Snapshot should be ready!
│
Symptom: Snapshot slow/timeout
│
├─ Check: ETCD database size
│  ├─ >10GB → Expected to be slow, increase timeout
│  │
│  └─ <10GB → Check: Network latency to ETCD
│     ├─ >100ms → Network issue
│     │
│     └─ <100ms → Check: ETCD cluster health
│        ├─ Unhealthy → Fix quorum issues
│        │
│        └─ Healthy → Check: Snapshot PVC performance
│           └─ Check: Storage backend performance
```

### Version Compatibility Matrix

| etcd-snapshot-driver | Kubernetes | ETCD | CSI Spec | Velero |
|---|---|---|---|---|
| 1.0.x | 1.20-1.27 | 3.4-3.5 | 1.3.0-1.8.0 | 1.10-1.12 |
| 1.1.x | 1.25-1.29 | 3.5-3.6 | 1.6.0-1.8.0 | 1.11-1.13 |
| 2.0.x | 1.27+ | 3.5+ | 1.8.0+ | 1.12+ |

### Specification Maintenance Process

**Updates and Changes**:
1. Changes documented in CHANGELOG.md
2. Version bumped (semver: major.minor.patch)
3. This specification updated for new versions
4. Breaking changes require major version bump
5. New features documented in appropriate section

**Release Cadence**:
- Patch releases: Monthly (bug fixes)
- Minor releases: Quarterly (features)
- Major releases: Annually (breaking changes)

---

## License

Apache License 2.0

## Summary

This comprehensive specification enables developers to implement the etcd-snapshot-driver from scratch using only this document. It covers all technical aspects from architecture through operations, providing code examples, configuration templates, and detailed workflows for CSI-based ETCD snapshot management in Kubernetes environments.

