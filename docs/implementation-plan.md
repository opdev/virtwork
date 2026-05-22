# Virtwork Implementation Plan

> **Historical snapshot — original phased build (phases 0–12).**
>
> This document describes the original incremental plan executed to build virtwork through phase 12. It is preserved as a record of the layered design and as context for why packages are organized the way they are. **Subsequent additions are not reflected here.** For the current state, see:
>
> - Chaos workloads (chaos-disk, chaos-network, chaos-process) → [chaos-workloads.md](chaos-workloads.md)
> - TPS workload → [architecture.md](architecture.md) and [guide/02-deploying-workloads.md](guide/02-deploying-workloads.md)
> - Structured logging (`internal/logging`) → [architecture.md](architecture.md) and [development.md](development.md)
> - Golden container disk image → [../build/golden-image/README.md](../build/golden-image/README.md)
> - Audit schema (5 tables) → [audit-schema.md](audit-schema.md)
> - Complete configuration reference → [configuration.md](configuration.md)
> - OpenShift deployment deep-dive → [deployment.md](deployment.md)

This document breaks the [design plan](openshift-virtualization-workload-automation.md) into incremental phases. Each phase produces a testable, committable increment that builds on the previous one. See [architecture.md](architecture.md) for the high-level architecture and diagrams.

---

## Phase Summary

| Phase | Layer | Packages | Tests (approx) | Goroutines? |
|-------|-------|----------|-----------------|-------------|
| 0 | — | Project scaffold, go.mod, Ginkgo bootstrap | 1 smoke | No |
| 1 | 0+1 | `internal/constants`, `internal/config` | 14-16 | No |
| 2 | 1 | `internal/cloudinit` | 18-20 | No |
| 3 | 1 | `internal/cluster` | 5 | No |
| 4 | 2 | `internal/vm` | 18-20 | Yes (CRUD) |
| 5 | 2 | `internal/resources`, `internal/wait` | 11 | Yes |
| 6 | 3 | `internal/workloads` (interface, cpu, memory, disk) | 22-26 | No |
| 7 | 3 | `internal/workloads` (database, network) | 14-16 | No |
| 8 | 3 | `internal/workloads` (registry) | 10 | No |
| 9 | 4 | `internal/cleanup` | 7-10 | Yes |
| 10 | 4 | `cmd/virtwork` (Cobra CLI + orchestration) | 22-25 + BDD | Yes |
| 11 | — | Integration tests (alongside source) + E2E tests (`tests/e2e/`) | ~51 | Yes |
| 12 | 4 | `internal/audit` (SQLite audit tracking) | 14 | No |
| **Total** | | **12 packages + testutil + e2e** | **~220 tests** | |

**Note:** Test counts increased from the initial estimate to account for SSH credential support (cloud-init users block, config fields, CLI flags) and the memory workload. These are lessons from the Python implementation experience that revealed the true scope.

---

## Phase 0: Project Foundation

**Goal:** Establish the project skeleton so that `go test ./...` passes and the Ginkgo test suite is bootstrapped.

### Files to Create

- `cmd/virtwork/main.go` — minimal `func main()` placeholder
- `internal/constants/constants.go` — package declaration only
- `internal/config/config.go` — package declaration only
- `internal/cluster/cluster.go` — package declaration only
- `internal/cloudinit/cloudinit.go` — package declaration only
- `internal/vm/vm.go` — package declaration only
- `internal/resources/resources.go` — package declaration only
- `internal/wait/wait.go` — package declaration only
- `internal/cleanup/cleanup.go` — package declaration only
- `internal/workloads/workload.go` — package declaration only

### Ginkgo Bootstrap

Bootstrap Ginkgo test suites for each package:

```bash
cd internal/constants && ginkgo bootstrap
cd internal/config && ginkgo bootstrap
cd internal/cloudinit && ginkgo bootstrap
cd internal/cluster && ginkgo bootstrap
cd internal/vm && ginkgo bootstrap
cd internal/resources && ginkgo bootstrap
cd internal/wait && ginkgo bootstrap
cd internal/cleanup && ginkgo bootstrap
cd internal/workloads && ginkgo bootstrap
```

Create a smoke test:

- `internal/constants/constants_test.go` — one `Describe` block asserting the package is importable

### Files to Modify

- `go.mod`:
  - Add required dependencies: `github.com/spf13/cobra`, `github.com/spf13/viper`, `gopkg.in/yaml.v3`
  - Add K8s dependencies: `sigs.k8s.io/controller-runtime`, `k8s.io/api`, `k8s.io/apimachinery`, `k8s.io/client-go`
  - Add KubeVirt dependencies: `kubevirt.io/api`, `kubevirt.io/containerized-data-importer-api`
  - Add test dependencies: `github.com/onsi/ginkgo/v2`, `github.com/onsi/gomega`

### Verification

- `go build ./...` succeeds
- `go test ./...` → 1 test collected, 1 passed
- `ginkgo -r` runs the smoke test

### Commits

- `chore: scaffold virtwork package structure and Ginkgo test infrastructure`

---

## Phase 1: Constants and Config

**Goal:** Define all project constants and the Viper-based configuration system. Pure Go — no external system interaction.

### Files to Create

**`internal/constants/constants.go`**
- KubeVirt API: `KubevirtAPIGroup`, `KubevirtAPIVersion`, `KubevirtVMPlural`, `KubevirtVMIPlural`
- CDI API: `CDIAPIGroup`, `CDIAPIVersion`, `CDIDVPlural`
- Defaults: `DefaultContainerDiskImage` (quay.io/containerdisks/fedora:41), `DefaultNamespace` (virtwork), `DefaultCPUCores` (2), `DefaultMemory` (2Gi), `DefaultDiskSize` (10Gi), `DefaultSSHUser` (virtwork)
- Labels: `LabelAppName`, `LabelManagedBy`, `LabelComponent`, `ManagedByValue`
- Polling: `DefaultReadyTimeout` (600 * time.Second), `DefaultPollInterval` (15 * time.Second)

