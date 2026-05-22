# Plan: OpenShift Virtualization Workload Automation ("virtwork")

> **Historical snapshot — original design rationale.**
>
> This document captured the motivation and design choices at project inception, when no application code existed yet. It is preserved as context for *why* the layered architecture, the deploy-and-exit model, and the systemd-inside-VM workload lifecycle were chosen. **Implementation has since evolved.** For the current state, see:
>
> - Architecture and design decisions → [architecture.md](architecture.md)
> - Built-in workloads (now nine total) → [README.md](../README.md#workloads), [architecture.md](architecture.md), [chaos-workloads.md](chaos-workloads.md)
> - Developer guide and patterns → [development.md](development.md)
> - Configuration reference → [configuration.md](configuration.md)
> - OpenShift deployment → [deployment.md](deployment.md)

## Context

This codebase (`virtwork`) is a Go CLI tool for creating VMs on an OpenShift cluster with OpenShift Virtualization (CNV) already installed. The goal is to run continuous, varied workloads (CPU, memory, database, network, disk I/O) inside those VMs so that monitoring tools (Prometheus, Grafana, etc.) have realistic metrics to observe. The codebase currently has a `go.mod` and documentation — no application code yet.

---

## Project Structure

```
virtwork/
├── cmd/
│   └── virtwork/
│       └── main.go                # Entry point: Cobra root command
├── internal/
│   ├── config/
│   │   └── config.go              # Config struct, Viper priority chain
│   ├── constants/
│   │   └── constants.go           # API groups, versions, label keys, defaults
│   ├── cluster/
│   │   └── cluster.go             # controller-runtime client init
│   ├── cloudinit/
│   │   └── cloudinit.go           # Cloud-init YAML builder
│   ├── vm/
│   │   └── vm.go                  # VM spec construction + CRUD
│   ├── resources/
│   │   └── resources.go           # Namespace + Service helpers
│   ├── wait/
│   │   └── wait.go                # Poll VMI status until Running
│   ├── cleanup/
│   │   └── cleanup.go             # Teardown by label selector
│   └── workloads/
│       ├── workload.go            # Workload interface
│       ├── registry.go            # Registry map + lookup
│       ├── cpu.go                 # stress-ng CPU continuous workload
│       ├── memory.go              # stress-ng VM memory pressure workload
│       ├── database.go            # PostgreSQL + pgbench loop
│       ├── network.go             # iperf3 server/client pair
│       └── disk.go                # fio mixed I/O profiles
├── tests/
│   ├── integration/               # Integration tests (build tag)
│   └── e2e/                       # E2E tests (build tag)
├── docs/
├── go.mod
├── go.sum
└── CLAUDE.md
```

---

## Implementation Steps

### 1. Update `go.mod` and add dependencies

```
module virtwork

go 1.25

require (
    github.com/spf13/cobra v1.9+
    github.com/spf13/viper v1.20+
    gopkg.in/yaml.v3 v3.0+
    sigs.k8s.io/controller-runtime v0.20+
    kubevirt.io/api v1.5+
    kubevirt.io/containerized-data-importer-api v1.62+
    k8s.io/api v0.32+
    k8s.io/apimachinery v0.32+
    k8s.io/client-go v0.32+
)
```

Development dependencies (test tooling):
```
github.com/onsi/ginkgo/v2 v2.23+
github.com/onsi/gomega v1.37+
```

### 2. Create `internal/constants/constants.go`

- KubeVirt API coordinates: group `kubevirt.io`, version `v1`, plurals for `virtualmachines` and `virtualmachineinstances`
- CDI API coordinates: group `cdi.kubevirt.io`, version `v1beta1`, plural for `datavolumes`
- Default container disk image: `quay.io/containerdisks/fedora:41`
- Standard Kubernetes labels (`app.kubernetes.io/name`, `managed-by`, `component`)
- Default values: namespace=`virtwork`, cpu=2, memory=`2Gi`, disk=`10Gi`
- Polling defaults: timeout=600s, interval=15s

### 3. Create `internal/config/config.go`

- `WorkloadConfig` struct: `Enabled bool`, `VMCount int`, `CPUCores int`, `Memory string`
- `Config` struct: `Namespace`, `ContainerDiskImage`, `DataDiskSize`, workloads map, `KubeconfigPath`, `CleanupMode`, `WaitForReady`, `ReadyTimeoutSeconds`, `DryRun`, `Verbose`, `SSHUser`, `SSHPassword`, `SSHAuthorizedKeys`
- Viper-based loading: Cobra flags > env vars (`VIRTWORK_*`) > YAML config file > defaults
- Scalar fields map directly through Viper; list fields (`SSHAuthorizedKeys`) need special handling at each config layer (YAML=list, env=comma-separated, CLI=merged from `--ssh-key` and `--ssh-key-file`)

### 4. Create `internal/cluster/cluster.go`

- `Connect(kubeconfigPath string) (client.Client, error)` function
- Uses controller-runtime's `client.New()` with rest config
- Try in-cluster config first via `rest.InClusterConfig()`, fall back to `clientcmd.BuildConfigFromFlags()`
- Registers KubeVirt and CDI types in the runtime scheme

### 5. Create `internal/cloudinit/cloudinit.go`

- `BuildCloudConfig(opts CloudConfigOpts) (string, error)`
- `CloudConfigOpts` struct: `Packages []string`, `WriteFiles []WriteFile`, `RunCmd [][]string`, `Extra map[string]interface{}`, `SSHUser string`, `SSHPassword string`, `SSHAuthorizedKeys []string`
- Returns `#cloud-config\n` + YAML marshal
- Omits keys with empty/nil values
- When `SSHUser` is set, emits a `users` block with sudo access, shell, and optional password/authorized keys
- `lock_passwd: true` when no password (key-only auth); `lock_passwd: false` + `plain_text_passwd` when password provided
- `ssh_pwauth: true` at top level only when password is set

### 6. Create workloads

**`internal/workloads/workload.go`** — Interfaces and base struct:
```go
type Workload interface {
    Name() string
    CloudInitUserdata() (string, error)
    VMResources() VMResourceSpec
    ExtraVolumes() []kubevirtv1.Volume
    ExtraDisks() []kubevirtv1.Disk
    DataVolumeTemplates() []cdiv1beta1.DataVolumeTemplateSpec
    RequiresService() bool
    ServiceSpec(namespace string) *corev1.Service
    VMCount() int
}

// MultiVMWorkload extends Workload for workloads needing per-role userdata (e.g., network).
type MultiVMWorkload interface {
    Workload
    UserdataForRole(role string, namespace string) (string, error)
}
```

**SSH credential passthrough:** `BaseWorkload` stores optional SSH fields (`SSHUser`, `SSHPassword`, `SSHAuthorizedKeys`) and provides a `BuildCloudConfig()` helper method that injects them into every `cloudinit.BuildCloudConfig()` call. Concrete workloads call `w.BuildCloudConfig(opts)` instead of `cloudinit.BuildCloudConfig(opts)` directly — a single point of injection for the cross-cutting SSH concern. The registry passes SSH credentials via functional options on the constructor.

**`internal/workloads/cpu.go`** — stress-ng CPU
- Installs `stress-ng`, creates systemd service
- `stress-ng --cpu 0 --cpu-method all --timeout 0` (all CPUs, varied methods, runs forever)

**`internal/workloads/memory.go`** — stress-ng VM memory pressure
- Installs `stress-ng`, creates systemd service
- `stress-ng --vm 1 --vm-bytes 80% --vm-method all --timeout 0` (single VM worker, 80% of available memory, rotates all memory stressor methods)
- 80% target provides meaningful pressure without triggering immediate OOM-kill
- No extra volumes, disks, or services needed

**`internal/workloads/database.go`** — PostgreSQL + pgbench
- Needs a blank DataVolume for `/var/lib/pgsql/data`
- Cloud-init: install `postgresql-server`, setup script formats/mounts data disk, inits DB, creates `pgbench` database with scale factor 50
- Uses `ExecStartPre` for database initialization (format, mount, initdb) and `ExecStart` for the benchmark loop — separating one-time setup from the continuous workload
- Systemd service loops: `pgbench -c 10 -j 2 -T 300` (5-min bursts, 10s pause)

**`internal/workloads/network.go`** — iperf3 server/client
- Always creates 2 VMs (server + client)
- Creates a Kubernetes `Service` selecting the server VM by label `virtwork/role: server`
- Server: `iperf3 -s` via systemd (runs forever)
- Client: systemd loops `iperf3 -c <service-dns> -t 60 -P 4 --bidir` (60s tests, 15s pause)
- Client uses DNS name `virtwork-iperf3-server.<namespace>.svc.cluster.local` — no IP polling needed
- Retry-on-fail: systemd `Restart=always` handles server not yet ready
- Constructor accepts `namespace` parameter (in addition to `WorkloadConfig`) for DNS name construction

**`internal/workloads/disk.go`** — fio
- Needs a blank DataVolume for `/mnt/data`
- Two fio profiles written via cloud-init as separate `write_files` entries (bench script separated from systemd unit for clarity):
  - `mixed-rw.fio`: 4K random R/W, 70/30 mix, 4 jobs, 5 min
  - `seq-write.fio`: 128K sequential write, 2 jobs, 5 min
- Systemd service alternates between profiles with 10s pauses

### 7. Create `internal/vm/vm.go`

- `BuildVMSpec(opts VMSpecOpts) *kubevirtv1.VirtualMachine`
  - Uses `containerDisk` for OS (fast pull, ephemeral root is fine for workload VMs)
  - `cloudInitNoCloud` with embedded userdata
  - Masquerade networking (`pod` network)
  - Virtio bus for all disks
  - `spec.Running: true` to auto-start
- `BuildDataVolumeTemplate(name, size string) cdiv1beta1.DataVolumeTemplateSpec` for blank data disks
- `CreateVM(ctx context.Context, c client.Client, vm *kubevirtv1.VirtualMachine) error` — AlreadyExists=skip, retry on transient errors
- `DeleteVM(ctx context.Context, c client.Client, name, namespace string) error`
- `ListVMs(ctx context.Context, c client.Client, namespace string, labels map[string]string) ([]kubevirtv1.VirtualMachine, error)`
- `GetVMIPhase(ctx context.Context, c client.Client, name, namespace string) (kubevirtv1.VirtualMachineInstancePhase, error)`
- Retry logic: AlreadyExists=skip, rate-limited/5xx=retry with backoff, NotFound=fatal "CNV not installed?", Unauthorized/Forbidden=fatal auth error

### 8. Create `internal/resources/resources.go`

- `EnsureNamespace(ctx context.Context, c client.Client, name string, labels map[string]string) error` — create if not exists (AlreadyExists=skip)
- `CreateService(ctx context.Context, c client.Client, svc *corev1.Service) error` — AlreadyExists=skip
- `DeleteManagedServices(ctx context.Context, c client.Client, namespace string, labels map[string]string) (int, error)` — returns count

### 9. Create `internal/wait/wait.go`

- `WaitForVMReady(ctx context.Context, c client.Client, name, namespace string, timeout, interval time.Duration) error`
- Polls VMI phase, returns nil when `phase == Running`, returns error on timeout
- `WaitForAllVMsReady(ctx context.Context, c client.Client, names []string, namespace string, timeout, interval time.Duration) map[string]error`
- Uses `errgroup.Group` for concurrent polling

### 10. Create `internal/cleanup/cleanup.go`

- `CleanupAll(ctx context.Context, c client.Client, namespace string, deleteNamespace bool) (*CleanupResult, error)`
- `CleanupResult` struct: `VMsDeleted int`, `ServicesDeleted int`, `NamespaceDeleted bool`, `Errors []error`
- List all VMs with label `app.kubernetes.io/managed-by=virtwork`
- Delete each VM (errors logged, not fatal)
- Delete managed Services
- Optionally delete namespace

### 11. Create `cmd/virtwork/main.go` with Cobra commands

- Root command with persistent flags: `--namespace`, `--kubeconfig`, `--config`, `--verbose`
- `run` subcommand (default): `--workloads`, `--vm-count`, `--cpu-cores`, `--memory`, `--disk-size`, `--container-disk-image`, `--no-wait`, `--timeout`, `--dry-run`, `--ssh-user`, `--ssh-password`, `--ssh-key`, `--ssh-key-file`
- `cleanup` subcommand: `--delete-namespace`
- `run` orchestration flow (`runCmd.RunE`):
  1. Load config via Viper (Cobra flags > env > file > defaults)
  2. If dry-run: generate specs, print as YAML, exit
  3. Connect to cluster via controller-runtime
  4. Ensure namespace
  5. For each enabled workload:
     - Get workload from registry, passing SSH credentials via functional options
     - If `VMCount() > 1` and workload implements `MultiVMWorkload` interface: iterate roles (server, client), call `UserdataForRole()` for each, add `virtwork/role` label per VM
     - Otherwise: single `CloudInitUserdata()` call
     - Create Service if `RequiresService()` (must exist before client VM for DNS resolution)
     - Build VM spec(s), create VM(s) — parallel via errgroup
  6. If not `--no-wait`: poll for readiness via errgroup
  7. Print summary table
- `cleanup` orchestration flow (`cleanupCmd.RunE`):
  1. Load config via Viper
  2. Connect to cluster
  3. Delete all labeled resources (`cleanup.CleanupAll()`)
  4. Print cleanup summary

---

## Key Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Boot disk | `containerDisk` | Fast (kubelet image pull), cached on nodes, no CDI import wait. Ephemeral root is fine for workload VMs. |
| Data disk | Blank `DataVolume` | Formatted on first boot by cloud-init. Only needed for database and fio workloads. |
| Workload lifecycle | systemd services | Survive reboots, auto-restart on failure, proper logging via journald. |
| Network coordination | K8s Service + DNS | No IP polling from Go. Client retries via systemd `Restart=always` until server is ready. |
| Cleanup tracking | Label selectors | No state file needed. Works even if tool crashed mid-creation. |
| Auth | In-cluster first, kubeconfig fallback | Works both inside pods (CI/CD) and from developer machines. |
| Concurrency | goroutines + errgroup | Native Go concurrency. Parallel VM creation and readiness polling with structured error handling. |
| K8s client | controller-runtime `client.Client` | Higher-level than raw client-go. Typed operations with KubeVirt/CDI API types. Common in OpenShift ecosystem. |
| CLI framework | Cobra + Viper | Standard Go CLI stack. Viper integrates with Cobra for config priority chain. |
| Testing | Ginkgo v2 + Gomega | BDD framework with expressive matchers. Native Describe/Context/It blocks. |
| Idempotency | AlreadyExists = skip | Safe to re-run. Enables declarative approach. |
| Retry | Backoff for rate-limited/5xx | Handles transient cluster issues. NotFound/Unauthorized/Forbidden are fatal. |
| Multi-VM detection | `VMCount() > 1` + `MultiVMWorkload` type assertion | Keeps orchestration generic; future multi-VM workloads work without changing orchestration code. |
| SSH credential injection | `BaseWorkload.BuildCloudConfig()` helper | Each workload calls one helper method instead of passing SSH args individually. Single-word change per workload. |
| Cleanup error semantics | Inline iteration with per-resource error capture | Existing delete functions raise on error (correct for create-time). Cleanup needs continue-on-error — different semantics justify separate implementation. |
| Subcommand separation | `run` and `cleanup` are separate Cobra subcommands; dry-run is an early-return within `run` | Distinct flag sets and error semantics (fail-fast vs continue-on-error) justify separate commands. Each is a linear sequence. |
| Registry functional options | `Get(name, config, ...Option)` | NetworkWorkload needs `namespace` beyond standard `WorkloadConfig`. Functional options keep the registry interface extensible without breaking changes. |

---

## Verification

1. **Dry run**: `go run ./cmd/virtwork run --dry-run` — prints all VM specs as YAML, no cluster needed
2. **Create resources**: `go run ./cmd/virtwork run --namespace virtwork-test --verbose`
3. **Check VMs**: `oc get vm -n virtwork-test` — all VMs should show `Running`
4. **Check VMIs**: `oc get vmi -n virtwork-test` — all VMIs should show `Running` with IPs
5. **Verify workloads inside VMs**: `virtctl console virtwork-cpu-0` then `systemctl status virtwork-cpu`
6. **Verify SSH access**: `virtctl ssh --ssh-key ~/.ssh/id_rsa virtwork@virtwork-cpu-0` (when `--ssh-key-file` or `--ssh-password` was provided)
7. **Verify monitoring**: Check Prometheus/Grafana for CPU, memory, disk, network, and database metrics from the VMs
8. **Cleanup**: `go run ./cmd/virtwork cleanup --namespace virtwork-test` — all resources removed
9. **Idempotency**: Run create twice — second run should skip existing resources (AlreadyExists handled)
