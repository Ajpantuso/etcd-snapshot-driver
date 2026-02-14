---
title: HyperShift ETCD Architecture and Management
---

# HyperShift ETCD Architecture and Management

## Overview

HyperShift provides two distinct strategies for managing ETCD clusters: **Managed** (automatic provisioning by HyperShift) and **Unmanaged** (externally managed by the user). The ETCD configuration is defined in the HostedCluster API spec and is **immutable** once set to prevent data corruption and maintain cluster stability.

## ETCD Management Types

### Managed ETCD (Default)

**Strategy:** HyperShift automatically provisions and operates the ETCD cluster.

**Configuration Location:** `spec.etcd.managed`

**Storage:**
- Only PersistentVolume storage type currently supported
- One PersistentVolume per ETCD member (1 or 3 depending on HA mode)
- Default size: 8Gi per PV
- Storage class: Configurable (optional, uses cluster default if not specified)

**Deployment:**
- Runs as a StatefulSet in the hosted control plane namespace
- Services:
  - `etcd-discovery` (headless service for peer discovery on port 2380)
  - `etcd-client` (regular service for client access on port 2379)
- Pod Disruption Budget for ensuring availability during disruptions
- ServiceMonitor for Prometheus monitoring integration
- High priority scheduling via `hypershift.openshift.io/etcd-priority-class`

**Automatic Maintenance:**
- Defragmentation controller runs every 10 minutes to prevent database bloat
- Member health monitoring and recovery
- Automatic monitoring and alerting via ServiceMonitor

**Example Configuration:**
```yaml
spec:
  etcd:
    managementType: Managed
    managed:
      storage:
        type: PersistentVolume
        persistentVolume:
          size: 8Gi
          storageClassName: "fast-ssd"
```

### Unmanaged ETCD

**Strategy:** User provides and manages their own ETCD cluster endpoint.

**Configuration Location:** `spec.etcd.unmanaged`

**Requirements:**
- HTTPS endpoint URL (must match pattern: `^https://`)
- mTLS client authentication with:
  - CA certificate (for server verification)
  - Client certificate
  - Client private key

**TLS Configuration:**
- All credentials stored in a Kubernetes Secret with standard keys:
  - `etcd-client-ca.crt`: Certificate Authority certificate
  - `etcd-client.crt`: Client certificate
  - `etcd-client.key`: Client private key

**Use Cases:**
- Multiple HostedClusters sharing a single ETCD instance
- ETCD infrastructure in different location/provider
- Custom ETCD configuration requirements

**Example Configuration:**
```yaml
spec:
  etcd:
    managementType: Unmanaged
    unmanaged:
      endpoint: "https://etcd-client:2379"
      tls:
        clientSecret:
          name: "my-etcd-credentials"
```

## Storage Configuration

### PersistentVolume Storage Spec

```yaml
spec:
  etcd:
    managed:
      storage:
        type: PersistentVolume
        persistentVolume:
          storageClassName: "standard"      # Optional, uses cluster default if omitted
          size: "8Gi"                       # Default size, immutable after creation
        restoreSnapshotURL:                 # Optional
        - "https://s3.amazonaws.com/bucket/snapshot.db?signed-params"
```

### Important: Storage Immutability

**The following are IMMUTABLE after cluster creation:**
- Storage class
- Storage size
- Storage type
- Management type (cannot switch Managed ↔ Unmanaged)

**Why Immutable:**
1. **Data Consistency:** Changing storage parameters mid-cluster could corrupt ETCD data
2. **Quorum Stability:** Fixed replica count ensures predictable availability
3. **Performance:** Prevents unexpected storage performance changes

### Snapshot Restoration

**When Snapshots are Restored:**
- Only during **cluster creation** (immutable operation)
- Automatically when PersistentVolume is empty
- Useful for disaster recovery and cluster migration

**How to Use:**
1. Create a pre-signed S3 URL: `aws s3 presign s3://bucket/snapshot.db`
2. Add `restoreSnapshotURL` to initial HostedCluster spec
3. Ensure encryption secret matches the original cluster's key