**`internal/config/config.go`**
- `WorkloadConfig` struct: `Enabled bool`, `VMCount int`, `CPUCores int`, `Memory string`
- `Config` struct: `Namespace`, `ContainerDiskImage`, `DataDiskSize`, `Workloads map[string]WorkloadConfig`, `KubeconfigPath`, `CleanupMode`, `WaitForReady`, `ReadyTimeoutSeconds`, `DryRun`, `Verbose`, `SSHUser`, `SSHPassword`, `SSHAuthorizedKeys`
- List fields (`SSHAuthorizedKeys`) need explicit handling outside the generic Viper binding loop: YAML passes lists directly, env vars split on commas, CLI merges from `--ssh-key` and `--ssh-key-file`
- `LoadConfig(cmd *cobra.Command) (*Config, error)` — reads from Viper (Cobra flags > env > YAML > defaults)
- `SetDefaults()` — registers Viper defaults
- `BindFlags(cmd *cobra.Command)` — binds Cobra flags to Viper keys

### Tests (write first — TDD)

**`internal/constants/constants_test.go`** (Ginkgo):
```go
Describe("Constants", func() {
    It("should have correct KubeVirt API group", ...)
    It("should have correct default namespace", ...)
    It("should have valid label key format", ...)
})
```

**`internal/config/config_test.go`** (Ginkgo):
```go
Describe("Config", func() {
    Context("with defaults", func() {
        It("should have correct default namespace", ...)
        It("should have correct default CPU cores", ...)
    })
    Context("with YAML config file", func() {
        It("should load namespace from file", ...)
        It("should return error for invalid YAML", ...)
        It("should return error for missing file", ...)
    })
    Context("with environment variables", func() {
        It("should override defaults with VIRTWORK_ env vars", ...)
    })
    Context("priority chain", func() {
        It("should prefer flags over env vars", ...)
        It("should prefer env vars over config file", ...)
    })
    Context("SSH config fields", func() {
        It("should default SSHUser to virtwork", ...)
        It("should default SSHPassword to empty", ...)
        It("should default SSHAuthorizedKeys to empty", ...)
        It("should split comma-separated VIRTWORK_SSH_AUTHORIZED_KEYS", ...)
    })
})
```

### Verification

- `go test ./internal/constants/... ./internal/config/...` all pass
- `ginkgo -r internal/constants internal/config` all pass

### Commits

- `feat: add constants package with KubeVirt API coordinates and defaults`
- `feat: add config package with Viper-based priority chain`

---

## Phase 2: Cloud-init Builder

**Goal:** Build the `cloudinit` package — pure string/YAML manipulation, no K8s dependency.

### Files to Create

**`internal/cloudinit/cloudinit.go`**
- `CloudConfigOpts` struct: `Packages []string`, `WriteFiles []WriteFile`, `RunCmd [][]string`, `Extra map[string]interface{}`, `SSHUser string`, `SSHPassword string`, `SSHAuthorizedKeys []string`
- `WriteFile` struct: `Path string`, `Content string`, `Permissions string`
- `BuildCloudConfig(opts CloudConfigOpts) (string, error)`
- Returns `#cloud-config\n` + YAML marshal
- Omits keys with empty/nil values
- When `SSHUser` is non-empty, emits a `users` block:
  - `name`, `sudo: ALL=(ALL) NOPASSWD:ALL`, `shell: /bin/bash`
  - `lock_passwd: true` when no password (key-only auth), `lock_passwd: false` + `plain_text_passwd` when password set
  - `ssh_authorized_keys` list when keys provided
  - `ssh_pwauth: true` at top level only when password is set
- No `users` block when `SSHUser` is empty (backward compatible)

### Tests (TDD)

**`internal/cloudinit/cloudinit_test.go`** (Ginkgo):
```go
Describe("BuildCloudConfig", func() {
    It("should return valid #cloud-config header for empty opts", ...)
    It("should include packages list", ...)
    It("should include write_files with correct structure", ...)
    It("should include runcmd entries", ...)
    It("should merge extra keys at top level", ...)
    It("should omit nil/empty values", ...)
    It("should produce output parseable by yaml.Unmarshal", ...)
    It("should have exact #cloud-config header", ...)

    Context("SSH user support", func() {
        It("should create users block when SSHUser is set", ...)
        It("should set lock_passwd true when no password", ...)
        It("should set lock_passwd false and plain_text_passwd when password set", ...)
        It("should set ssh_pwauth true only when password is set", ...)
        It("should include ssh_authorized_keys when keys provided", ...)
        It("should handle multiple authorized keys", ...)
        It("should handle combined password and keys", ...)
        It("should not create users block when SSHUser is empty", ...)
        It("should not include ssh_authorized_keys when keys list is empty", ...)
        It("should coexist with workload params (packages, runcmd)", ...)
    })
})
```

**Implementation lesson from Python:** When testing YAML output, always parse the YAML string with `yaml.Unmarshal` before asserting on values — never assert on raw string content. YAML serializers may reorder keys, fold long lines (Go's `gopkg.in/yaml.v3` handles this differently than Python's PyYAML, but the principle holds), or vary whitespace.

### Verification

- `go test ./internal/cloudinit/...` all pass
- Output round-trips through `yaml.Unmarshal`

### Commits

- `feat: add cloud-init YAML builder`

---

## Phase 3: Cluster Client

**Goal:** controller-runtime client initialization — first package with external system interaction.

### Files to Create

