# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build Commands

```bash
make build                # Build binary to bin/etcd-snapshot-driver (Linux x86_64, CGO disabled)
make test                 # Run unit tests with race detection
make test-coverage        # Run tests and generate coverage.html
make integration-test     # Run integration tests (requires Kind cluster)
make lint                 # Run golangci-lint
make fmt                  # Format code and organize imports
make docker-build         # Build Docker image (etcd-snapshot-driver:latest)
make all                  # Full pipeline: clean, lint, test, build
```

Run a single test:
```bash
go test -v -race ./internal/driver/ -run TestFunctionName
```

## Architecture

This is a Kubernetes CSI (Container Storage Interface) driver that manages point-in-time snapshots of ETCD clusters. It implements the CSI GroupController service to create, query, and delete consistent snapshots across multiple ETCD volumes.

### Core Flow

The driver runs as a single-replica Kubernetes Deployment exposing:
- **gRPC server** on a Unix socket — implements CSI Identity and GroupController services
- **HTTP server** on `:8080` — Prometheus metrics (`/metrics`), liveness (`/healthz`), readiness (`/ready`)

**Snapshot creation workflow** (`CreateVolumeGroupSnapshot`):
1. Parse volume IDs → extract namespace/PVC pairs
2. Discover ETCD cluster via label selectors on PVCs (configurable label key, default `etcd.io/cluster`)
3. Validate cluster health (quorum, leader, member health) via TLS-authenticated ETCD client
4. Provision a dedicated snapshot PVC in the target namespace
5. Create and monitor a Kubernetes Job that runs `etcdctl snapshot save`
6. Store metadata in ConfigMaps in `kube-system` (`etcd-snapshot-metadata`, `etcd-group-snapshot-metadata`)

### Package Layout

- **`cmd/etcd-snapshot-driver/`** — CLI entry point using Cobra/Viper. Wires together all components.
- **`internal/driver/`** — CSI gRPC service implementations:
  - `driver.go` — gRPC server lifecycle
  - `identity.go` — CSI Identity service (plugin info, capabilities, probe)
  - `groupcontroller.go` — CSI GroupController service (create/delete/get group snapshots) — the main business logic
  - `snapshotpvc.go` — Provisions PVCs for storing snapshot data
  - `controller.go` — Helper functions for volume ID parsing and PVC validation
  - `options.go` — Functional options pattern for configuring driver components
- **`internal/etcd/`** — ETCD cluster discovery via Kubernetes label selectors and health validation (quorum, leader election, per-member checks)
- **`internal/job/`** — Kubernetes Job creation, monitoring, and templating for etcdctl operations (save/delete/restore)
- **`internal/snapshot/`** — Metadata persistence using Kubernetes ConfigMaps as the backing store
- **`internal/health/`** — Health checker for liveness/readiness probes
- **`internal/metrics/`** — Prometheus metric definitions (snapshot ops, CSI RPCs, storage, ETCD health)
- **`internal/util/`** — Custom error types (`SnapshotNotFoundError`, `ETCDDiscoveryError`, `JobExecutionError`)

### Key Design Patterns

- **Functional options** (`DriverOption` interface) for configuring `Driver`, `GroupControllerServer`, and `IdentityServer`
- **Idempotent deletes** — `DeleteVolumeGroupSnapshot` always returns success; missing resources are treated as already deleted
- **ConfigMap-backed metadata** — No external database; snapshot metadata is JSON-serialized into ConfigMaps

### Key Dependencies

- `github.com/container-storage-interface/spec` v1.12.0 — CSI protobuf definitions
- `go.etcd.io/etcd/client/v3` — ETCD client for health checks
- `k8s.io/client-go` — Kubernetes API interactions (Jobs, PVCs, ConfigMaps)
- `google.golang.org/grpc` — gRPC server
- `go.uber.org/zap` — Structured logging

### Deployment

Kustomize-based deployment in `deploy/`. Base resources include namespace, RBAC, Deployment, CSIDriver registration, and VolumeGroupSnapshotClass. Dev overlay available in `deploy/overlays/dev/`.