**Snapshot Limitations:**
- Cannot restore to existing cluster (immutable at creation only)
- Requires matching encryption key for data decryption
- Must provide pre-signed URL (no direct credentials)

## ETCD Utilities and Maintenance

### ETCD Backup

**Location in Repository:** `etcd-backup/` subdirectory

**Purpose:** Automated snapshots with S3 upload capability for disaster recovery.

**Execution Model:**
- Runs as a CronJob in the hosted control plane namespace
- Service Account: `etcd-backup-sa`
- Uses `etcdctl snapshot save` for consistent snapshots

**Workflow:**
1. Creates local snapshot file: `/tmp/snapshot.db`
2. Uploads to configured S3 bucket with timestamp-based key
3. Optional: Applies metadata tags to S3 object

**Configuration:**
- ETCD endpoint (with mTLS certificates)
- S3 bucket name and region
- S3 key prefix for organization
- Optional object tags for filtering/cost allocation
- Client timeout: 5 minutes (default)

**Example Command:**
```bash
etcd-backup \
  --etcd-endpoint https://etcd-client:2379 \
  --etcd-client-cert /etc/etcd/tls/client/etcd-client.crt \
  --etcd-client-key /etc/etcd/tls/client/etcd-client.key \
  --etcd-ca-cert /etc/etcd/tls/etcd-ca/ca.crt \
  --s3-bucket-name my-backups \
  --s3-key-prefix etcd/production \
  --s3-object-tags cluster=prod,env=production
```

### ETCD Defragmentation Controller

**Location in Repository:** `etcd-defrag/` subdirectory

**Purpose:** Prevent database bloat from accumulated deleted keys.

**Background:**
ETCD stores data in a B+tree. When keys are deleted, space isn't immediately reclaimed. Over time, this can bloat the database file, reducing performance and wasting storage.

**How It Works:**
- Runs continuously every 10 minutes
- Checks cluster health before attempting defragmentation
- Monitors each member's fragmentation percentage
- Triggers defrag when thresholds are exceeded

**Defragmentation Thresholds:**
- Minimum bytes to defrag: 100MB
- Maximum allowed fragmentation: 45%
- Minimum wait between defrag cycles: 36 seconds
- Maximum consecutive failures before degrading: 3

**Member Monitoring:**
- `DbSize`: Total database file size on disk
- `DbSizeInUse`: Actual data size (space actually needed)
- Fragmentation = (DbSize - DbSizeInUse) / DbSize × 100%

**Example Metrics:**
```
Member: etcd-0
  DbSize: 1.5GB
  DbSizeInUse: 1.0GB
  Fragmentation: 33%  → Triggers defrag
```

### ETCD Recovery Tool

**Location in Repository:** `etcd-recovery/` subdirectory

**Purpose:** Diagnostic and recovery operations for unhealthy ETCD clusters.

**Commands:**

#### Status Reporting
```bash
recover-etcd status \
  --namespace openshift-cnv \
  --etcd-client-cert /etc/etcd/tls/client/etcd-client.crt \
  --etcd-client-key /etc/etcd/tls/client/etcd-client.key \
  --etcd-ca-cert /etc/etcd/tls/etcd-ca/ca.crt
```

Reports:
- Member health status (healthy/unhealthy)
- Current cluster leader
- Database size and fragmentation metrics
- Detailed error messages for problem diagnosis

#### Cluster Recovery
```bash
recover-etcd run \
  --namespace openshift-cnv \
  --etcd-client-cert /etc/etcd/tls/client/etcd-client.crt \
  --etcd-client-key /etc/etcd/tls/client/etcd-client.key \
  --etcd-ca-cert /etc/etcd/tls/etcd-ca/ca.crt
```

Capabilities:
- Recovers clusters that still have quorum
- Handles non-functional cluster members
- Waits up to 5 minutes for cluster to become healthy
- Useful for member failure recovery without full restore

**When to Use Recovery:**
- A single member becomes unhealthy
- Leader election fails but majority is healthy
- Temporary network partitions caused member lag
- Post-maintenance cluster verification

