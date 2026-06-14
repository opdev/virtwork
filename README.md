# virtwork

virtwork is a CLI tool that creates virtual machines on OpenShift clusters with [OpenShift Virtualization](https://docs.openshift.com/container-platform/latest/virt/about_virt/about-virt.html) (CNV) installed and runs continuous workloads inside them. It produces realistic CPU, memory, database, network, and disk I/O metrics for monitoring systems like Prometheus and Grafana.

virtwork is a **one-shot deployment tool** — it creates resources and exits. Workload lifecycle management is handled by systemd inside each VM.

## Motivation

The virtwork project was born from a practical necessity; the need to rapidly deploy and manage synthetic test workloads within OpenShift Virtualization environments. As organizations increasingly migrate virtualized infrastructure into OpenShift, the need for a standardized, repeatable way to stress-test these environments becomes critical. virtwork bridges this gap by providing a streamlined framework to validate how your cluster handles real-world pressure.

Whether you are fine-tuning high-speed network throughput, conducting rigorous CPU, memory and disk benchmarking, or verifying the accuracy of your monitoring and observability stack, virtwork automates the heavy lifting. By spinning up diverse, customizable workloads, it ensures that your OpenShift clusters are not just functional, but optimized for peak performance and production-ready reliability.

## virtwork vs kube-burner
virtwork and kube-burner solve adjacent but non-overlapping problems. kube-burner measures how the platform handles VMs — boot latency, clone throughput, migration speed. virtwork generates what happens inside them — real I/O, real traffic, real database load — as persistent systemd services for partner products to observe and validate. See the [full comparison](docs/virtwork-vs-kube-burner.md) for more details..

## Prerequisites

- **Go**: 1.26+ (for building from source)
- **OpenShift**: 4.12+ with OpenShift Virtualization (CNV) installed
  - KubeVirt API v1.7.0+ compatible
  - CDI (Containerized Data Importer) v1.64.0+ compatible
- **Cluster access**: `kubeconfig` with permissions to create/delete:
  - Namespaces
  - VirtualMachines, VirtualMachineInstances, DataVolumes
  - Services, Secrets
- **Storage**: Default StorageClass for persistent volumes

**For testing only:**
- Integration tests: Cluster with above requirements + namespace admin permissions
- E2E tests: All integration requirements + ability to build Go binaries

See [Development Guide](docs/development.md#cluster-prerequisites-for-integration-and-e2e-tests) for detailed cluster setup.

## Installation

```bash
go build -o virtwork ./cmd/virtwork
```

Or run directly:

```bash
go run ./cmd/virtwork --help
```

## Golden Container Disk Image (Optional)

For faster VM boot times, virtwork provides a golden container disk image with all workload tools pre-installed:

- **stress-ng** (CPU/memory workloads)
- **fio** (disk I/O workloads)
- **iperf3** (network workloads)
- **postgresql-server** (database workloads)
- **tc, iptables** (chaos engineering tools)

Using this image reduces VM boot time by 2-5 minutes by eliminating package installation.

### Using the Golden Image

```bash
# Use via CLI flag
virtwork run --container-disk-image quay.io/opdev/virtwork-disk:latest

# Or via environment variable
export VIRTWORK_CONTAINER_DISK_IMAGE=quay.io/opdev/virtwork-disk:latest
virtwork run

# Or via config file
echo "container_disk_image: quay.io/opdev/virtwork-disk:latest" > config.yaml
virtwork run --config config.yaml
```

### Building the Golden Image

See [build/golden-image/README.md](build/golden-image/README.md) for build instructions.

**Note**: The golden image is optional. If you use the default Fedora image, packages will be installed at VM boot time via cloud-init as usual.

## Quick Start

```bash
# Preview what would be deployed (no cluster required)
virtwork run --dry-run

# Deploy all workloads with defaults
virtwork run

# Deploy specific workloads
virtwork run --workloads cpu,memory,disk

# Deploy with SSH access for debugging
virtwork run --ssh-user virtwork --ssh-key-file ~/.ssh/id_ed25519.pub

# Clean up all managed resources
virtwork cleanup

# Clean up resources from a specific run
virtwork cleanup --run-id <uuid>
```

## Workloads

virtwork ships nine built-in workloads, grouped by purpose. With `--vm-count 1` and no `--workloads` filter, a full run creates **11 VMs**: seven single-VM workloads plus two server/client pairs (network and tps).

**Core load generators** — saturate one resource per VM:

| Workload | VMs | Description | Tools |
|----------|-----|-------------|-------|
| **cpu** | N | Continuous CPU stress | `stress-ng --cpu 0 --cpu-load 100 --cpu-method all` (configurable via `params`) |
| **memory** | N | Memory pressure | `stress-ng --vm 1 --vm-bytes 80% --vm-method all` (configurable via `params`) |
| **disk** | N | Mixed random and sequential I/O on a data disk | `fio` with multiple profiles (configurable via `params`) |
| **database** | N | PostgreSQL with pgbench loop on a data disk | `pgbench -c 10 -j 2 -T 300` (configurable via `params`) |

**Multi-VM benchmarks** — server/client pairs coordinated via a ClusterIP Service:

| Workload | VMs | Description | Tools |
|----------|-----|-------------|-------|
| **network** | N × 2 | Bidirectional throughput | `iperf3 -P 4 -t 60 --bidir` on port 5201 (configurable via `params`) |
| **tps** | N × 2 | TCP request/response + HTTP file transfer | `netperf` (12865/12866) + Python HTTP server (8080) |

**Chaos engineering** — inject failures inside the VM to test resilience. ⚠️ See [docs/chaos-workloads.md](docs/chaos-workloads.md) before deploying:

| Workload | VMs | Description | Tools |
|----------|-----|-------------|-------|
| **chaos-disk** | N | Fill the data disk to a target percentage, release, repeat | `fallocate`, `dd` |
| **chaos-network** | N | Inject latency and packet loss on egress | `tc` + `netem` |
| **chaos-process** | N | Randomly send signals to non-essential processes | shell + `ps`/`kill` |

All workloads run as systemd services inside the VMs, surviving reboots and auto-restarting on failure.

**Custom workloads via catalog** — operators can add workloads without writing Go code by placing systemd service files and an optional YAML manifest in a catalog directory. Deploy with `--from-catalog`. See the [catalog tutorial](docs/guide/04-adding-a-catalog-workload.md) for a hands-on walkthrough.

## Usage

### `virtwork run`

Deploy VMs with workloads.

```
Flags:
      --workloads strings          Workloads to deploy (default: all nine — chaos-disk, chaos-network, chaos-process, cpu, database, disk, memory, network, tps)
      --vm-count int               Number of VMs per workload (default 1)
      --cpu-cores int              CPU cores per VM
      --memory string              Memory per VM (e.g., 2Gi)
      --disk-size string           Data disk size
      --container-disk-image string Container disk image for VMs
      --dry-run                    Print specs without creating resources
      --no-wait                    Skip waiting for VM readiness
      --timeout int                Readiness timeout in seconds
      --ssh-user string            SSH user for VMs
      --ssh-password string        SSH password for VMs
      --ssh-key strings            SSH authorized key (repeatable)
      --ssh-key-file strings       SSH key file path (repeatable)
      --vm-concurrency int         Max concurrent VM creation operations
      --params string              Per-workload params (comma-separated workload.key=value pairs)
      --from-catalog strings      Catalog entries to load (comma-separated)
      --catalog-dir string        Path to catalog directory (default ~/.virtwork/catalog)

Global Flags:
      --namespace string           Kubernetes namespace for VMs
      --kubeconfig string          Path to kubeconfig file
      --config string              Path to YAML config file
      --verbose                    Enable verbose output
      --audit                      Enable audit tracking (default true)
      --no-audit                   Disable audit tracking
      --audit-db string            Path to SQLite audit database (default "virtwork.db")
```

### `virtwork cleanup`

Delete all resources managed by virtwork.

```
Flags:
      --delete-namespace           Also delete the namespace
      --run-id string              Target a specific run for cleanup
```

Cleanup is error-tolerant — individual resource deletion failures are logged but do not abort the operation. All resources are tracked via the `app.kubernetes.io/managed-by: virtwork` label and `virtwork/run-id` labels, so cleanup works even if the tool crashed mid-deployment.

## Configuration

virtwork uses a priority chain for configuration (highest to lowest):

1. CLI flags
2. Environment variables (`VIRTWORK_` prefix)
3. YAML config file (`--config`)
4. Defaults

The tables below cover the common surface. For a complete reference of every flag, environment variable, YAML key, and per-workload parameter, see [docs/configuration.md](docs/configuration.md).

### Environment Variables

| Variable | Description |
|----------|-------------|
| `VIRTWORK_NAMESPACE` | Kubernetes namespace |
| `VIRTWORK_SSH_USER` | SSH user for VMs |
| `VIRTWORK_SSH_PASSWORD` | SSH password for VMs |
| `VIRTWORK_SSH_AUTHORIZED_KEYS` | Comma-separated SSH public keys |
| `VIRTWORK_PARAMS` | Per-workload params (comma-separated `workload.key=value`) |
| `VIRTWORK_AUDIT` | Enable audit tracking (true/false) |
| `VIRTWORK_AUDIT_DB` | Path to SQLite audit database |

### YAML Config File

```yaml
namespace: virtwork-prod
container_disk_image: quay.io/containerdisks/fedora:41
data_disk_size: 20Gi

ssh_user: virtwork
ssh_authorized_keys:
  - ssh-ed25519 AAAA...

workloads:
  cpu:
    enabled: true
    vm_count: 2
    cpu_cores: 4
    memory: 4Gi
  database:
    enabled: true
    cpu_cores: 2
    memory: 4Gi
```

## Audit Tracking

Every execution is tracked in a local SQLite database for operational visibility. Each `virtwork run` and `virtwork cleanup` generates a UUID applied as a `virtwork/run-id` label on all K8s resources.

The audit database records execution parameters, timestamps, workload details, VM details, resource details, and events. During cleanup, run IDs are collected from resources and linked back to the cleanup record.

No SSH credentials are stored — only a boolean indicating whether SSH authentication was configured.

For the full schema (five tables with relationships and column descriptions) and common query patterns, see [docs/audit-schema.md](docs/audit-schema.md).

```bash
# Disable audit tracking
virtwork run --no-audit

# Use a custom database path
virtwork run --audit-db /path/to/audit.db

# Query recent executions
sqlite3 virtwork.db "SELECT run_id, command, status, started_at FROM audit_log ORDER BY id DESC LIMIT 10;"

# Query VMs from a specific run
sqlite3 virtwork.db "SELECT vm_name, component, cpu_cores, memory FROM vm_details WHERE audit_id = 1;"

# Query events timeline
sqlite3 virtwork.db "SELECT event_type, message, occurred_at FROM events WHERE audit_id = 1 ORDER BY occurred_at;"
```

## SSH Access

VMs can be configured with SSH access for debugging and inspection.

```bash
# Deploy with SSH key
virtwork run --ssh-user virtwork --ssh-key-file ~/.ssh/id_ed25519.pub

# Access via virtctl
virtctl ssh --ssh-key ~/.ssh/id_ed25519 virtwork@virtwork-cpu-0

# Access via port forward
oc port-forward vmi/virtwork-cpu-0 2222:22
ssh -p 2222 virtwork@localhost
```

When no SSH flags are provided, no user account is configured in the VMs.

> **Note:** SSH passwords passed via `--ssh-password` are visible in process listings and stored as plaintext in the VM spec. Use SSH key authentication for anything beyond test environments.

## OpenShift Deployment

virtwork can run as a pod on the cluster using the provided Kustomize manifests in `deploy/`. The deployment manifest uses a semantic version tag (e.g., `v0.0.1`) to pin the image for reproducible deployments. Update the version tag in `deploy/deployment.yaml` when upgrading. For development or testing, you can use `quay.io/opdev/virtwork:latest`.

The section below is a quick reference. For a manifest-by-manifest deep-dive — resource topology, RBAC scope, image pinning policy, sizing, audit-DB persistence — see [docs/deployment.md](docs/deployment.md).

### Deploy with Kustomize

```bash
oc apply -k deploy/
```

This creates:
- A `virtwork` namespace with a ServiceAccount and RBAC for managing VMs, Services, and Secrets
- A ConfigMap with default configuration (editable)
- A Secret for sensitive values (SSH password)
- A PVC for the audit database
- A Deployment running the virtwork container

### Container Configuration

The pod behavior is controlled by two environment variables in the Deployment:

| Variable | Description |
|----------|-------------|
| `VIRTWORK_COMMAND` | Set to `run` or `cleanup` to auto-execute on pod start. Leave empty for interactive mode. |
| `VIRTWORK_ARGS` | Additional CLI arguments (e.g., `--workloads cpu,memory --vm-count 2`) |

When `VIRTWORK_COMMAND` is empty, the pod sleeps indefinitely. Use `oc exec` to run virtwork commands interactively:

```bash
oc exec -it deploy/virtwork -- virtwork run --dry-run
oc exec -it deploy/virtwork -- virtwork run --workloads cpu,memory
oc exec -it deploy/virtwork -- virtwork cleanup
```

### Building the Container Image

```bash
podman build -t quay.io/opdev/virtwork:latest .

# The Dockerfile uses a multi-stage build:
# Stage 1: golang:1.26-alpine for pure-Go compilation (CGO_ENABLED=0)
# Stage 2: ubi9/ubi-minimal for a minimal runtime
```

## Architecture

The codebase follows a strict layered architecture where each layer depends only on layers below it.

```
Layer 4 — Orchestration     cmd/virtwork, orchestrator, cleanup
Layer 3 — Workload Defs     workloads (Workload + MultiVMWorkload interfaces, registry,
                            9 built-in + catalog system: GenericWorkload,
                            GenericMultiWorkload for no-code extensions)
Layer 2 — K8s Abstractions  vm, resources, wait
Layer 1 — Infrastructure    config, cluster, cloudinit, logging, audit
Layer 0 — Definitions       constants
```

Concurrency uses goroutines with `errgroup.Group` for structured error handling and `context.Context` for timeouts and cancellation. VM creation, readiness polling, and cleanup all run concurrently.

See [docs/architecture.md](docs/architecture.md) for detailed diagrams and design decisions.

## Project Structure

```
virtwork/
├── cmd/virtwork/main.go           # Cobra CLI + orchestration
├── internal/
│   ├── constants/                 # API coordinates, labels, defaults
│   ├── config/                    # Viper-based config priority chain
│   ├── cluster/                   # controller-runtime client init
│   ├── cloudinit/                 # Cloud-config YAML builder
│   ├── logging/                   # Structured logger (log/slog wrapper)
│   ├── vm/                        # VM spec construction + CRUD + retry
│   ├── resources/                 # Namespace + Service + Secret helpers
│   ├── wait/                      # VMI readiness polling
│   ├── cleanup/                   # Label-based teardown (VMs, Services, Secrets)
│   ├── orchestrator/              # Run + cleanup orchestration logic
│   ├── audit/                     # SQLite audit tracking (Auditor interface, schema, records)
│   ├── workloads/                 # Workload + MultiVMWorkload interfaces, 9 built-in + catalog (generic, generic_multi), registry, param schemas
│   └── testutil/                  # Shared test helpers for integration + E2E
├── tests/
│   └── e2e/                       # E2E acceptance tests (//go:build e2e)
├── build/
│   └── golden-image/              # Optional Fedora container disk with pre-installed tools
├── deploy/                        # Kustomize manifests for OpenShift deployment
│   ├── kustomization.yaml
│   ├── namespace.yaml
│   ├── serviceaccount.yaml
│   ├── rbac.yaml
│   ├── configmap.yaml
│   ├── secret.yaml
│   ├── pvc.yaml
│   └── deployment.yaml
├── Dockerfile                     # Multi-stage build (Alpine builder + UBI9 runtime)
├── Dockerfile.ci                  # CI variant of the runtime image
├── entrypoint.sh                  # Container entrypoint (auto-run or sleep)
├── Makefile                       # CI targets (test, vet, lint, build, ci, verify)
├── docs/
│   ├── README.md                  # Documentation index
│   ├── architecture.md            # Layered architecture and diagrams
│   ├── development.md             # Developer guide
│   ├── configuration.md           # Complete config reference (flags, env, YAML)
│   ├── deployment.md              # OpenShift deployment deep-dive
│   ├── audit-schema.md            # SQLite audit schema reference
│   ├── chaos-workloads.md         # Chaos engineering workload guide
│   ├── virtwork-vs-kube-burner.md # Positioning vs kube-burner
│   ├── guide/                     # Hands-on guides (overview, deploying, adding workloads, catalog workloads)
│   ├── mermaid/                   # Standalone mermaid diagram source files
│   ├── implementation-plan.md     # Historical: original phased build plan
│   └── openshift-virtualization-workload-automation.md  # Historical: original design rationale
├── OWNERS
├── go.mod
└── go.sum
```

## Development

### Testing

```bash
# Unit tests
go test ./...

# With race detector
go test -race ./...

# Using Ginkgo BDD runner
ginkgo -r

# Integration tests (requires cluster with KubeVirt/CNV)
go test -tags integration ./internal/...

# E2E tests (requires cluster + builds binary automatically)
go test -tags e2e ./tests/e2e/...

# All tests
go test -tags "integration e2e" ./...
```

### Building

```bash
go build -o virtwork ./cmd/virtwork
```

### Makefile shortcuts

```bash
make help          # list all targets
make test          # unit tests with race detector and coverage
make ci            # vet + test + build (no cluster required)
make verify        # fmt + vet + lint + test (full pre-commit)
make build         # build binary to bin/virtwork
make container-build  # build the OCI image locally
```

See [docs/development.md](docs/development.md) for the full developer guide, including instructions for adding new workloads.

## License

Apache License 2.0. See [LICENSE](LICENSE).