**`internal/cluster/cluster.go`**
- `Connect(kubeconfigPath string) (client.Client, error)`
  - Build runtime scheme with core types, KubeVirt types, CDI types
  - Try `rest.InClusterConfig()`
  - On error, fall back to `clientcmd.BuildConfigFromFlags("", kubeconfigPath)`
  - Both fail → return wrapped error
  - Create `client.New(restConfig, client.Options{Scheme: scheme})`
- `NewScheme() *runtime.Scheme` — registers all needed types

### Tests (TDD)

**`internal/cluster/cluster_test.go`** (Ginkgo):
```go
Describe("Connect", func() {
    It("should return error when both in-cluster and kubeconfig fail", ...)
    It("should use kubeconfig path when provided", ...)
})
Describe("NewScheme", func() {
    It("should register core types", ...)
    It("should register KubeVirt types", ...)
    It("should register CDI types", ...)
})
```

### Verification

- `go test ./internal/cluster/...` all pass
- `Connect()` returns `(client.Client, error)`

### Commits

- `feat: add cluster client initialization with controller-runtime and scheme registration`

---

## Phase 4: VM Spec Building and CRUD

**Goal:** VM spec construction (pure functions) + K8s CRUD operations with retry logic. Largest single package.

### Files to Create

**`internal/vm/vm.go`**

Types:
- `VMSpecOpts` struct: `Name`, `Namespace`, `ContainerDiskImage`, `CloudInitUserdata`, `CPUCores`, `Memory`, `Labels`, `ExtraDisks`, `ExtraVolumes`, `DataVolumeTemplates`

Pure functions:
- `BuildVMSpec(opts VMSpecOpts) *kubevirtv1.VirtualMachine`
  - `containerDisk` for OS, `cloudInitNoCloud` for userdata, masquerade networking, virtio bus, `spec.Running: true`
- `BuildDataVolumeTemplate(name, size string) cdiv1beta1.DataVolumeTemplateSpec`

CRUD operations:
- `CreateVM(ctx context.Context, c client.Client, vm *kubevirtv1.VirtualMachine) error` — AlreadyExists=skip, transient=retry
- `DeleteVM(ctx context.Context, c client.Client, name, namespace string) error`
- `ListVMs(ctx context.Context, c client.Client, namespace string, labels map[string]string) ([]kubevirtv1.VirtualMachine, error)`
- `GetVMIPhase(ctx context.Context, c client.Client, name, namespace string) (kubevirtv1.VirtualMachineInstancePhase, error)`
- `retryOnTransient(ctx context.Context, fn func() error, maxRetries int) error` — unexported retry helper

### Tests (TDD)

**`internal/vm/vm_test.go`** (Ginkgo):
```go
Describe("BuildVMSpec", func() {
    It("should set correct API version and kind", ...)
    It("should set name and namespace", ...)
    It("should configure containerDisk volume", ...)
    It("should configure cloudInitNoCloud volume", ...)
    It("should set labels", ...)
    It("should set CPU and memory resources", ...)
    It("should set running to true", ...)
    It("should configure masquerade networking", ...)
    It("should include extra disks when provided", ...)
    It("should include data volume templates when provided", ...)
})

Describe("CreateVM", func() {
    It("should create VM successfully", ...)
    It("should skip on AlreadyExists", ...)
    It("should retry on transient errors", ...)
    It("should fail on NotFound", ...)
    It("should fail on Unauthorized", ...)
})

Describe("DeleteVM", func() { ... })
Describe("ListVMs", func() { ... })
Describe("GetVMIPhase", func() { ... })
```

### Verification

- `go test ./internal/vm/...` all pass
- `BuildVMSpec()` output matches KubeVirt VirtualMachine schema
- Retry logic covers all documented error conditions

### Commits

- `feat: add VM spec builder with container disk and cloud-init support`
- `feat: add VM CRUD operations with retry logic`

---

## Phase 5: Resources and Wait

**Goal:** Complete Layer 2 — namespace/service helpers and VMI readiness polling.

### Files to Create

**`internal/resources/resources.go`**
- `EnsureNamespace(ctx context.Context, c client.Client, name string, labels map[string]string) error` — create if not exists, AlreadyExists=skip
- `CreateService(ctx context.Context, c client.Client, svc *corev1.Service) error` — AlreadyExists=skip
- `DeleteManagedServices(ctx context.Context, c client.Client, namespace string, labels map[string]string) (int, error)` — returns count

**`internal/wait/wait.go`**
- `WaitForVMReady(ctx context.Context, c client.Client, name, namespace string, timeout, interval time.Duration) error` — polls VMI phase, uses `time.Sleep()`
- `WaitForAllVMsReady(ctx context.Context, c client.Client, names []string, namespace string, timeout, interval time.Duration) map[string]error` — `errgroup.Group` for concurrent polling

### Tests (TDD)

**`internal/resources/resources_test.go`** (Ginkgo):
```go
Describe("EnsureNamespace", func() {
    It("should create namespace", ...)
    It("should skip on AlreadyExists", ...)
    It("should apply labels", ...)
})
Describe("CreateService", func() {
    It("should create service", ...)
    It("should skip on AlreadyExists", ...)
})
Describe("DeleteManagedServices", func() {
    It("should return delete count", ...)
})
```

**`internal/wait/wait_test.go`** (Ginkgo):
```go
Describe("WaitForVMReady", func() {
    It("should return nil when immediately ready", ...)
    It("should return nil after N polls", ...)
    It("should return error on timeout", ...)
})
Describe("WaitForAllVMsReady", func() {
    It("should poll all VMs concurrently", ...)
    It("should report partial failures", ...)
})
```

### Verification

- `go test ./internal/resources/... ./internal/wait/...` all pass
- `WaitForVMReady` uses `time.Sleep()` not blocking the caller indefinitely
- `WaitForAllVMsReady` uses `errgroup.Group`

### Commits

- `feat: add namespace and service resource helpers`
- `feat: add VMI readiness polling with concurrent support`

---