## PKI and TLS Management

### Certificate Architecture

HyperShift automatically manages ETCD certificates for secure communication:

**Certificate Chain:**
1. **ETCD Signer (CA):** Issues all other ETCD certificates
2. **Server Certificate:** Authenticates ETCD server to clients
3. **Peer Certificate:** Authenticates members in cluster
4. **Client Certificate:** Authenticates API servers connecting to ETCD

**Automatically Reconciled Secrets:**

| Secret | Purpose | Used By |
|--------|---------|---------|
| `etcd-signer-secret` | CA for signing ETCD certs | Control plane operator |
| `etcd-signer-configmap` | Distributed CA public key | All control plane components |
| `etcd-server-secret` | Server identity certificate | ETCD StatefulSet |
| `etcd-peer-secret` | Peer authentication | ETCD peer communication |
| `etcd-client-secret` | Client authentication | API servers (kube-apiserver, etc.) |

### Priority Classes

ETCD pods are assigned high priority to ensure availability:
- **Priority Class:** `hypershift.openshift.io/etcd-priority-class`
- **Purpose:** Prevents eviction during resource pressure
- **Similar Classes:**
  - `hypershift.openshift.io/api-critical-priority-class` (API servers)
  - `hypershift.openshift.io/control-plane-priority-class` (Other control plane pods)

## Backup and Restore Workflow

### Manual Snapshot Process

**Scenario:** You need to backup ETCD state for disaster recovery.

**Steps:**

1. **Pause Reconciliation:**
   ```bash
   CLUSTER_NAME=my-cluster
   PAUSED_UNTIL=$(date -u -d "+1 hour" +%Y-%m-%dT%H:%M:%SZ)

   oc patch -n clusters hostedclusters/${CLUSTER_NAME} \
     -p "{\"spec\":{\"pausedUntil\":\"${PAUSED_UNTIL}\"}}" \
     --type=merge
   ```
   This prevents the control plane operator from modifying ETCD during the snapshot.

2. **Scale Down API Writers:**
   ```bash
   HOSTED_CLUSTER_NAMESPACE=clusters-${CLUSTER_NAME}

   oc scale deployment -n ${HOSTED_CLUSTER_NAMESPACE} \
     --replicas=0 \
     kube-apiserver \
     openshift-apiserver \
     openshift-oauth-apiserver
   ```
   This ensures no writes during snapshot (ETCD freezes during snapshot).

3. **Create Snapshot:**
   ```bash
   oc exec -it etcd-0 -n ${HOSTED_CLUSTER_NAMESPACE} -- \
     env ETCDCTL_API=3 /usr/bin/etcdctl \
     --cacert /etc/etcd/tls/etcd-ca/ca.crt \
     --cert /etc/etcd/tls/client/etcd-client.crt \
     --key /etc/etcd/tls/client/etcd-client.key \
     --endpoints=localhost:2379 \
     snapshot save /var/lib/data/snapshot.db
   ```

4. **Verify Snapshot:**
   ```bash
   oc exec -it etcd-0 -n ${HOSTED_CLUSTER_NAMESPACE} -- \
     env ETCDCTL_API=3 /usr/bin/etcdctl \
     snapshot status /var/lib/data/snapshot.db
   ```

5. **Upload to S3:**
   ```bash
   BUCKET_NAME=etcd-backups
   CLUSTER_NAME=my-cluster

   # Using presigned URL (preferred)
   PRESIGNED_URL=$(aws s3 presign "s3://${BUCKET_NAME}/${CLUSTER_NAME}/snapshot.db")

   oc exec -it etcd-0 -n ${HOSTED_CLUSTER_NAMESPACE} -- \
     curl -X PUT -T /var/lib/data/snapshot.db "${PRESIGNED_URL}"
   ```

6. **Save Encryption Key:**
   ```bash
   # For later restoration
   oc get secret ${CLUSTER_NAME}-etcd-encryption-key \
     -o jsonpath='{.data.key}' | base64 -d > encryption-key.txt
   ```

