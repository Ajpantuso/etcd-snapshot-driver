# Troubleshooting Guide

## Common Issues and Solutions

### 1. Snapshot Fails with "ETCD discovery failed"

**Symptoms**: CreateVolumeGroupSnapshot RPC returns discovery error

**Solutions**:
1. Verify PVC has proper labels:
   ```bash
   kubectl get pvc -L etcd.io/cluster
   ```

2. Verify ETCD pods exist with matching labels:
   ```bash
   kubectl get pods -l etcd.io/cluster=<cluster-name>
   ```

3. Check driver logs:
   ```bash
   kubectl logs deployment/etcd-snapshot-driver -n etcd-snapshot-driver
   ```

### 2. Snapshot Fails with "credential resolution failed"

**Symptoms**: Snapshot succeeds at discovery but fails at credential resolution

**Solutions**:
1. Verify credential secret exists:
   ```bash
   kubectl get secret -n default | grep etcd
   ```

2. Verify secret has correct keys:
   ```bash
   kubectl get secret etcd-client-certs -o yaml
   # Should contain: tls.crt, tls.key, ca.crt
   ```

### 3. Snapshot Job Fails to Execute

**Symptoms**: Job created but never completes successfully

**Solutions**:
1. Check job status:
   ```bash
   kubectl describe job etcd-snapshot-save-<snapshot-id>
   kubectl logs job/etcd-snapshot-save-<snapshot-id>
   ```

2. Verify ETCD endpoints are reachable:
   ```bash
   kubectl run -it --image=nicolaka/netcat etcd-test -- \
     nc -zv https://etcd-0.etcd.svc.cluster.local 2379
   ```

3. Increase job timeout if necessary:
   ```bash
   kubectl patch configmap etcd-snapshot-driver-config -n etcd-snapshot-driver \
     --type merge -p '{"data":{"config.yaml":"snapshot_timeout: 600\n..."}}'
   ```

### 4. Snapshot Validation Fails

**Symptoms**: Job succeeds but snapshot validation fails

**Solutions**:
1. Verify snapshot file was created:
   ```bash
   kubectl exec -it pod/<snapshot-pvc-pod> -- ls -lh /snapshots/
   ```

2. Manually validate snapshot:
   ```bash
   kubectl run -it --image=quay.io/coreos/etcd:v3.5.0 etcd-validator -- \
     etcdutl snapshot status /snapshots/<snapshot-id>.db
   ```

### 5. Driver Won't Start

**Symptoms**: Pod CrashLoopBackOff or not ready

**Solutions**:
1. Check pod logs:
   ```bash
   kubectl logs pod/etcd-snapshot-driver-xxx -n etcd-snapshot-driver
   ```

2. Verify Kubernetes API is accessible:
   ```bash
   kubectl auth can-i list pvc --as=system:serviceaccount:etcd-snapshot-driver:etcd-snapshot-driver
   ```

3. Check socket permissions:
   ```bash
   kubectl exec -it pod/etcd-snapshot-driver-xxx -n etcd-snapshot-driver -- \
     ls -la /var/lib/kubelet/plugins/etcd-snapshot-driver/
   ```

## Debugging Tips

### Enable Debug Logging

```bash
kubectl set env deployment/etcd-snapshot-driver -n etcd-snapshot-driver \
  LOG_LEVEL=debug
```

### Check Metrics

```bash
kubectl port-forward svc/etcd-snapshot-driver 8080:8080 -n etcd-snapshot-driver
curl http://localhost:8080/metrics | grep etcd_snapshot
```

### Test ETCD Connectivity

```bash
kubectl run -it --image=quay.io/coreos/etcd:v3.5.0 etcd-tester -- bash
# Inside pod:
etcdctl --endpoints=https://etcd-0.etcd.svc.cluster.local:2379 \
  --cacert=/var/run/secrets/etcd/client/ca.crt \
  --cert=/var/run/secrets/etcd/client/tls.crt \
  --key=/var/run/secrets/etcd/client/tls.key \
  member list
```

## Performance Tuning

### Increase Snapshot Timeout for Large Clusters

```bash
# In deployment.yaml env section
- name: SNAPSHOT_TIMEOUT
  value: "600"  # 10 minutes
```

### Allocate More Resources

```bash
kubectl set resources deployment/etcd-snapshot-driver -n etcd-snapshot-driver \
  --requests=cpu=200m,memory=512Mi \
  --limits=cpu=1000m,memory=1Gi
```

### Monitor Resource Usage

```bash
kubectl top pod -n etcd-snapshot-driver
```

## Getting Help

1. Check logs: `kubectl logs -f deployment/etcd-snapshot-driver -n etcd-snapshot-driver`
2. Describe resources: `kubectl describe snapshot <name>`
3. Check events: `kubectl get events -n etcd-snapshot-driver`
4. File issue on GitHub with logs and configuration