## Phase 6: Workload Interface + CPU + Memory + Disk

**Goal:** Define the Workload interface and implement the three simpler workloads. These are pure data producers — no I/O, no goroutines.

### Files to Create

**`internal/workloads/workload.go`**
- `Workload` interface: `Name()`, `CloudInitUserdata()`, `VMResources()`, `ExtraVolumes()`, `ExtraDisks()`, `DataVolumeTemplates()`, `RequiresService()`, `ServiceSpec()`, `VMCount()`
- `VMResourceSpec` struct: `CPUCores int`, `Memory string`
- `BaseWorkload` struct: `Config WorkloadConfig`, `SSHUser string`, `SSHPassword string`, `SSHAuthorizedKeys []string` — embedded struct providing defaults for optional methods
  - `ExtraVolumes() → nil`, `ExtraDisks() → nil`, `DataVolumeTemplates() → nil`, `RequiresService() → false`, `ServiceSpec() → nil`, `VMCount() → 1`
  - `BuildCloudConfig(opts CloudConfigOpts) (string, error)` — helper that injects SSH credentials into `cloudinit.BuildCloudConfig()` calls; workloads call this instead of the package-level function

**`internal/workloads/cpu.go`**
- `CPUWorkload` struct embedding `BaseWorkload`
- Installs `stress-ng`, creates systemd service: `stress-ng --cpu 0 --cpu-method all --timeout 0`
- No extra volumes/disks/services

**`internal/workloads/memory.go`**
- `MemoryWorkload` struct embedding `BaseWorkload`
- Installs `stress-ng`, creates systemd service: `stress-ng --vm 1 --vm-bytes 80% --vm-method all --timeout 0`
- `--vm 1` (not `--vm 0`): Unlike `--cpu 0` which means "use all CPUs" (a parallelism concern), `--vm 0` would match CPU count and spawn multiple independent allocators. Memory pressure is about capacity, not parallelism, so a single worker (`--vm 1`) that targets a percentage of total memory is the correct approach.
- `--vm-bytes 80%`: Provides meaningful pressure without triggering immediate OOM-kill. At 80%, the guest OS remains functional while the memory subsystem is under sustained load. Higher values (90-100%) risk the OOM killer terminating stress-ng or other guest processes.
- `--vm-method all` rotates through all memory stressor methods (mmap/write/munmap patterns) for broad coverage of memory subsystem behavior
- No extra volumes/disks/services (structurally identical to CPU workload)

**`internal/workloads/disk.go`**
- `DiskWorkload` struct embedding `BaseWorkload`
- Installs `fio`, writes two profiles (`mixed-rw.fio`: 4K random R/W 70/30 mix; `seq-write.fio`: 128K sequential write)
- Systemd service alternates between profiles with 10s pauses
- DataVolumeTemplate for `/mnt/data`

### Tests (TDD)

**`internal/workloads/workload_test.go`** (Ginkgo):
```go
Describe("BaseWorkload", func() {
    It("should return nil for ExtraVolumes", ...)
    It("should return nil for ExtraDisks", ...)
    It("should return nil for DataVolumeTemplates", ...)
    It("should return false for RequiresService", ...)
    It("should return nil for ServiceSpec", ...)
    It("should return 1 for VMCount", ...)
})
```

**`internal/workloads/cpu_test.go`** (Ginkgo):
```go
Describe("CPUWorkload", func() {
    It("should return 'cpu' for Name", ...)
    It("should include stress-ng in packages", ...)
    It("should include systemd service in cloud-init", ...)
    It("should produce valid YAML", ...)
    It("should have no extra disks", ...)
    It("should have no service", ...)
    It("should reflect config in VMResources", ...)
})
```

**`internal/workloads/memory_test.go`** (Ginkgo):
```go
Describe("MemoryWorkload", func() {
    It("should return 'memory' for Name", ...)
    It("should include stress-ng in packages", ...)
    It("should include systemd service with --vm flag", ...)
    It("should include --vm-bytes 80% in stress-ng args", ...)
    It("should include --vm-method all in stress-ng args", ...)
    It("should produce valid YAML", ...)
    It("should have no extra disks", ...)
    It("should have no extra volumes", ...)
    It("should have no data volume templates", ...)
    It("should not require service", ...)
    It("should reflect config in VMResources", ...)
})
```

**`internal/workloads/disk_test.go`** (Ginkgo):
```go
Describe("DiskWorkload", func() {
    It("should return 'disk' for Name", ...)
    It("should include fio in packages", ...)
    It("should include fio profiles in write_files", ...)
    It("should have data volume template", ...)
    It("should have extra disk for /mnt/data", ...)
    It("should not require service", ...)
})
```

### Verification

- `go test ./internal/workloads/...` all pass
- Cloud-init output is valid YAML
- `BaseWorkload` defaults work via embedding

### Commits

- `feat: add Workload interface and BaseWorkload defaults with SSH support`
- `feat: add CPU stress-ng workload`
- `feat: add memory stress-ng VM pressure workload`
- `feat: add disk fio workload`

---

## Phase 7: Database and Network Workloads

**Goal:** Implement the two more complex workloads. Database needs DataVolume + multi-step cloud-init. Network needs two VMs + K8s Service.

### Files to Create

**`internal/workloads/database.go`**
- `DatabaseWorkload` struct embedding `BaseWorkload`
- Installs `postgresql-server`, writes setup script (format data disk, mount, initdb, create pgbench DB with scale 50)
- Uses `ExecStartPre` for one-time database initialization (format, mount, initdb) and `ExecStart` for the continuous benchmark loop — this separation ensures init runs once and the benchmark restarts cleanly on failure
- Systemd service loops `pgbench -c 10 -j 2 -T 300` with 10s pauses
- DataVolumeTemplate for `/var/lib/pgsql/data`

