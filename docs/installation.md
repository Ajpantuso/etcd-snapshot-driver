# Installation Guide

## Prerequisites

- Kubernetes 1.24+ cluster
- `kubectl` command-line tool configured to access the cluster
- Sufficient RBAC permissions to create namespaces and RBAC resources
- VolumeSnapshot support enabled (snapshot.storage.k8s.io API group)

## Installation Steps

### 1. Create Namespace

```bash
kubectl create namespace etcd-snapshot-driver
```

### 2. Apply RBAC Resources

```bash
kubectl apply -f deploy/kubernetes/rbac.yaml
```

### 3. Apply CSI Driver

```bash
kubectl apply -f deploy/kubernetes/csi-driver.yaml
```

### 4. Deploy the Driver

```bash
# Build and push the image
docker build -t myregistry/etcd-snapshot-driver:latest -f build/Dockerfile .
docker push myregistry/etcd-snapshot-driver:latest

# Update the image in deployment.yaml
sed -i 's|etcd-snapshot-driver:latest|myregistry/etcd-snapshot-driver:latest|g' deploy/kubernetes/deployment.yaml

# Deploy
kubectl apply -f deploy/kubernetes/deployment.yaml
```

### 5. Verify Installation

```bash
# Check deployment
kubectl rollout status deployment/etcd-snapshot-driver -n etcd-snapshot-driver

# Check logs
kubectl logs -f deployment/etcd-snapshot-driver -n etcd-snapshot-driver

# Check health endpoints
kubectl port-forward -n etcd-snapshot-driver svc/etcd-snapshot-driver 8080:8080
curl http://localhost:8080/healthz
curl http://localhost:8080/ready
curl http://localhost:8080/metrics
```

## Configuration

### Environment Variables

- `CSI_ENDPOINT`: CSI socket endpoint (default: `unix:///var/lib/kubelet/plugins/etcd-snapshot-driver/csi.sock`)
- `LOG_LEVEL`: Logging level (default: `info`)
- `SNAPSHOT_TIMEOUT`: Snapshot operation timeout in seconds (default: `300`)
- `JOB_BACKOFF_LIMIT`: Kubernetes Job backoff limit (default: `3`)

### Snapshot Timeout

Configure snapshot timeout via ConfigMap:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: etcd-snapshot-driver-config
  namespace: etcd-snapshot-driver
data:
  config.yaml: |
    snapshot_timeout: 300
    storage_class: standard
```
