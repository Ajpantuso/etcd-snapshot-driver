# Deployment

This directory contains Kustomize-based Kubernetes manifests for the etcd-snapshot-driver.

## Structure

- `base/`: Base Kustomize configuration (environment-agnostic)
- `overlays/dev/`: Development overlay

## Usage

### Deploy to Development

```bash
# Preview what will be deployed
kubectl kustomize deploy/overlays/dev

# Apply to cluster
kubectl apply -k deploy/overlays/dev

# Delete from cluster
kubectl delete -k deploy/overlays/dev
```

### Deploy Base (without overlay)

```bash
kubectl apply -k deploy/base
```

## Customization

### Change Image Tag

Edit `deploy/overlays/dev/kustomization.yaml`:

```yaml
images:
  - name: etcd-snapshot-driver
    newTag: v1.2.3  # Change this
```

### Change Resource Limits

Edit `deploy/overlays/dev/deployment-patch.yaml` to adjust CPU/memory.

### Change Configuration

Edit `deploy/overlays/dev/config-patch.yaml` to modify ConfigMap values.

## Verification Steps

After deployment, you can verify the resources:

```bash
# 1. Validate base kustomization
kubectl kustomize deploy/base

# 2. Validate dev overlay
kubectl kustomize deploy/overlays/dev

# 3. Check for valid YAML output
kubectl kustomize deploy/overlays/dev | kubectl apply --dry-run=client -f -

# 4. Deploy to dev cluster
kubectl apply -k deploy/overlays/dev

# 5. Verify resources created
kubectl get all -n etcd-snapshot-driver-dev

# 6. Check namespace created with suffix
kubectl get namespace etcd-snapshot-driver-dev

# 7. Verify ConfigMap has debug logging
kubectl get configmap etcd-snapshot-driver-config -n etcd-snapshot-driver-dev -o yaml

# 8. Verify labels applied
kubectl get deployment etcd-snapshot-driver -n etcd-snapshot-driver-dev -o yaml | grep -A5 labels

# 9. Clean up
kubectl delete -k deploy/overlays/dev
```

## File Structure

**Base:**
- `base/kustomization.yaml` - Main base configuration
- `base/namespace.yaml` - Namespace definition
- `base/rbac.yaml` - RBAC resources
- `base/csi-driver.yaml` - CSI driver and snapshot class
- `base/deployment.yaml` - Main deployment and ConfigMap

**Dev Overlay:**
- `overlays/dev/kustomization.yaml` - Dev-specific kustomization
- `overlays/dev/deployment-patch.yaml` - Resource limits patch
- `overlays/dev/config-patch.yaml` - ConfigMap debug settings

## Future Enhancements

- Add staging overlay (`deploy/overlays/staging/`)
- Add production overlay (`deploy/overlays/production/`)
- Use ConfigMapGenerator for config files
- Add secretGenerator for sensitive data
- Use components for reusable pieces
- Add Kustomize variables for templating
- Integrate with CI/CD (GitLab CI, GitHub Actions)
- Add Helm chart as alternative deployment method