**`internal/workloads/network.go`**
- `NetworkWorkload` struct embedding `BaseWorkload` + `Namespace string`
- Constructor accepts `namespace` parameter (beyond standard `WorkloadConfig`) for DNS name construction — this is the primary reason the registry uses functional options
- `VMCount() → 2` (server + client)
- Implements `MultiVMWorkload` interface with `UserdataForRole(role, namespace)`
- Server role: installs `iperf3`, runs `iperf3 -s` via systemd
- Client role: installs `iperf3`, loops `iperf3 -c <dns-name> -t 60 -P 4 --bidir` via systemd
- `RequiresService() → true`
- `ServiceSpec(namespace)`: ClusterIP targeting server VM by label `virtwork/role: server`, port 5201
- Client uses DNS: `virtwork-iperf3-server.<namespace>.svc.cluster.local`

### Design Note — Network Workload and Multi-VM Detection

The `Workload` interface defines `CloudInitUserdata()` returning a single string. The network workload needs different userdata for server vs. client. Solution: the orchestration layer checks `VMCount() > 1` and type-asserts to the `MultiVMWorkload` interface to call `UserdataForRole()`. This keeps detection generic — any future multi-VM workload with per-role userdata works without changing orchestration code.

```go
// MultiVMWorkload extends Workload for workloads that need per-role userdata.
type MultiVMWorkload interface {
    Workload
    UserdataForRole(role string, namespace string) (string, error)
}
```

The orchestration adds a `virtwork/role` label to each VM (e.g., `server`, `client`). This label is what the K8s Service selector uses to route traffic to the server VM. The Service must be created before the client VM so DNS resolves correctly.

### Design Note — Registry Functional Options

The `NetworkWorkload` constructor needs a `namespace` parameter beyond the standard `WorkloadConfig`. Rather than adding every possible workload-specific parameter to the registry's `Get()` signature, use functional options:

```go
type Option func(*RegistryOpts)
func WithNamespace(ns string) Option { return func(o *RegistryOpts) { o.Namespace = ns } }

func (r Registry) Get(name string, config WorkloadConfig, opts ...Option) (Workload, error) { ... }
```

This keeps the registry interface extensible without breaking changes when future workloads need additional constructor parameters.

### Tests (TDD)

**`internal/workloads/database_test.go`** (Ginkgo):
```go
Describe("DatabaseWorkload", func() {
    It("should return 'database' for Name", ...)
    It("should include postgresql-server in packages", ...)
    It("should include setup script in cloud-init", ...)
    It("should include pgbench systemd service", ...)
    It("should have data volume template", ...)
    It("should have extra disk for data", ...)
    It("should not require service", ...)
})
```

**`internal/workloads/network_test.go`** (Ginkgo):
```go
Describe("NetworkWorkload", func() {
    It("should return 'network' for Name", ...)
    It("should return 2 for VMCount", ...)
    It("should require service", ...)
    It("should produce server userdata with iperf3 -s", ...)
    It("should produce client userdata with DNS name", ...)
    It("should have service spec with correct port", ...)
    It("should have service spec with correct selector", ...)
})
```

### Verification

- `go test ./internal/workloads/...` all pass
- Database cloud-init includes disk mount and PostgreSQL init sequence
- Network workload produces distinct server/client configs
- Service spec uses correct label selector

### Commits

- `feat: add database PostgreSQL+pgbench workload`
- `feat: add network iperf3 server/client workload`

---

## Phase 8: Workload Registry

**Goal:** Complete Layer 3 with a registry for dynamic workload lookup by name.

### Files to Create

**`internal/workloads/registry.go`**
- `Registry` map type: `map[string]WorkloadFactory`
- `Option` type and `RegistryOpts` struct for functional options (e.g., `WithNamespace()`, `WithSSHCredentials()`)
- `DefaultRegistry() Registry` — returns registry with all five workloads registered (cpu, memory, database, network, disk)
- `(r Registry) Get(name string, config WorkloadConfig, opts ...Option) (Workload, error)` — returns error listing available names for unknown workloads
- `(r Registry) List() []string` — returns sorted workload names
- `AllWorkloadNames` package-level variable (sorted: cpu, database, disk, memory, network)

### Tests (TDD)

**`internal/workloads/registry_test.go`** (Ginkgo):
```go
Describe("Registry", func() {
    It("should have 5 entries registered", ...)
    It("should return CPU workload by name", ...)
    It("should return memory workload by name", ...)
    It("should return database workload by name", ...)
    It("should return network workload by name", ...)
    It("should return disk workload by name", ...)
    It("should return error for unknown name with available names", ...)
    It("should list all names sorted alphabetically", ...)
    It("should create workloads with provided config", ...)
    It("should pass namespace option to network workload", ...)
})
```

**Implementation lesson from Python:** When adding a new workload, expect a ripple effect in registry tests (entry count, name lists) and orchestration BDD tests (total VM count assertions). Well-written tests catch this cascade immediately — the fixes are mechanical (updating expected counts and lists).

### Verification

- `go test ./internal/workloads/...` all pass
- All workloads discoverable by name string
- Unknown names produce clear error

### Commits

- `feat: add workload registry with dynamic lookup`

---

## Phase 9: Cleanup

**Goal:** Implement label-based resource teardown. Error-tolerant — individual failures don't abort.

**Key design decision from Python implementation:** The cleanup module implements deletion inline rather than reusing existing `DeleteVM()` and `DeleteManagedServices()` functions. Those functions use fail-fast error semantics (correct for create-time operations), while cleanup needs continue-on-error semantics. Different error handling requirements justify separate implementation — sometimes duplication is the lesser evil.

### Files to Create