### Snapshot Restoration Process

**Scenario:** Create new HostedCluster from previous snapshot.

**Steps:**

1. **Generate Pre-Signed URL:**
   ```bash
   BUCKET_NAME=etcd-backups
   CLUSTER_NAME=my-cluster

   ETCD_SNAPSHOT="s3://${BUCKET_NAME}/${CLUSTER_NAME}/snapshot.db"
   ETCD_SNAPSHOT_URL=$(aws s3 presign ${ETCD_SNAPSHOT})
   ```

2. **Create HostedCluster with Snapshot:**
   ```yaml
   apiVersion: hypershift.openshift.io/v1beta1
   kind: HostedCluster
   metadata:
     name: restored-cluster
     namespace: clusters
   spec:
     etcd:
       managementType: Managed
       managed:
         storage:
           type: PersistentVolume
           persistentVolume:
             size: 4Gi
           restoreSnapshotURL:
           - "${ETCD_SNAPSHOT_URL}"
     secretEncryption:
       aescbc:
         activeKey:
           name: "restored-cluster-etcd-encryption-key"
     # ... rest of HostedCluster spec
   ```

3. **Ensure Matching Encryption Key:**
   ```bash
   # Create secret with the key saved during backup
   oc create secret generic restored-cluster-etcd-encryption-key \
     --from-file=key=encryption-key.txt \
     -n clusters
   ```

4. **Monitor Restoration:**
   ```bash
   # Watch ETCD StatefulSet initialization
   oc logs -n clusters-restored-cluster -f etcd-0

   # Check ETCD cluster status once running
   oc exec -it etcd-0 -n clusters-restored-cluster -- \
     env ETCDCTL_API=3 /usr/bin/etcdctl \
     --cacert /etc/etcd/tls/etcd-ca/ca.crt \
     --cert /etc/etcd/tls/client/etcd-client.crt \
     --key /etc/etcd/tls/client/etcd-client.key \
     --endpoints=localhost:2379 \
     member list
   ```

**Important Notes:**
- Snapshot restoration **only happens at cluster creation**
- Snapshot is only restored if PersistentVolume is empty
- Must use **exact same encryption key** as original backup
- Pre-signed URL prevents exposing AWS credentials in manifest

## Configuration Validation

### Immutability Rules

HyperShift enforces these immutability validations:

```yaml
# Rule 1: managementType is immutable
- self == oldSelf

# Rule 2: Managed type requires managed config
- self.managementType == 'Managed' ? has(self.managed) : !has(self.managed)

# Rule 3: Unmanaged type requires unmanaged config
- self.managementType == 'Unmanaged' ? has(self.unmanaged) : !has(self.unmanaged)

# Rule 4: Storage size is immutable
- self == oldSelf

# Rule 5: Storage class is immutable
- self == oldSelf
```

### Type Validation

| Field | Validation | Reason |
|-------|-----------|--------|
| `managementType` | Must be "Managed" or "Unmanaged" | Contract definition |
| `managed` | Only set if managementType="Managed" | Prevents config conflicts |
| `unmanaged` | Only set if managementType="Unmanaged" | Prevents config conflicts |
| `unmanaged.endpoint` | Must match `^https://` | Only HTTPS endpoints |
| `restoreSnapshotURL` | Max 1 entry, max 1024 chars | Single restore per cluster |

## Deployment Patterns

### Single-Node (Non-HA) Control Plane

**Use Case:** Development, testing, or resource-constrained environments

**Configuration:**
```yaml
spec:
  controlPlaneAvailability: SingleReplica
  etcd:
    managementType: Managed
    managed:
      storage:
        type: PersistentVolume
        persistentVolume:
          size: 8Gi
```

**Characteristics:**
- 1 ETCD replica
- 1 PersistentVolume (8Gi default)
- Single control plane pod for each component
- Acceptable downtime for maintenance

### High-Availability (HA) Control Plane

**Use Case:** Production clusters requiring high availability

