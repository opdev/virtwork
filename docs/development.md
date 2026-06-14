# Development Guide

## Environment Setup

### Prerequisites

- Go 1.26+
- [Ginkgo CLI](https://onsi.github.io/ginkgo/#installing-ginkgo) (for BDD test runner)

### Install Ginkgo CLI

```bash
go install github.com/onsi/ginkgo/v2/ginkgo@latest
```

### Install Dependencies

```bash
go mod download
```

## Building

```bash
# Build the binary
go build -o virtwork ./cmd/virtwork

# Run without building
go run ./cmd/virtwork --help
go run ./cmd/virtwork run --dry-run
go run ./cmd/virtwork run --dry-run --ssh-user virtwork --ssh-key-file ~/.ssh/id_ed25519.pub
```

## Running Tests

```bash
# Full unit test suite
go test ./...

# With race detector
go test -race ./...

# Specific package
go test ./internal/vm/...

# With verbose output
go test -v ./...

# With coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Using Ginkgo

```bash
# Run all tests recursively
ginkgo -r

# Run specific package
ginkgo ./internal/vm/

# Verbose with labels
ginkgo -r -v

# With race detector
ginkgo -r -race

# Run in parallel (multiple Ginkgo nodes)
ginkgo -r -p

# Focus on specific tests (use FDescribe/FIt in code)
ginkgo -r --focus "BuildVMSpec"
```

### Test Organization

| Location | Build Tag | Cluster Required | Coverage | Description |
|----------|-----------|------------------|----------|-------------|
| `internal/*/_test.go` | (none) | No | ~60-80% | Unit tests alongside source, all K8s calls use fake client |
| `internal/*/_integration_test.go` | `integration` | Yes | ~40-60% | Integration tests alongside source, real cluster interactions |
| `tests/e2e/` | `e2e` | Yes | Black-box | E2E/acceptance tests, CLI binary testing against real cluster |
| `internal/testutil/` | mixed | Conditional | 58.2% | Shared test helpers: unit tests (no cluster) + integration tests (requires cluster) |

Unit tests use controller-runtime's fake client:

```go
fake.NewClientBuilder().
    WithScheme(cluster.NewScheme()).
    WithObjects(existingResources...).
    Build()
```

Integration tests use a real cluster connection:

```go
c = testutil.MustConnect("")
namespace = testutil.UniqueNamespace("integ-prefix")
DeferCleanup(func() { testutil.CleanupNamespace(ctx, c, namespace) })
```

E2E tests invoke the built binary:

```go
stdout, stderr, exitCode, err := testutil.RunVirtwork("run", "--dry-run", "--workloads", "cpu")
Expect(exitCode).To(Equal(0))
```

#### testutil Package

The `internal/testutil/` package provides shared test helpers used by integration and E2E tests. As of the test coverage improvements, it now has comprehensive unit and integration test coverage.

**Core Functions:**

- `MustConnect(kubeconfigPath string) client.Client` - Connects to cluster, panics on failure (suitable for BeforeEach setup)
- `UniqueNamespace(prefix string) string` - Generates collision-proof names like `virtwork-test-<prefix>-<random>`
- `EnsureTestNamespace(ctx, c, namespace) error` - Creates namespace with managed-by labels
- `CleanupNamespace(ctx, c, namespace)` - Deletes all managed resources + namespace (error-tolerant for AfterEach)
- `ManagedLabels() map[string]string` - Returns standard virtwork labels for resource tracking
- `DefaultVMOpts(name, namespace) VMSpecOpts` - Returns minimal VM spec (1 CPU, 512Mi, Fedora disk, basic cloud-init)
- `WaitForVMRunning(ctx, c, name, namespace, timeout) error` - Polls until VMI phase is Running
- `BinaryPath() (string, error)` - Returns path to built virtwork binary (checks `VIRTWORK_BINARY` env var, builds on first call)
- `RunVirtwork(args...) (stdout, stderr string, exitCode int, err error)` - Executes virtwork binary for E2E tests

**Example Usage Pattern:**

```go
var _ = Describe("MyFeature", func() {
    var ctx context.Context
    var c client.Client
    var namespace string

    BeforeEach(func() {
        ctx = context.Background()
        c = testutil.MustConnect("")
        namespace = testutil.UniqueNamespace("my-feature")
        Expect(testutil.EnsureTestNamespace(ctx, c, namespace)).To(Succeed())
    })

    AfterEach(func() {
        testutil.CleanupNamespace(ctx, c, namespace)
    })

    It("should deploy a VM", func() {
        opts := testutil.DefaultVMOpts("test-vm", namespace)
        vmObj := vm.BuildVMSpec(opts)
        Expect(vm.CreateVM(ctx, c, vmObj)).To(Succeed())
        
        // Wait for VM to boot
        Expect(testutil.WaitForVMRunning(ctx, c, "test-vm", namespace, 5*time.Minute)).To(Succeed())
    })
})
```

**Testing the testutil package:**

```bash
# Unit tests (no cluster required)
go test ./internal/testutil -v

# Integration tests (cluster required)
go test -tags integration ./internal/testutil -v

# Coverage report
go test ./internal/testutil -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Cluster Prerequisites for Integration and E2E Tests

Integration and E2E tests require a live OpenShift cluster with specific operators and configurations.

#### Minimum Requirements

- **OpenShift**: 4.12+ (tested on 4.14, 4.15, 4.16)
- **OpenShift Virtualization (CNV)**: 4.12+ 
  - KubeVirt API compatibility: v1.7.0+
  - CDI (Containerized Data Importer) API: v1.64.0+
- **Storage**: Default StorageClass with ReadWriteOnce support
- **Networking**: Pod network with masquerade networking support
- **Permissions**: Cluster admin or namespace admin with permissions to:
  - Create/delete namespaces
  - Create/delete VirtualMachines, VirtualMachineInstances
  - Create/delete Services, Secrets, DataVolumes
  - List namespaces cluster-wide (for cleanup tests)

#### Kubeconfig Setup

Tests use the following kubeconfig resolution order:

1. `KUBECONFIG` environment variable (recommended for CI)
2. `~/.kube/config` (default for local development)
3. In-cluster config (when running inside a pod)

```bash
# Set kubeconfig for tests
export KUBECONFIG=/path/to/your/kubeconfig

# Verify cluster access
oc whoami
oc get nodes
```

#### Verifying Cluster Readiness

Before running integration or E2E tests, verify operators are installed:

```bash
# Check KubeVirt/CNV operator
oc get csv -n openshift-cnv | grep kubevirt

# Check CDI operator
oc get csv -n openshift-cnv | grep containerized-data-importer

# Verify default StorageClass exists
oc get sc | grep default

# Test namespace creation permission
oc create namespace virtwork-test-check
oc delete namespace virtwork-test-check
```

#### Resource Consumption and Runtime Estimates

| Test Type | Namespace Count | VM Count | Disk Usage | Runtime | Cluster Load |
|-----------|----------------|----------|------------|---------|--------------|
| Unit tests | 0 | 0 | ~50MB (built binaries) | ~5-10s | None (fake client) |
| Integration tests | ~15-20 | 0-5 | ~100MB | ~2-5min | Low (namespace/resource CRUD) |
| E2E tests | ~5-10 | 2-10 | ~500MB-2GB (containerDisks) | ~10-30min | Medium (VM boots, workload execution) |
| Full suite | ~25-30 | 10-15 | ~2-3GB | ~15-40min | Medium |

**Notes:**
- Integration tests create namespaces and minimal resources but rarely boot VMs
- E2E tests deploy full workloads with VM boot (slower, higher resource usage)
- Tests use unique namespace names (`virtwork-test-*-<random>`) to avoid collisions
- Cleanup is automatic via `DeferCleanup()` but KubeVirt finalizers may delay namespace deletion

#### Common Test Failures and Solutions

**"connection refused" errors:**
```bash
# Check kubeconfig is set and cluster is accessible
echo $KUBECONFIG
oc cluster-info

# Verify API server connectivity
oc get nodes
```

**"no matches for kind VirtualMachine":**
```bash
# KubeVirt CRDs not installed
oc get crd | grep kubevirt

# Install OpenShift Virtualization operator via OperatorHub
oc get csv -n openshift-cnv
```

**"timeout waiting for VM to be running":**
- Check cluster has sufficient resources (CPU, memory, storage)
- Verify default StorageClass is available and bound
- Check VM events: `oc get events -n <namespace> --sort-by='.lastTimestamp'`
- Increase timeout in test if cluster is resource-constrained

**"namespace stuck in Terminating":**
- KubeVirt finalizers are cleaning up VM resources
- Wait up to 60 seconds for automatic cleanup
- Force delete if stuck: `oc delete namespace <name> --grace-period=0 --force`

**Tests fail with "AlreadyExists" errors:**
- Previous test run cleanup incomplete
- Clean up manually: `virtwork cleanup` or `oc delete namespace virtwork-test-<prefix>-*`

### Running Integration Tests

Integration tests live alongside source code with `//go:build integration` build tags. They are excluded from `go test ./...` (no tag).

**Prerequisites:**
- `KUBECONFIG` set or `~/.kube/config` available
- Cluster with KubeVirt/CNV and CDI operators installed
- Permissions to create/delete namespaces, VMs, Services, Secrets

```bash
# Run all integration tests
go test -tags integration ./internal/...

# Run integration tests for a specific package
go test -tags integration ./internal/vm/...

# Via Ginkgo
ginkgo -r --build-tags integration ./internal/

# Skip slow tests (VM boot required)
ginkgo -r --build-tags integration --label-filter='!slow' ./internal/
```

### Running E2E Tests

E2E tests live in `tests/e2e/` and exercise the CLI binary as a black box. The binary is built automatically in `BeforeSuite`, or you can provide a pre-built binary via `VIRTWORK_BINARY`.

**Prerequisites:**
- All integration test prerequisites above
- Go toolchain (for binary build) or `VIRTWORK_BINARY` env var

```bash
# Run all E2E tests
go test -tags e2e ./tests/e2e/...

# Via Ginkgo
ginkgo -r --build-tags e2e ./tests/e2e/

# Skip slow tests (cluster deployment)
ginkgo -r --build-tags e2e --label-filter='!slow' ./tests/e2e/

# Use a pre-built binary
VIRTWORK_BINARY=./virtwork go test -tags e2e ./tests/e2e/...

# Run everything (unit + integration + e2e)
go test -tags "integration e2e" ./...
```

## Makefile Targets

A `Makefile` provides convenient shortcuts for common development tasks:

```bash
# Show all available targets
make help

# Run unit tests (excludes integration/e2e)
make test

# Run integration tests (requires cluster)
make test-integration

# Run e2e tests (requires cluster)
make test-e2e

# Run all tests (unit + integration + e2e)
make test-all

# Run go vet
make vet

# Run golangci-lint
make lint

# Format code
make fmt

# Build the binary
make build

# Run full CI validation locally (vet + test + build)
make ci

# Run all verification checks (fmt + vet + lint + test)
make verify

# Clean build artifacts
make clean

# Install development tools (golangci-lint)
make install-tools

# Build container image locally (uses podman by default)
make container-build

# Build with docker instead of podman
CONTAINER_RUNTIME=docker make container-build
```

## CI/CD Pipeline

The project uses GitHub Actions for automated validation on push and pull requests:

### Workflows

- **Build & Test** (`.github/workflows/build.yml`) — Runs on every push/PR to main
  - Go vet checks
  - Unit tests with race detector
  - Binary build verification
  - Only runs unit tests (integration/e2e require live cluster)

- **Linting** (`.github/workflows/lint.yml`) — Runs golangci-lint on every push/PR
  - Code quality checks
  - Static analysis
  - Best practices enforcement

- **Container Image** (`.github/workflows/image.yml`) — Builds and pushes container images
  - Triggered on main branch pushes and tags

- **Release** (`.github/workflows/release.yml`) — Automated releases via GoReleaser
  - Triggered on version tags (e.g., `v1.0.0`)
  - Builds multi-platform binaries
  - Generates release notes

### Running CI Checks Locally

Before pushing code, run the same checks that CI will execute:

```bash
# Quick validation (vet + test + build)
make ci

# Full verification (includes fmt and lint)
make verify

# Or run individual checks
make vet
make test
make lint
```

## Project Layout

```
cmd/virtwork/       # Entry point (Cobra root + subcommands)
internal/           # Application packages (not importable externally)
  constants/        # API coordinates, labels, defaults
  config/           # Config struct, Viper priority chain
  cluster/          # controller-runtime client init + scheme
  cloudinit/        # Cloud-config YAML builder
  logging/          # Structured slog logger (verbose -> DEBUG)
  vm/               # VM spec construction + typed CRUD + retry
  resources/        # Namespace + Service + Secret helpers
  wait/             # VMI readiness polling (errgroup)
  cleanup/          # Label-based teardown (VMs, Services, Secrets)
  audit/            # SQLite audit tracking (Auditor interface, schema, records)
  orchestrator/     # Run + Cleanup orchestrators (errgroup concurrency, catalog loading)
  workloads/        # Workload + MultiVMWorkload interfaces, built-in + generic implementations, registry, catalog
  testutil/         # Shared test helpers for integration and E2E tests
tests/              # Tests requiring external infrastructure
  e2e/              # E2E acceptance tests (//go:build e2e)
build/
  golden-image/     # Optional Fedora container disk with pre-installed tools
deploy/             # Kustomize manifests for OpenShift deployment
docs/               # Documentation
  README.md         # Documentation index
  architecture.md   # Layered architecture and mermaid diagrams
  development.md    # This file
  configuration.md  # Complete configuration reference
  deployment.md     # OpenShift deployment deep-dive
  audit-schema.md   # SQLite audit schema reference
  chaos-workloads.md  # Chaos engineering workload guide
  guide/            # Hands-on guides (overview, deploying, adding workloads)
```

## Architecture Layers

The codebase follows a strict layered architecture where each layer depends only on layers below it. See [architecture.md](architecture.md) for full diagrams.

| Layer | Packages | Goroutines | Purpose |
|-------|----------|------------|---------|
| 0 | `constants` | No | Pure values — API coordinates, labels, defaults |
| 1 | `config`, `cloudinit`, `cluster`, `logging`, `audit` | No | Configuration, cloud-init YAML, K8s client init, structured logging, audit tracking |
| 2 | `vm`, `resources`, `wait` | Yes | K8s CRUD operations with retry, readiness polling |
| 3 | `workloads` | No | Pure data producers — cloud-init specs, resource structs |
| 4 | `cmd/virtwork`, `orchestrator`, `cleanup` | Yes | Dependency wiring, orchestration, teardown |

### Concurrency Pattern

Go's native concurrency is used throughout. The `internal/orchestrator` package drives parallel VM creation and readiness polling via `errgroup.Group` for structured error handling, with `context.Context` for timeouts and cancellation.

```go
g, ctx := errgroup.WithContext(ctx)
for _, vmName := range vmNames {
    name := vmName
    g.Go(func() error {
        return vm.CreateVM(ctx, c, spec)
    })
}
if err := g.Wait(); err != nil {
    return err
}
```

## Adding a New Workload

### 1. Create the Workload Struct

Create `internal/workloads/<name>.go`:

```go
package workloads

type MyWorkload struct {
    BaseWorkload
}

func NewMyWorkload(cfg config.WorkloadConfig, sshUser, sshPassword string, sshKeys []string) *MyWorkload {
    return &MyWorkload{BaseWorkload: BaseWorkload{
        Config:            cfg,
        SSHUser:           sshUser,
        SSHPassword:       sshPassword,
        SSHAuthorizedKeys: sshKeys,
    }}
}

func (w *MyWorkload) Name() string {
    return "my-workload"
}

func (w *MyWorkload) CloudInitUserdata() (string, error) {
    // Use BaseWorkload's helper to inject SSH credentials automatically
    return w.BuildCloudConfig(cloudinit.CloudConfigOpts{
        Packages: []string{"my-package"},
        // ...
    })
}
```

### 2. Override Optional Methods

`BaseWorkload` provides defaults via embedding. Override only what you need:

| Method | Default | Override When |
|--------|---------|---------------|
| `ExtraVolumes()` | `nil` | VM needs additional volume mounts |
| `ExtraDisks()` | `nil` | VM needs additional disk definitions |
| `DataVolumeTemplates()` | `nil, nil` | Workload needs persistent storage (returns `[]DataVolumeTemplateSpec, error`) |
| `RequiresService()` | `false` | VMs need a K8s Service for communication |
| `ServiceSpec()` | `nil` | Define the Service when `RequiresService()` is true |
| `VMCount()` | `1` | Workload needs multiple VMs (e.g., server/client) |

For multi-VM workloads with per-role userdata, implement the `MultiVMWorkload` interface:

```go
func (w *MyWorkload) UserdataForRole(role string, namespace string) (string, error) {
    switch role {
    case "server":
        return w.serverUserdata()
    case "client":
        return w.clientUserdata(namespace)
    default:
        return "", fmt.Errorf("unknown role: %s", role)
    }
}
```

### 3. Register the Workload

Add a `RegistryEntry` to `internal/workloads/registry.go`. Each entry pairs a `WorkloadFactory` with a `ParamSchema` so the registry can validate user-supplied params at deploy time. `DefaultRegistry()` currently has nine entries; add yours alongside:

```go
func DefaultRegistry() Registry {
    return Registry{
        "cpu": {
            Factory: func(cfg config.WorkloadConfig, opts *RegistryOpts) Workload {
                return NewCPUWorkload(cfg, opts.SSHUser, opts.SSHPassword, opts.SSHAuthorizedKeys)
            },
            ParamSchema: CPUParamSchema,
        },
        // ... other built-in entries ...
        "my-workload": {
            Factory: func(cfg config.WorkloadConfig, opts *RegistryOpts) Workload {
                return NewMyWorkload(cfg, opts.SSHUser, opts.SSHPassword, opts.SSHAuthorizedKeys)
            },
            ParamSchema: MyParamSchema,
        },
    }
}
```

**Important:** When adding a new workload, expect these ripple effects:
- Registry tests will fail (entry count, name list assertions)
- Orchestration BDD tests will fail (total VM count assertions)
- Update both before considering the feature complete

### Going Further: Multi-VM Workloads

If your workload needs more than one role of VM (e.g., a server and a client), implement the `MultiVMWorkload` interface in addition to `Workload`:

```go
// In internal/workloads/workload.go
type MultiVMWorkload interface {
    Workload
    RoleDistribution() []RoleSpec
    UserdataForRole(role string, namespace string) (string, error)
}

type RoleSpec struct {
    Role    string
    VMCount int
}
```

Pattern:

1. Embed `BaseWorkload` and store any per-workload state (e.g., a `Namespace` field for in-cluster DNS).
2. Implement `RoleDistribution()` to return per-role VM counts (e.g., `[]RoleSpec{{Role: "server", VMCount: 1}, {Role: "client", VMCount: 1}}`).
3. Override `VMCount()` to sum the `RoleSpec.VMCount` values from `RoleDistribution()`.
4. Implement `UserdataForRole(role, namespace)` to return role-specific cloud-init. The default `CloudInitUserdata()` typically delegates to `UserdataForRole("server", namespace)`.
5. Set `RequiresService()` to `true` and provide a `ServiceSpec()` selecting server VMs by the `virtwork/role: server` label that the orchestrator applies automatically.
6. Clients reach servers via the in-cluster DNS name `<service-name>.<namespace>.svc.cluster.local` — never poll for pod IPs.

The canonical references are `internal/workloads/network.go` (simplest — one port, iperf3) and `internal/workloads/tps.go` (multi-port Service). All workloads support configurable `Params` via a typed param schema (see [Configurable Params](#going-further-configurable-params) below).

### Going Further: Storage-Backed Workloads

If your workload needs persistent storage inside the VM:

1. Override `DataVolumeTemplates()` to return a CDI `DataVolume` for each volume needed. Use `vm.BuildDataVolumeTemplate(name, size)` from `internal/vm`.
2. Override `ExtraVolumes()` and `ExtraDisks()` to wire the DataVolume into the VM. **Always set the `Serial` field on the `Disk`** — the in-VM script discovers the device through `/dev/disk/by-id/virtio-<serial>`, which is deterministic across reboots (unlike `/dev/vdX`, which is not).
3. In your cloud-init userdata, write the shared `diskSetupScript(serial, mountPoint)` helper (from `internal/workloads/workload.go`) as the first script. It waits for the symlink, formats with XFS if empty, mounts, and writes `/etc/fstab` for persistence across reboots.
4. The `NamespaceDataVolumes` helper in `internal/orchestrator/types.go` automatically suffixes DV template names with the VM name to avoid collisions across multiple VMs of the same workload. Your template name should be the un-suffixed base (e.g., `virtwork-chaos-disk-data`).

Reference workloads: `disk.go` (single fio mount), `database.go` (PostgreSQL data dir), `chaos_disk.go` (fill/release loop). All three follow the same pattern.

### Going Further: Configurable Params

All workloads expose tunable knobs through `WorkloadConfig.Params map[string]string`. Users set these in YAML config under `workloads.<name>.params`. Each workload declares a typed **param schema** — a slice of `ParamDef` entries that define the key, type, default, and description for every supported param:

```go
var CPUParamSchema = ParamSchema{
    {Key: "cpu-load-percent", Type: ParamInt, Default: "100", Desc: "Target CPU load percentage for stress-ng (--cpu-load)"},
    {Key: "cpu-method", Type: ParamString, Default: "all", Desc: "CPU stressor method for stress-ng (--cpu-method)"},
}
```

Set the schema in your constructor on the embedded `BaseWorkload`:

```go
func NewMyWorkload(cfg config.WorkloadConfig, sshUser, sshPassword string, sshKeys []string) *MyWorkload {
    return &MyWorkload{
        BaseWorkload: BaseWorkload{
            Config:      cfg,
            ParamSchema: MyParamSchema,
            SSHUser:     sshUser, SSHPassword: sshPassword, SSHAuthorizedKeys: sshKeys,
        },
    }
}
```

Then call `w.GetParam("key")` to retrieve the value — it returns the user's override if set, otherwise the schema default:

```go
func (w *MyWorkload) CloudInitUserdata() (string, error) {
    unit := fmt.Sprintf(mySystemdUnitTemplate, w.GetParam("concurrency"), w.GetParam("duration"))
    return w.BuildCloudConfig(CloudConfigOpts{
        WriteFiles: []WriteFile{{
            Path: "/etc/systemd/system/virtwork-my-workload.service",
            Content: unit,
        }},
    })
}
```

`GetParam` panics on unknown keys — this is intentional; it catches typos in workload code at test time. The orchestrator calls `registry.ValidateParams()` before constructing workloads, rejecting unknown keys (with "did you mean?" suggestions) and type-mismatched values at deploy time.

Five param types are available: `ParamString`, `ParamInt`, `ParamBool`, `ParamList` (semicolon-separated), and `ParamDict` (semicolon-separated `key=value` pairs). Register your workload in `DefaultRegistry()` as a `RegistryEntry` pairing the factory with the schema so validation applies automatically. Catalog workloads declare the same param schema in `workload.yaml` and use `{{key}}` placeholders in their service files instead of `GetParam()` calls — see [Adding a Catalog Workload](#adding-a-catalog-workload) for details.

Every workload should have a `Context("param wiring")` test block with three cases:

1. **Nil params** — `Params` field omitted, output contains default values
2. **Full override** — all param keys set, output reflects custom values
3. **Partial override** — some keys set, unset keys fall back to defaults

See `internal/workloads/cpu_test.go` for the simplest example, or `disk_test.go` for a workload with multiple output files to verify. Document new param keys in `docs/configuration.md` — both in the YAML example and the params table.

### Going Further: Structured Logging

The `internal/logging` package provides a shared `*slog.Logger` returned by `NewLogger(w io.Writer, verbose bool)`. Use it instead of `fmt.Fprintf` or `log.Printf` in any code path under `cmd/` or in packages that perform I/O (`internal/wait` is the current example):

```go
logger := logging.NewLogger(cmd.OutOrStdout(), verbose)
logger.Info("vm created",
    slog.String("vm_name", name),
    slog.String("namespace", ns),
    slog.String("workload", component))
```

The output is JSON. `--verbose` flips the level from `INFO` to `DEBUG`. Workload constructors and the pure data layer (`internal/workloads`, `internal/cloudinit`) should not log — they remain pure data producers.

## Adding a Catalog Workload

The catalog system lets users and operators add new workloads **without modifying Go code**. A catalog workload is a directory containing one or more systemd `.service` files and an optional `workload.yaml` manifest. At deploy time the orchestrator loads catalog entries, registers them alongside the built-in workloads, and creates VMs with the same cloud-init pipeline.

Use the catalog when:
- You want to deploy a custom workload without rebuilding the binary
- You want to iterate on workload definitions without a Go development environment
- You want to share workload definitions as portable directories

Catalog entries also support declarative [storage](#storage-backed-catalog-entries) (DataVolumes, extra disks) and [K8s Services](#service-backed-catalog-entries) via the `workload.yaml` manifest — no Go code required for these either.

### Catalog Directory Layout

```
~/.virtwork/catalog/          # default catalog-dir (override with --catalog-dir)
├── my-stress/                # single-role entry
│   ├── workload.yaml         # optional manifest (packages, params)
│   └── workload.service      # systemd unit file
└── my-benchmark/             # multi-role entry
    ├── workload.yaml         # required for multi-role (declares roles)
    ├── server.service        # one service file per role
    └── client.service
```

Each subdirectory is a catalog entry. The directory name becomes the workload name.

**Rules:**
- At least one `*.service` file is required
- Single-role entries: all `.service` files are written to every VM
- Multi-role entries: each declared role must have a matching `{role}.service` file
- `workload.yaml` is optional for single-role entries, required for multi-role entries

### workload.yaml Schema

```yaml
description: "Human-readable description of the workload"

packages:
  - stress-ng          # system packages to install via cloud-init

params:
  - key: cpu-load      # parameter name (used as {{cpu-load}} in service files)
    type: int           # string | int | bool | list | dict
    default: "50"       # default value (always a string)
    desc: "CPU load percentage for stress-ng"

roles:                  # omit for single-role entries
  - name: server
    vm-count: 1         # 0 means "use the global --vm-count flag"
  - name: client
    vm-count: 2

storage:                # optional — persistent volumes attached to VMs
  - name: data          # DataVolume name (suffixed with VM name automatically)
    size: 10Gi          # volume size
    serial: vw-data     # virtio serial (device discovered via /dev/disk/by-id/virtio-<serial>)
    mount: /mnt/data    # mount point inside the VM

service:                # optional — K8s Service for inter-VM communication
  ports:
    - name: iperf       # port name
      port: 5201        # port number
      protocol: TCP     # TCP or UDP
  selector-role: server # role whose VMs the Service selects (multi-role entries)
```

All fields are optional for single-role entries. When `roles:` is present the entry becomes multi-role, which requires a manifest and one `{role}.service` file per declared role.

### Single-Role Entry

A single-role catalog entry produces a `GenericWorkload` — one VM type running all the declared service files.

**Example — a custom stress-ng workload:**

```
~/.virtwork/catalog/my-stress/
├── workload.yaml
└── workload.service
```

`workload.yaml`:
```yaml
description: "Custom CPU stress test"
packages:
  - stress-ng
params:
  - key: cpu-load
    type: int
    default: "50"
    desc: "CPU load percentage"
  - key: method
    type: string
    default: "all"
    desc: "stress-ng CPU method"
```

`workload.service`:
```ini
[Unit]
Description=Virtwork custom stress
After=network-online.target

[Service]
Type=simple
ExecStart=/usr/bin/stress-ng --cpu 0 --cpu-load {{cpu-load}} --cpu-method {{method}} --timeout 0
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

**Deploy it:**
```bash
virtwork run --from-catalog my-stress --dry-run
virtwork run --from-catalog my-stress --params my-stress.cpu-load=75
```

The generated cloud-init will install `stress-ng`, write the service file to `/etc/systemd/system/workload.service`, substitute `{{cpu-load}}` and `{{method}}` with param values, and enable the service.

### Multi-Role Entry

A multi-role catalog entry produces a `GenericMultiWorkload` — multiple VM roles with per-role service files. This is the catalog equivalent of implementing the `MultiVMWorkload` interface.

**Example — an iperf3 server/client benchmark:**

```
~/.virtwork/catalog/my-benchmark/
├── workload.yaml
├── server.service
└── client.service
```

`workload.yaml`:
```yaml
description: "iperf3 server/client benchmark"
packages:
  - iperf3
params:
  - key: duration
    type: int
    default: "60"
    desc: "Test duration in seconds"
roles:
  - name: server
    vm-count: 1
  - name: client
    vm-count: 2
```

`server.service`:
```ini
[Unit]
Description=iperf3 server
After=network-online.target

[Service]
Type=simple
ExecStart=/usr/bin/iperf3 -s
Restart=always

[Install]
WantedBy=multi-user.target
```

`client.service`:
```ini
[Unit]
Description=iperf3 client
After=network-online.target

[Service]
Type=simple
ExecStart=/usr/bin/iperf3 -c server -t {{duration}} --forceflush
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

**Deploy it:**
```bash
virtwork run --from-catalog my-benchmark --dry-run
```

This creates 3 VMs total (1 server + 2 clients). Each role gets its own cloud-init with only that role's service file.

**VM count resolution:** If a role's `vm-count` is `0` (or omitted), the global `--vm-count` flag is used instead, with a minimum of 1 VM per role. Roles with an explicit `vm-count > 0` always use the manifest value.

### Parameter Substitution

Catalog workloads use `{{key}}` placeholders in service file content. At deploy time, each placeholder is replaced with:

1. The user-supplied value (via `--params entry-name.key=value`), if provided
2. The `default` from `workload.yaml`, otherwise

This is the catalog equivalent of calling `w.GetParam("key")` in a Go workload. The same five param types are supported: `string`, `int`, `bool`, `list` (semicolon-separated), and `dict` (semicolon-separated `key=value` pairs). The orchestrator validates param types before VM creation, rejecting unknown keys with "did you mean?" suggestions.

### Storage-Backed Catalog Entries

Catalog entries can declare persistent storage via the `storage:` field in `workload.yaml`. Each storage entry creates a CDI DataVolume, attaches it as an extra disk, and injects a `diskSetupScript` into the cloud-init userdata that waits for the device, formats it with XFS if empty, mounts it, and writes an `/etc/fstab` entry for persistence across reboots.

```yaml
storage:
  - name: data
    size: 10Gi
    serial: vw-data
    mount: /mnt/data
```

**Field reference:**

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | DataVolume base name (automatically suffixed with the VM name to avoid collisions) |
| `size` | Yes | Volume size (e.g., `10Gi`, `500Mi`) |
| `serial` | Yes | Virtio serial — the in-VM device is discovered at `/dev/disk/by-id/virtio-<serial>`, which is deterministic across reboots |
| `mount` | Yes | Mount point inside the VM (e.g., `/mnt/data`) |

This is the catalog equivalent of overriding `DataVolumeTemplates()`, `ExtraDisks()`, and `ExtraVolumes()` in a built-in workload. The same serial-based discovery pattern used by `disk.go`, `database.go`, and `chaos_disk.go` is applied automatically.

**Example — fio workload with persistent storage:**

`workload.yaml`:
```yaml
description: "fio benchmark with persistent volume"
packages:
  - fio
storage:
  - name: virtwork-fio-data
    size: 20Gi
    serial: vw-fio
    mount: /mnt/fio
params:
  - key: runtime
    type: int
    default: "300"
    desc: "fio runtime in seconds"
```

`workload.service`:
```ini
[Unit]
Description=fio benchmark
After=network-online.target

[Service]
Type=simple
ExecStart=/usr/bin/fio --name=randrw --directory=/mnt/fio --rw=randrw --bs=4k --size=1G --runtime={{runtime}} --time_based --ioengine=libaio --direct=1 --numjobs=4 --group_reporting
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

### Service-Backed Catalog Entries

Catalog entries can declare a K8s Service via the `service:` field in `workload.yaml`. This creates a ClusterIP Service that other VMs (or pods) can reach via in-cluster DNS.

```yaml
service:
  ports:
    - name: iperf
      port: 5201
      protocol: TCP
  selector-role: server
```

**Field reference:**

| Field | Required | Description |
|-------|----------|-------------|
| `ports[].name` | Yes | Port name |
| `ports[].port` | Yes | Port number |
| `ports[].protocol` | Yes | `TCP` or `UDP` |
| `selector-role` | No | For multi-role entries, the role whose VMs the Service selects. Omit for single-role entries (selects all VMs of this workload). |

This is the catalog equivalent of overriding `RequiresService()` and `ServiceSpec()` in a built-in workload. Clients reach the service via `<workload-name>.<namespace>.svc.cluster.local`.

### Mixing Built-in and Catalog Workloads

Both `--workloads` and `--from-catalog` can be specified in the same invocation:

```bash
virtwork run --workloads cpu,memory --from-catalog my-stress --dry-run
```

**Behavior:**
- If `--from-catalog` is set but `--workloads` is not explicitly provided, the default built-in workloads are cleared — only catalog entries run
- If both flags are set, all specified workloads (built-in + catalog) run together
- A catalog entry **cannot** shadow a built-in workload name (e.g., naming a catalog entry `cpu` produces an error)

### CLI Flags and Configuration

| Flag | Default | Description |
|------|---------|-------------|
| `--catalog-dir` | `~/.virtwork/catalog` | Path to catalog directory |
| `--from-catalog` | (none) | Catalog entry names to load (repeatable, comma-separated) |

| Environment Variable | Description |
|---------------------|-------------|
| `VIRTWORK_FROM_CATALOG` | Same as `--from-catalog` |

YAML config:
```yaml
catalog_dir: /path/to/catalog
from_catalog:
  - my-stress
  - my-benchmark
```

### How It Works

1. CLI parses `--from-catalog` and `--catalog-dir` into `Config.FromCatalog` and `Config.CatalogDir`
2. The orchestrator starts with `DefaultRegistry()` (built-in workloads only)
3. For each catalog entry name, `LoadCatalogEntry()` reads the directory, parses the manifest, and discovers service files
4. `CatalogEntry.Factory()` returns a `WorkloadFactory` that produces either a `GenericWorkload` (single-role) or `GenericMultiWorkload` (multi-role)
5. The entry is injected into the registry as a `RegistryEntry` with its factory and param schema
6. From this point, catalog workloads follow the same VM planning, creation, and cloud-init pipeline as built-in workloads

## Writing Tests for Workloads

Whether adding a built-in workload or a catalog entry, write tests that cover the workload's cloud-init output and configuration.

### Built-in Workload Tests

Create `internal/workloads/my_workload_test.go` using Ginkgo:

```go
var _ = Describe("MyWorkload", func() {
    var wl *MyWorkload

    BeforeEach(func() {
        wl = NewMyWorkload(config.WorkloadConfig{CPUCores: 2, Memory: "2Gi"}, "virtwork", "", nil)
    })

    It("should return correct name", func() {
        Expect(wl.Name()).To(Equal("my-workload"))
    })

    It("should produce valid cloud-init YAML", func() {
        userdata, err := wl.CloudInitUserdata()
        Expect(err).NotTo(HaveOccurred())
        Expect(userdata).To(HavePrefix("#cloud-config"))

        var parsed map[string]interface{}
        Expect(yaml.Unmarshal([]byte(userdata), &parsed)).To(Succeed())
    })

    It("should reflect config in VMResources", func() {
        res := wl.VMResources()
        Expect(res.CPUCores).To(Equal(2))
        Expect(res.Memory).To(Equal("2Gi"))
    })
})
```

### Catalog Entry Tests

Catalog entries are tested through `GenericWorkload` and `GenericMultiWorkload`. Create a temporary catalog directory in your test, write the manifest and service files, then verify loading and cloud-init output:

```go
var _ = Describe("My catalog entry", func() {
    var catalogDir string

    BeforeEach(func() {
        catalogDir, _ = os.MkdirTemp("", "virtwork-catalog-*")
        entryDir := filepath.Join(catalogDir, "my-entry")
        os.MkdirAll(entryDir, 0o755)

        os.WriteFile(filepath.Join(entryDir, "workload.yaml"), []byte(`
description: "test entry"
packages:
  - curl
params:
  - key: url
    type: string
    default: "http://example.com"
    desc: "Target URL"
`), 0o644)

        os.WriteFile(filepath.Join(entryDir, "workload.service"), []byte(`[Service]
ExecStart=/usr/bin/curl {{url}}
`), 0o644)
    })

    AfterEach(func() {
        os.RemoveAll(catalogDir)
    })

    It("should load and substitute params", func() {
        entry, err := workloads.LoadCatalogEntry(catalogDir, "my-entry")
        Expect(err).NotTo(HaveOccurred())

        factory := entry.Factory()
        wl := factory(config.WorkloadConfig{CPUCores: 1, Memory: "1Gi"}, &workloads.RegistryOpts{})
        userdata, err := wl.CloudInitUserdata()
        Expect(err).NotTo(HaveOccurred())
        Expect(userdata).To(ContainSubstring("http://example.com"))
    })
})
```

See `internal/workloads/catalog_test.go`, `generic_test.go`, and `generic_multi_test.go` for comprehensive examples covering error cases, multi-role entries, and param overrides.

## SSH Credential Configuration

VMs created by virtwork can be configured with SSH access for debugging and inspection.

### CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--ssh-user` | `virtwork` | Username for the VM user account |
| `--ssh-password` | (none) | Password for the VM user account |
| `--ssh-key` | (none) | Inline SSH public key (repeatable) |
| `--ssh-key-file` | (none) | Path to SSH public key file (repeatable) |

### Environment Variables

| Variable | Description |
|----------|-------------|
| `VIRTWORK_SSH_USER` | Same as `--ssh-user` |
| `VIRTWORK_SSH_PASSWORD` | Same as `--ssh-password` |
| `VIRTWORK_SSH_AUTHORIZED_KEYS` | Comma-separated SSH public keys |

### YAML Config

```yaml
ssh_user: virtwork
ssh_password: testpass
ssh_authorized_keys:
  - ssh-rsa AAAA...
  - ssh-ed25519 AAAA...
```

> **Note:** `--ssh-key-file` is a CLI-only convenience; it has no YAML or env-var equivalent. To configure SSH keys via YAML, supply the full public-key strings in the `ssh_authorized_keys` list.

### How It Works

SSH credentials are a global, cross-cutting concern applied to all VMs:

1. Config layer merges SSH fields from CLI/env/YAML with standard priority chain
2. Orchestration passes SSH credentials to the workload registry via functional options
3. `BaseWorkload` stores SSH fields and provides `BuildCloudConfig()` helper
4. Each workload calls `w.BuildCloudConfig(opts)` — SSH user/password/keys are injected automatically
5. The cloud-init output includes a `users` block with the configured credentials

When no SSH flags are provided, no `users` block is emitted — backward compatible with pre-SSH behavior.

## Audit Configuration

Every execution is tracked in a local SQLite database for operational visibility and compliance auditing.

### CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--audit` | `true` | Enable audit tracking (persistent flag) |
| `--no-audit` | — | Disable audit tracking |
| `--audit-db` | `virtwork.db` | Path to SQLite audit database file (persistent flag) |
| `--run-id` | (none) | Target a specific run for cleanup (cleanup command only) |

### Environment Variables

| Variable | Description |
|----------|-------------|
| `VIRTWORK_AUDIT` | Same as `--audit` (true/false) |
| `VIRTWORK_AUDIT_DB` | Same as `--audit-db` |

### How It Works

1. Each `virtwork run` or `virtwork cleanup` generates a UUID
2. The UUID is applied as a `virtwork/run-id` label on all K8s resources
3. An `audit_log` row records execution parameters, timestamps, and outcome
4. Detailed records are written to `workload_details`, `vm_details`, `resource_details`, and `events` tables
5. During cleanup, `virtwork/run-id` labels are collected from resources and stored as a JSON array in `linked_run_ids`
6. No SSH credentials are stored — only a `ssh_auth_configured` boolean

### Querying the Audit Database

```bash
# Recent executions
sqlite3 virtwork.db "SELECT id, run_id, command, status, started_at FROM audit_log ORDER BY id DESC LIMIT 10;"

# VMs created in a specific run
sqlite3 virtwork.db "SELECT vm_name, component, cpu_cores, memory, status FROM vm_details WHERE audit_id = 1;"

# Events timeline
sqlite3 virtwork.db "SELECT event_type, message, occurred_at FROM events WHERE audit_id = 1 ORDER BY occurred_at;"

# Find cleanups that affected a specific run
sqlite3 virtwork.db "SELECT * FROM audit_log WHERE command = 'cleanup' AND linked_run_ids LIKE '%<run-uuid>%';"
```

### Security Note

SSH passwords configured via `--ssh-password` appear in two places as plaintext:

1. **Process listing** — The password is visible in `ps aux` output when passed as a CLI flag. Use the `VIRTWORK_SSH_PASSWORD` environment variable or a YAML config file to avoid this.
2. **KubeVirt VM spec** — The password appears as `plain_text_passwd` in the cloud-init userdata, which is stored in the VirtualMachine custom resource. Anyone with read access to VM objects in the namespace can see it via `oc get vm <name> -o yaml`.

This is acceptable for test/lab environments. For production use, prefer SSH key-only authentication (`--ssh-key-file`) with no password.

### Accessing VMs

```bash
# Via virtctl (after deploying with --ssh-key-file)
virtctl ssh --ssh-key ~/.ssh/id_rsa virtwork@virtwork-cpu-0

# Via oc (port forward then SSH)
oc port-forward vmi/virtwork-cpu-0 2222:22
ssh -p 2222 virtwork@localhost
```

---

## Testing Patterns

### YAML Assertion Pattern

When testing cloud-init or any YAML output, always parse the YAML string before asserting on values:

```go
// GOOD: Parse, then assert on structure
userdata, err := wl.CloudInitUserdata()
Expect(err).NotTo(HaveOccurred())

var parsed map[string]interface{}
Expect(yaml.Unmarshal([]byte(userdata), &parsed)).To(Succeed())
Expect(parsed).To(HaveKey("packages"))

// BAD: Assert on raw string (fragile — key order, whitespace, line folding)
Expect(userdata).To(ContainSubstring("packages:\n- stress-ng"))
```

### Workload Systemd Unit Pattern

Each workload writes a systemd `.service` file via cloud-init `write_files`, then enables/starts it via `runcmd`. This ensures workloads survive VM reboots and can be managed with standard systemd tooling.

For workloads with initialization (database), use `ExecStartPre` for setup and `ExecStart` for the main loop. For workloads with multiple configurations (disk/fio), write job files as separate `write_files` entries.

---

## Commit Conventions

This project uses [Conventional Commits](https://www.conventionalcommits.org/):

| Prefix | Use For |
|--------|---------|
| `feat:` | New functionality |
| `fix:` | Bug fixes |
| `test:` | Test additions or changes |
| `docs:` | Documentation only |
| `refactor:` | Code restructuring without behavior change |
| `chore:` | Build, tooling, or maintenance |

Every commit must include a DCO `Signed-off-by` trailer matching the author's identity; a `commit-msg` hook enforces this.

---

## Related Documentation

- [architecture.md](architecture.md) — Layered architecture, mermaid diagrams, key design decisions
- [configuration.md](configuration.md) — Complete config reference (flags, env vars, YAML keys, ConfigMap)
- [audit-schema.md](audit-schema.md) — SQLite schema for the audit database
- [chaos-workloads.md](chaos-workloads.md) — Operator guide for chaos-disk, chaos-network, chaos-process
- [deployment.md](deployment.md) — OpenShift deployment via Kustomize
- [guide/03-adding-a-workload.md](guide/03-adding-a-workload.md) — TDD walkthrough that builds a new workload from scratch

## Idempotency and Safety

- `apierrors.IsAlreadyExists()` responses are treated as success (resource already exists)
- `apierrors.IsTooManyRequests()` and server errors trigger retry with exponential backoff
- `apierrors.IsNotFound()` is fatal for CRUD (CNV not installed?)
- `apierrors.IsUnauthorized()` / `apierrors.IsForbidden()` are fatal (auth errors)
- All created resources are labeled with `app.kubernetes.io/managed-by: virtwork` for cleanup tracking
- `--dry-run` prints specs without any cluster interaction
- OpenShift HAProxy load balancers may drop the first TLS connection when connection pools are cold, causing transient failures on the first API call after an idle period. The retry logic (backoff on `IsTooManyRequests()` and server errors) covers this. If running against remote clusters and seeing intermittent first-call failures, this is expected behavior — the retry will succeed.