**`internal/cleanup/cleanup.go`**
- `CleanupResult` struct: `VMsDeleted int`, `ServicesDeleted int`, `NamespaceDeleted bool`, `Errors []error`
- `CleanupAll(ctx context.Context, c client.Client, namespace string, deleteNamespace bool) (*CleanupResult, error)`
  - List VMs by label `managed-by=virtwork` via `client.MatchingLabels`
  - Delete each VM individually — errors appended to `Errors` slice, not fatal
  - Delete managed services with same error-tolerance pattern
  - Optionally delete namespace (last step, after resources are drained)
  - Returns summary with accurate counts of successful deletions

### Tests (TDD)

**`internal/cleanup/cleanup_test.go`** (Ginkgo):
```go
Describe("CleanupAll", func() {
    It("should delete VMs by managed-by label", ...)
    It("should tolerate individual VM deletion errors", ...)
    It("should delete services by managed-by label", ...)
    It("should tolerate individual service deletion errors", ...)
    It("should not delete namespace by default", ...)
    It("should delete namespace when flagged", ...)
    It("should tolerate namespace deletion error", ...)
    It("should report accurate counts for successful deletions", ...)
    It("should handle empty namespace gracefully", ...)
    It("should use correct managed-by=virtwork label selector", ...)
})
```

### Verification

- `go test ./internal/cleanup/...` all pass
- Individual deletion failures don't abort
- Summary has accurate counts

### Commits

- `feat: add cleanup package with error-tolerant teardown`

---

## Phase 10: CLI and Entry Point

**Goal:** Build the Cobra command tree, orchestration flow, and wire up `cmd/virtwork/main.go`. This is the integration point for all packages.

### Files to Create/Modify

**`cmd/virtwork/main.go`**
- `rootCmd` with persistent flags: `--namespace`, `--kubeconfig`, `--config`, `--verbose`
- `runCmd` subcommand: `--workloads`, `--vm-count`, `--cpu-cores`, `--memory`, `--disk-size`, `--container-disk-image`, `--no-wait`, `--timeout`, `--dry-run`, `--ssh-user`, `--ssh-password`, `--ssh-key`, `--ssh-key-file`
- `cleanupCmd` subcommand: `--delete-namespace`
- `func main()` calls `rootCmd.Execute()`

**Run flow (in `runCmd.RunE`) — dry-run early-return pattern:**
1. Load config via Viper
2. Build workload instances from registry, passing SSH credentials and namespace via functional options
3. Build VM specs for all workloads (handles multi-VM workloads via `MultiVMWorkload` type assertion)
4. Check dry-run → if true: print specs as YAML, return
5. Connect to cluster via `cluster.Connect()`
6. `resources.EnsureNamespace()`
7. For each enabled workload: create Service if `RequiresService()` (before VMs for DNS), spawn VM creation goroutines via errgroup
8. `errgroup.Wait()` for all VM creates
9. If not `--no-wait`: `wait.WaitForAllVMsReady()` via errgroup
10. Print summary table

**Cleanup flow (in `cleanupCmd.RunE`):**
1. Load config via Viper
2. Connect to cluster via `cluster.Connect()`
3. `cleanup.CleanupAll()` — delete all labeled resources with error tolerance
4. Print cleanup summary

Each subcommand is a linear sequence — no nested if/else chains.

### Tests (TDD)

**`cmd/virtwork/main_test.go`** (Ginkgo):

Argument parsing tests:
```go
Describe("Run command flags", func() {
    It("should have default namespace", ...)
    It("should accept custom namespace", ...)
    It("should accept workloads CSV", ...)
    It("should accept vm-count", ...)
    It("should accept cpu-cores", ...)
    It("should accept memory", ...)
    It("should accept disk-size", ...)
    It("should accept dry-run flag", ...)
    It("should accept no-wait flag", ...)
    It("should accept verbose flag", ...)
    It("should accept timeout", ...)
    It("should accept config file", ...)
    It("should accept ssh-user flag", ...)
    It("should accept ssh-password flag", ...)
    It("should accept ssh-key flag (repeatable)", ...)
    It("should accept ssh-key-file flag (repeatable)", ...)
})
```

Orchestration tests (with fake client.Client):
```go
Describe("Run orchestration", func() {
    Context("dry-run mode", func() {
        It("should skip cluster connection", ...)
        It("should print specs to stdout", ...)
    })
    Context("normal mode", func() {
        It("should create namespace", ...)
        It("should create VMs for each workload", ...)
        It("should create service for network workload", ...)
        It("should wait for readiness", ...)
        It("should skip wait when --no-wait", ...)
    })
})

Describe("Cleanup command", func() {
    It("should delete managed resources", ...)
    It("should print summary", ...)
})
```

BDD-style scenarios (Ginkgo Describe/Context/It):
```go
Describe("CLI end-to-end scenarios", func() {
    Context("when running with --dry-run --workloads cpu", func() {
        It("should not attempt cluster connection", ...)
        It("should print VM specs to stdout", ...)
    })
    Context("when running with --cleanup", func() {
        It("should delete all managed VMs", ...)
        It("should print a cleanup summary", ...)
    })
    Context("when running with default arguments", func() {
        // Default run creates 6 VMs: cpu=1 + memory=1 + disk=1 + database=1 + network=2
        It("should create VMs for all workloads", ...)
        It("should begin readiness polling", ...)
    })
})
```

### Verification

- `go test ./...` all pass (full suite)
- `ginkgo -r` all pass
- `go run ./cmd/virtwork --help` shows all flags
- `go run ./cmd/virtwork run --dry-run` prints YAML without a cluster
- `go build -o virtwork ./cmd/virtwork` produces binary

### Commits

- `feat: add Cobra command tree with run and cleanup subcommands`
- `feat: add run orchestration with errgroup concurrency`
- `feat: wire up main.go entry point`
- `test: add BDD scenarios for CLI`

---

## Dependencies

### Runtime