**Configuration:**
```yaml
spec:
  controlPlaneAvailability: HighlyAvailable
  etcd:
    managementType: Managed
    managed:
      storage:
        type: PersistentVolume
        persistentVolume:
          size: 20Gi
```

**Characteristics:**
- 3 ETCD replicas for quorum
- 3 PersistentVolumes with diversity across nodes
- Pod Disruption Budget prevents quorum loss
- Self-healing via defragmentation
- Automatic recovery from member failure
- Pod Affinity: Spread across nodes

### External/Shared ETCD

**Use Case:** Multiple HostedClusters sharing single ETCD instance

**Configuration:**
```yaml
spec:
  etcd:
    managementType: Unmanaged
    unmanaged:
      endpoint: "https://shared-etcd-cluster:2379"
      tls:
        clientSecret:
          name: "shared-etcd-creds"
```

**Advantages:**
- Cost efficiency (single ETCD scales to multiple clusters)
- Simplified operations (one ETCD to manage)
- Larger backup windows

**Disadvantages:**
- Tight coupling (shared ETCD failure affects all clusters)
- Shared quota (noisy neighbor issues)
- Complex TLS/network setup
- Single failure domain

## Operational Considerations

### Monitoring and Alerting

**Key Metrics to Monitor:**
- `etcd_server_has_leader` (cluster stability)
- `etcd_disk_backend_commit_duration_seconds` (write latency)
- `etcd_server_proposals_failed_total` (write failures)
- `etcd_debugging_snap_save_total_duration_seconds` (snapshot time)
- `process_resident_memory_bytes` (memory usage)

**Common Alerts:**
- Unhealthy cluster members
- High database fragmentation (>45%)
- Slow commit operations
- Member reboot after reconciliation change

### Capacity Planning

**Storage Size Calculation:**
```
Baseline: 8Gi (default minimum)
Per 1000 Objects: ~50MB (varies with average object size)
Buffer for Deletes: 2x (before defragmentation)

Example: 100,000 objects → ~5GB + 16GB buffer = 20Gi recommended
```

**Disk I/O Considerations:**
- Use fast storage (SSD/NVMe)
- Avoid noisy neighbor workloads on same disk
- Monitor commit latencies (target: <25ms)

### Disaster Recovery Strategy

1. **Automated Backups:** CronJob every 6-24 hours
2. **Point-in-Time Recovery:** Keep recent snapshots (7-30 days)
3. **Encryption Key Backup:** Store separately from snapshots
4. **Regular Tests:** Practice restore procedures quarterly
5. **Cross-Region:** Store backups in separate region/provider

## HyperShift Repository Reference

**Key Source Locations:**

| Component | Path | Purpose |
|-----------|------|---------|
| API Types | `api/hypershift/v1beta1/hostedcluster_types.go:1579-1724` | ETCD spec definitions |
| Manifests | `control-plane-operator/controllers/hostedcontrolplane/manifests/etcd.go` | Kubernetes resource templates |
| PKI | `control-plane-operator/controllers/hostedcontrolplane/pki/etcd.go` | Certificate generation |
| Backup | `etcd-backup/etcdbackup.go` | Snapshot and S3 upload |
| Defrag | `etcd-defrag/etcddefrag_controller.go` | Fragmentation controller |
| Recovery | `etcd-recovery/etcdrecovery.go` | Diagnostic and recovery CLI |
| Backup Docs | `docs/content/how-to/aws/etc-backup-restore.md` | Backup/restore guide |

## Summary

HyperShift's ETCD management provides:
- **Managed ETCD:** Fully automated, opinionated deployment suitable for most use cases
- **Unmanaged ETCD:** Flexible external integration for specialized deployments
- **Immutable Configuration:** Strong safety guarantees preventing accidental corruption
- **Automated Maintenance:** Defragmentation, monitoring, and recovery tools
- **Disaster Recovery:** Snapshot/restore capabilities with encryption support
- **High Availability:** Multi-member deployments with quorum protection

The architecture balances ease of use (Managed) with flexibility (Unmanaged) while enforcing operational safety through immutability constraints.