| Dependency | Purpose |
|------------|---------|
| `github.com/spf13/cobra` | CLI command tree |
| `github.com/spf13/viper` | Config priority chain (flags > env > file > defaults) |
| `gopkg.in/yaml.v3` | Cloud-init YAML generation, config file parsing |
| `sigs.k8s.io/controller-runtime` | High-level K8s client with scheme-based typed operations |
| `k8s.io/api` | Core K8s API types (Namespace, Service) |
| `k8s.io/apimachinery` | K8s meta types, errors, labels |
| `k8s.io/client-go` | REST config, kubeconfig loading, retry utilities |
| `kubevirt.io/api` | KubeVirt VirtualMachine/VirtualMachineInstance types |
| `kubevirt.io/containerized-data-importer-api` | CDI DataVolume types |
| `golang.org/x/sync` | `errgroup` for structured concurrent operations |
| `github.com/mattn/go-sqlite3` | CGo SQLite3 driver for audit database |
| `github.com/google/uuid` | UUID generation for run ID labels |

### Development

| Dependency | Purpose |
|------------|---------|
| `github.com/onsi/ginkgo/v2` | BDD test framework (Describe/Context/It) |
| `github.com/onsi/gomega` | Expressive test matchers |

### Test Configuration

Build tags for test levels:

```go
//go:build integration
// +build integration

package integration_test
```

Run specific test levels:

```bash
# Unit tests only (default, no build tag)
go test ./...

# Integration tests (alongside source, requires cluster)
go test -tags integration ./internal/...

# E2E tests (separate directory, requires cluster + binary)
go test -tags e2e ./tests/e2e/...

# All tests
go test -tags "integration e2e" ./...

# Via Ginkgo
ginkgo -r
ginkgo -r --build-tags integration ./internal/
ginkgo -r --build-tags e2e ./tests/e2e/
```

---

## Phase 11: Integration and E2E Tests

**Goal:** Add integration tests alongside source code with `//go:build integration` tags and E2E acceptance tests in `tests/e2e/` with `//go:build e2e` tags.

### Test Architecture

| Category | Location | Build Tag | What it tests |
|----------|----------|-----------|---------------|
| Integration | `internal/*/_integration_test.go` | `integration` | Individual packages against a real KubeVirt cluster |
| E2E | `tests/e2e/*.go` | `e2e` | CLI binary as a black box (deploy, cleanup, dry-run) |
| Helpers | `internal/testutil/` | (none) | Shared utilities: namespace generation, cluster connect, binary execution |

### Files Created

**Shared helpers:**
- `internal/testutil/testutil.go` — `UniqueNamespace()`, `MustConnect()`, `CleanupNamespace()`, `ManagedLabels()`, `DefaultVMOpts()`, `EnsureTestNamespace()`, `WaitForVMRunning()`
- `internal/testutil/binary.go` — `BinaryPath()`, `RunVirtwork()` for E2E binary execution

**Integration tests (5 files):**
- `internal/cluster/cluster_integration_test.go` — Real cluster connectivity and scheme validation
- `internal/resources/resources_integration_test.go` — Real namespace/service/secret CRUD and idempotency
- `internal/vm/vm_integration_test.go` — Real VirtualMachine create/delete/list
- `internal/wait/wait_integration_test.go` — Real VMI readiness polling (Label("slow"))
- `internal/cleanup/cleanup_integration_test.go` — Real label-based cleanup with error tolerance

**E2E tests (5 files):**
- `tests/e2e/e2e_suite_test.go` — Ginkgo bootstrap, binary build in `BeforeSuite`
- `tests/e2e/dryrun_test.go` — `virtwork run --dry-run` scenarios (no cluster needed)
- `tests/e2e/run_test.go` — `virtwork run` with real cluster deployment
- `tests/e2e/cleanup_test.go` — `virtwork cleanup` after deployment
- `tests/e2e/fullcycle_test.go` — Deploy → verify → cleanup cycles

### Key Design Decisions

- Integration tests share existing `*_suite_test.go` runners (no build tag on suite files)
- Each test creates a unique namespace via `UniqueNamespace()` with `DeferCleanup` teardown
- Slow tests (VM boot ~60-120s) use Ginkgo `Label("slow")` for filtering
- E2E tests invoke the binary via `os/exec`, verifying stdout/stderr/exit code
- `internal/testutil/` is importable by both integration and E2E tests (same Go module)

### Verification

```bash
# Unit tests unchanged
go test ./...

# Integration tests (requires cluster)
go test -tags integration ./internal/...

# E2E tests (requires cluster + binary)
go test -tags e2e ./tests/e2e/...

# All tests
go test -tags "integration e2e" ./...
```

### Commits

- `test: add shared test helpers for integration and E2E tests`
- `test: add integration tests for cluster, resources, vm, wait, cleanup`
- `test: add E2E test suite with dry-run, run, cleanup, and full-cycle scenarios`
- `docs: add integration and E2E test documentation`

---

## Phase 12: Audit System

**Goal:** Add SQLite-based audit tracking for all executions. Every `run` and `cleanup` gets a database record with timestamps, configuration, and outcome details.

### Files Created

**`internal/audit/schema.go`**
- DDL for 5 tables: `audit_log`, `workload_details`, `vm_details`, `resource_details`, `events`
- Indexes on foreign keys and common query columns
- Foreign key constraints between all detail tables and `audit_log`

**`internal/audit/records.go`**
- `WorkloadRecord`, `VMRecord`, `ResourceRecord`, `EventRecord` structs
- Data transfer objects for audit operations

**`internal/audit/audit.go`**
- `Auditor` interface with 13 methods covering full execution lifecycle
- `SQLiteAuditor` implementation with WAL mode and foreign key enforcement
- `NoOpAuditor` for when audit is disabled
- `NewAuditor(dbPath)` constructor, `NewNoOpAuditor()` constructor

**`internal/audit/audit_suite_test.go`** — Ginkgo bootstrap
**`internal/audit/audit_test.go`** — 14 tests covering:
- Schema creation (tables + indexes)
- Full execution lifecycle (start → workloads → VMs → events → complete)
- Failure with error summary
- Cleanup linking (single and multi-run)
- Cleanup counts recording
- VM and resource deletion tracking
- Events with FK references
- Concurrent writes (20 goroutines)
- SSH auth boolean tracking
- NoOpAuditor does nothing without error

### Files Modified

**`internal/constants/constants.go`**
- Added `LabelRunID = "virtwork/run-id"` for run-to-cleanup linking
- Added `DefaultAuditDBPath = "virtwork.db"`

**`internal/config/config.go`**
- Added `AuditEnabled` (bool, default: true) and `AuditDBPath` (string, default: `virtwork.db`)
- Viper defaults and LoadConfig wiring

**`internal/cleanup/cleanup.go`**
- Added `runID string` parameter to `CleanupAll` for targeted cleanup
- Added `RunIDs []string` field to `CleanupResult`
- Added `collectRunID()` helper to extract `virtwork/run-id` from resource labels
- When `runID` is provided, adds it to label selector; otherwise collects all unique run IDs

**`cmd/virtwork/main.go`**
- Added `--audit`/`--no-audit`/`--audit-db` persistent flags
- Added `--run-id` flag on cleanup command
- Run flow: generates UUID, applies `virtwork/run-id` label, records audit events at each step
- Cleanup flow: uses `--run-id` for targeted deletion, links collected run IDs to audit record

**`.gitignore`** — Added `virtwork.db`

### Key Design Decisions

- **`Auditor` interface + `NoOpAuditor`:** Avoids nil checks throughout codebase when audit is disabled
- **WAL journal mode:** Allows concurrent reads during writes
- **Shared cache for `:memory:` tests:** `file::memory:?mode=memory&cache=shared` ensures all connections in the pool see the same database
- **JSON array for `linked_run_ids`:** Supports cleanup-all (multiple runs) while remaining PostgreSQL JSONB compatible
- **No SSH credentials stored:** Security by design — only a boolean `ssh_auth_configured`

### Verification

- `go test ./internal/audit/...` — 14 tests pass
- `go test ./...` — full suite passes (no regressions)
- `go vet ./...` — clean

### Commits

- `feat: add run-id label and audit DB path constants`
- `feat: add audit config fields (AuditEnabled, AuditDBPath)`
- `feat: add SQLite audit system with 5-table schema`
- `feat: add run-id label and targeted cleanup support`
- `feat: integrate audit tracking into CLI orchestration`
- `chore: add virtwork.db to .gitignore`
- `test: verify all tests pass after audit integration`

---

## Lessons from Python Implementation

The following patterns and lessons were discovered during the Python (`dynavirt`) implementation and should be applied to the Go build:

### Testing Patterns

1. **YAML assertion via parsing** — Never assert on raw YAML strings. Always `yaml.Unmarshal` the output and assert on the parsed structure. YAML serializers may reorder keys, fold long lines, or vary whitespace between implementations.

2. **Ripple effect on new workloads** — Adding a new workload triggers test failures in: registry tests (entry count, name list), orchestration BDD tests (total VM count assertions). These are mechanical fixes but must be expected.

3. **Systemd unit separation** — For workloads with both initialization and runtime phases (like database), use `ExecStartPre` for setup and `ExecStart` for the main loop. This is cleaner than a single script with conditional init logic.

4. **Fio profile separation** — Write fio job profiles as separate `write_files` entries rather than embedding them in the systemd unit. Keeps the systemd service definition readable and the profiles independently verifiable.

### Architectural Patterns

5. **Separate subcommands for distinct operations** — `run` and `cleanup` are separate Cobra subcommands, each with its own `RunE` function and flag set. Within `run`, dry-run is an early-return check. Cleanup has fundamentally different error semantics (continue-on-error) and flags (`--delete-namespace`), justifying a separate command rather than a mode flag.

6. **Error-tolerant vs. fail-fast** — Create-time operations should fail fast (return error immediately). Cleanup operations should accumulate errors and continue. These are fundamentally different error semantics — don't try to unify them by adding flags to shared functions.

7. **Cross-cutting concern injection via base struct** — When a concern (SSH credentials) must appear in every workload's output, add a helper method to `BaseWorkload` that wraps the underlying pure function. Workloads change one call site (the method name) and the base struct handles the wiring.

8. **Service-before-VM ordering** — For multi-VM workloads with DNS dependencies, the K8s Service must be created before the client VM. The client's systemd unit uses `Restart=always` to retry until DNS resolves, but the Service must exist for DNS to work at all.

---

## Risk Mitigations

| Risk | Mitigation |
|------|------------|
| KubeVirt API type versioning | Pin `kubevirt.io/api` version in go.mod; API coordinates isolated in constants package |
| controller-runtime fake client limitations | Use `fake.NewClientBuilder().WithScheme(scheme).WithObjects(...).Build()` for unit tests |
| Cloud-init YAML correctness | Validate all output against `yaml.Unmarshal` in tests |
| Network workload DNS timing | systemd `Restart=always` on client handles server not yet ready |
| Large dependency tree (client-go) | Go modules handle this; build caching keeps iteration fast |
| Race conditions in concurrent tests | Use `ginkgo -race` and `go test -race` in CI |
| HAProxy cold-pool TLS failures | Load balancers may close idle connections silently. controller-runtime handles TLS renegotiation, but be aware of transient 503s on first API call after idle period. Log and retry. |
| Default run creates too many VMs as workloads grow | Each new workload adds VMs to the default set (currently 6: cpu=1 + memory=1 + disk=1 + database=1 + network=2). Consider a curated default subset if workload count grows beyond 6-7. |
