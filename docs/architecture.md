# Virtwork Architecture

## Overview

Virtwork is a CLI tool that creates virtual machines on OpenShift clusters (with OpenShift Virtualization / CNV installed) and runs continuous workloads inside them. The goal is to produce realistic CPU, memory, database, network, and disk I/O metrics for monitoring systems (Prometheus, Grafana).

It is a **one-shot deployment tool** — it creates resources and exits. Workload lifecycle management is delegated to systemd inside each VM.

---

## Layered Architecture

The codebase is organized into five dependency layers. Each layer depends only on layers below it.

```mermaid
graph TD
    subgraph "Layer 4 — Orchestration"
        CMD["cmd/virtwork/main.go\nCobra commands + orchestration"]
        CLEANUP["internal/cleanup/cleanup.go\nlabel-based teardown\n(VMs, Services, Secrets)"]
        AUDIT["internal/audit/audit.go\nSQLite audit tracking"]
    end

    subgraph "Layer 3 — Workload Definitions"
        REGISTRY["internal/workloads/registry.go\nregistry + lookup"]
        IFACE["internal/workloads/workload.go\nWorkload interface"]
        MULTI["internal/workloads/workload.go\nMultiVMWorkload interface\n(Roles, UserdataForRole)"]
        CPU["internal/workloads/cpu.go\nstress-ng CPU"]
        MEM["internal/workloads/memory.go\nstress-ng VM memory"]
        DISK["internal/workloads/disk.go\nfio profiles"]
        DB["internal/workloads/database.go\nPostgreSQL + pgbench"]
        NET["internal/workloads/network.go\niperf3 server/client"]
        TPS["internal/workloads/tps.go\nnetperf TCP_RR + HTTP"]
        CDISK["internal/workloads/chaos_disk.go\nfill/release loop"]
        CNET["internal/workloads/chaos-network.go\ntc/netem latency + loss"]
        CPROC["internal/workloads/chaos_process.go\nrandom signal killer"]
    end

    subgraph "Layer 2 — K8s Abstractions"
        VM["internal/vm/vm.go\nVM spec CRUD + retry"]
        RES["internal/resources/resources.go\nnamespace + service + secret"]
        WAIT["internal/wait/wait.go\nVMI readiness polling"]
    end

    subgraph "Layer 1 — Infrastructure"
        CLUSTER["internal/cluster/cluster.go\ncontroller-runtime client init"]
        CONFIG["internal/config/config.go\nViper config"]
        CLOUDINIT["internal/cloudinit/cloudinit.go\ncloud-config YAML builder"]
        LOGGING["internal/logging/logging.go\nstructured slog logger"]
    end

    subgraph "Layer 0 — Definitions"
        CONST["internal/constants/constants.go\nAPI coords, labels, defaults"]
    end

    CMD --> CONFIG
    CMD --> CLUSTER
    CMD --> REGISTRY
    CMD --> VM
    CMD --> RES
    CMD --> WAIT
    CMD --> CLEANUP
    CMD --> AUDIT
    CMD --> LOGGING

    AUDIT --> CONFIG
    AUDIT --> CONST

    CLEANUP --> VM
    CLEANUP --> RES
    CLEANUP --> CONST

    REGISTRY --> CPU
    REGISTRY --> MEM
    REGISTRY --> DISK
    REGISTRY --> DB
    REGISTRY --> NET
    REGISTRY --> TPS
    REGISTRY --> CDISK
    REGISTRY --> CNET
    REGISTRY --> CPROC

    CPU --> IFACE
    MEM --> IFACE
    DISK --> IFACE
    DB --> IFACE
    CDISK --> IFACE
    CNET --> IFACE
    CPROC --> IFACE

    NET --> MULTI
    TPS --> MULTI
    MULTI --> IFACE

    VM --> CLUSTER
    VM --> CONST
    VM --> CLOUDINIT

    RES --> CLUSTER
    RES --> CONST

    WAIT --> CLUSTER
    WAIT --> CONST
    WAIT --> LOGGING

    CLUSTER --> CONST
    CONFIG --> CONST

    MAIN["cmd/virtwork/main.go"] --> CMD
```

---

## Concurrency Model

Go's native concurrency eliminates the need for async/sync bridging. All I/O operations run naturally in goroutines, coordinated by `errgroup` and controlled by `context.Context`.

```mermaid
graph LR
    subgraph "Orchestration (main goroutine)"
        CLI_RUN["Cobra RunE\nentry point"]
        ERRGRP["errgroup.Group\nparallel VM creation\nconcurrent polling"]
    end

    subgraph "Goroutines (spawned by errgroup)"
        G1["goroutine: CreateVM cpu-0"]
        G2["goroutine: CreateVM disk-0"]
        G3["goroutine: CreateVM db-0"]
        G4["goroutine: WaitForVMReady cpu-0"]
    end

    subgraph "Context"
        CTX["context.WithTimeout\ncancellation + deadline"]
    end

    CLI_RUN --> ERRGRP
    ERRGRP --> G1
    ERRGRP --> G2
    ERRGRP --> G3
    ERRGRP --> G4
    CTX --> G1
    CTX --> G2
    CTX --> G3
    CTX --> G4
```

```mermaid
graph LR
    subgraph "controller-runtime client"
        CR["client.Client\ntyped Get/Create/List/Delete"]
    end

    subgraph "K8s API Types"
        KV["kubevirtv1.VirtualMachine"]
        VMI["kubevirtv1.VirtualMachineInstance"]
        CDI["cdiv1beta1.DataVolume"]
        CORE["corev1.Namespace / Service"]
    end

    CR --> KV
    CR --> VMI
    CR --> CDI
    CR --> CORE
```

| Package | Goroutines | Rationale |
|---------|-----------|-----------|
| `internal/constants` | No | Pure values, no I/O |
| `internal/config` | No | One-time Viper load at startup |
| `internal/cloudinit` | No | Pure string/YAML generation |
| `internal/cluster` | No | One-time client init at startup |
| `internal/logging` | No | Stateless `slog` wrapper; safe to share across goroutines that call it |
| `internal/vm` | Yes | CRUD operations run in errgroup goroutines; retry loops use `time.Sleep` |
| `internal/resources` | Yes | Namespace/Service/Secret creation can run concurrently |
| `internal/wait` | Yes | Concurrent VMI polling via errgroup; uses `time.Sleep` between polls |
| `internal/workloads` | No | Pure data producers (cloud-init specs, resource structs) |
| `internal/cleanup` | No | Sequential VM/Service/Secret deletion with error accumulation |
| `internal/audit` | No | Sequential SQLite writes via `database/sql` connection pool |
| `cmd/virtwork` | Yes | Owns errgroup lifecycle; spawns goroutines for parallel operations |

---

## CLI Orchestration Flow

```mermaid
flowchart TD
    START([virtwork run]) --> LOAD_CFG[Load config via Viper\nflags > env > file > defaults]
    LOAD_CFG --> INIT_AUDIT[Init Auditor\nSQLiteAuditor or NoOpAuditor]
    INIT_AUDIT --> START_EXEC[StartExecution\ngenerate run UUID]
    START_EXEC --> DRY_CHECK{--dry-run?}

    DRY_CHECK -->|Yes| GEN_SPECS[Generate VM specs\nfor each workload]
    GEN_SPECS --> PRINT_YAML[Print specs as YAML]
    PRINT_YAML --> EXIT_D([Exit])

    DRY_CHECK -->|No| CONNECT[Connect to cluster\ncontroller-runtime client.New]
    CONNECT --> ENSURE_NS[EnsureNamespace]
    ENSURE_NS --> CTX_CREATE[Create context.WithTimeout]
    CTX_CREATE --> WORKLOAD_LOOP[For each enabled workload]

    WORKLOAD_LOOP --> GET_WL[Get workload from registry]
    GET_WL --> GEN_CI[Generate cloud-init userdata]
    GEN_CI --> SVC_CHECK{RequiresService?}
    SVC_CHECK -->|Yes| CREATE_SVC[CreateService\nmust exist before VMs for DNS]
    SVC_CHECK -->|No| SPAWN_VMS
    CREATE_SVC --> SPAWN_VMS[Spawn goroutines via errgroup\nBuildVMSpec + CreateVM]
    SPAWN_VMS --> MORE_WL{More workloads?}
    MORE_WL -->|Yes| WORKLOAD_LOOP
    MORE_WL -->|No| ERRGRP_WAIT[errgroup.Wait\nall VM creates]

    ERRGRP_WAIT --> WAIT_CHECK{--no-wait?}
    WAIT_CHECK -->|Yes| PRINT_SUMMARY[Print summary table]
    WAIT_CHECK -->|No| POLL[WaitForAllVMsReady\nerrgroup for concurrent polling]
    POLL --> COMPLETE_EXEC[CompleteExecution\nset status + timestamp]
    COMPLETE_EXEC --> PRINT_SUMMARY
    PRINT_SUMMARY --> EXIT([Exit])
```

```mermaid
flowchart TD
    START_C([virtwork cleanup]) --> LOAD_CFG_C[Load config via Viper]
    LOAD_CFG_C --> INIT_AUDIT_C[Init Auditor\nSQLiteAuditor or NoOpAuditor]
    INIT_AUDIT_C --> START_EXEC_C[StartExecution\ngenerate cleanup run UUID]
    START_EXEC_C --> CONNECT_C[Connect to cluster]
    CONNECT_C --> DO_CLEANUP[CleanupAll\ndelete labeled VMs + Services\ncollect run IDs from resources]
    DO_CLEANUP --> LINK_RUNS[LinkCleanupToRuns\nstore collected run IDs as JSON array]
    LINK_RUNS --> RECORD_COUNTS[RecordCleanupCounts\nVMs, Services, Secrets deleted]
    RECORD_COUNTS --> COMPLETE_C[CompleteExecution\nset status + timestamp]
    COMPLETE_C --> PRINT_SUMMARY_C[Print cleanup summary]
    PRINT_SUMMARY_C --> EXIT_C([Exit])
```

---

## Workload Architecture

Each workload implements the `Workload` interface and produces cloud-init userdata and VM resource requirements. Workloads do not perform any I/O — they are pure data producers.

```mermaid
classDiagram
    class Workload {
        <<interface>>
        +Name() string
        +CloudInitUserdata() (string, error)
        +VMResources() VMResourceSpec
        +ExtraVolumes() []Volume
        +ExtraDisks() []Disk
        +DataVolumeTemplates() []DataVolumeTemplateSpec
        +RequiresService() bool
        +ServiceSpec() *Service
        +VMCount() int
    }

    class MultiVMWorkload {
        <<interface>>
        +Roles() []string
        +UserdataForRole(role, namespace) (string, error)
    }

    class BaseWorkload {
        +Config WorkloadConfig
        +SSHUser string
        +SSHPassword string
        +SSHAuthorizedKeys []string
        +VMResources() VMResourceSpec
        +ExtraVolumes() []Volume
        +ExtraDisks() []Disk
        +DataVolumeTemplates() []DataVolumeTemplateSpec
        +RequiresService() false
        +ServiceSpec() nil
        +VMCount() int (Config.VMCount or 1)
        +BuildCloudConfig(opts) (string, error)
    }

    class CPUWorkload {
        +Name() "cpu"
        +CloudInitUserdata() stress-ng --cpu config
    }

    class MemoryWorkload {
        +Name() "memory"
        +CloudInitUserdata() stress-ng --vm config
    }

    class DiskWorkload {
        +Name() "disk"
        +CloudInitUserdata() fio profiles
        +DataVolumeTemplates() blank DV for /mnt/data
        +ExtraDisks() data disk with virtio Serial
    }

    class DatabaseWorkload {
        +Name() "database"
        +CloudInitUserdata() postgresql + pgbench
        +DataVolumeTemplates() blank DV for /var/lib/pgsql/data
        +ExtraDisks() data disk with virtio Serial
    }

    class NetworkWorkload {
        +Namespace string
        +Name() "network"
        +VMCount() count * 2
        +Roles() ["server", "client"]
        +UserdataForRole(role, ns) iperf3 server or client
        +RequiresService() true
        +ServiceSpec() ClusterIP virtwork-iperf3-server :5201
    }

    class TPSWorkload {
        +Namespace string
        +Name() "tps"
        +VMCount() count * 2
        +Roles() ["server", "client"]
        +UserdataForRole(role, ns) netperf + HTTP server / client loop
        +RequiresService() true
        +ServiceSpec() ClusterIP virtwork-tps-server :12865/:12866/:8080
        +Params file-size, iterations, duration
    }

    class ChaosDiskWorkload {
        +Name() "chaos-disk"
        +CloudInitUserdata() fallocate fill / rm release loop
        +DataVolumeTemplates() blank DV for /mnt/data
        +ExtraDisks() data disk with virtio Serial
    }

    class ChaosNetworkWorkload {
        +Latency int (ms)
        +PacketLoss float64 (%)
        +Name() "chaos-network"
        +CloudInitUserdata() tc qdisc netem delay + loss
    }

    class ChaosProcessWorkload {
        +Name() "chaos-process"
        +CloudInitUserdata() random kill loop with excluded patterns
    }

    Workload <|-- MultiVMWorkload
    Workload <|.. BaseWorkload
    BaseWorkload <|-- CPUWorkload
    BaseWorkload <|-- MemoryWorkload
    BaseWorkload <|-- DiskWorkload
    BaseWorkload <|-- DatabaseWorkload
    BaseWorkload <|-- NetworkWorkload
    BaseWorkload <|-- TPSWorkload
    BaseWorkload <|-- ChaosDiskWorkload
    BaseWorkload <|-- ChaosNetworkWorkload
    BaseWorkload <|-- ChaosProcessWorkload
    MultiVMWorkload <|.. NetworkWorkload
    MultiVMWorkload <|.. TPSWorkload
```

`BaseWorkload` is an embedded struct that provides default implementations for optional interface methods. Concrete workloads embed `BaseWorkload` and override only the methods they need — idiomatic Go composition over inheritance.

`BaseWorkload` also stores SSH credential fields and exposes a `BuildCloudConfig(opts)` helper method that injects SSH user/password/keys into the cloud-init output. Workload subclasses call `w.BuildCloudConfig(opts)` instead of `cloudinit.BuildCloudConfig(opts)` directly, keeping SSH injection as a single cross-cutting concern on the base struct.

### Workload Comparison

| Workload | VM Count | Data Volume | K8s Service | Packages | Workload Tool |
|----------|----------|-------------|-------------|----------|---------------|
| CPU | N (configurable) | No | No | stress-ng | `stress-ng --cpu 0 --cpu-method all` |
| Memory | N (configurable) | No | No | stress-ng | `stress-ng --vm 1 --vm-bytes 80% --vm-method all` |
| Disk | N (configurable) | Yes (`/mnt/data`) | No | fio | Mixed R/W + sequential write profiles |
| Database | N (configurable) | Yes (`/var/lib/pgsql/data`) | No | postgresql-server | `pgbench -c 10 -j 2 -T 300` loop |
| Network | N × 2 (server + client) | No | Yes — `virtwork-iperf3-server` :5201 | iperf3 | `iperf3 -s` / `iperf3 -c ... --bidir` |
| TPS | N × 2 (server + client) | No | Yes — `virtwork-tps-server` :12865 / :12866 / :8080 | netperf, python3 | `netperf -t TCP_RR` + curl HTTP file fetch loop |
| Chaos-disk | N (configurable) | Yes (`/mnt/data`) | No | (golden image: `fallocate`, `dd`) | Fill to target percent, sleep, release, repeat |
| Chaos-network | N (configurable) | No | No | iproute-tc (+ `sch_netem` kernel module) | `tc qdisc add ... netem delay 100ms loss 5%` |
| Chaos-process | N (configurable) | No | No | procps-ng | Random `kill -SIGTERM <pid>` of non-essential processes every 30s |

---

## Resource Tracking and Cleanup

All created resources are labeled with `app.kubernetes.io/managed-by: virtwork` and `virtwork/run-id: <uuid>`. Cleanup queries by label selector — no state file needed. This is resilient to crashes (works even if the tool terminated mid-creation).

Each `virtwork run` generates a UUID applied as the `virtwork/run-id` label to all resources it creates. During cleanup, these labels enable:
- **Targeted cleanup:** `virtwork cleanup --run-id <uuid>` deletes only resources from that specific run
- **Cleanup-all:** `virtwork cleanup` (no UUID) deletes all managed resources and collects unique run IDs from the resources into a JSON array for audit linking

```mermaid
flowchart LR
    subgraph "Create (with run UUID)"
        VM1["VM: virtwork-cpu-0\nlabels: managed-by=virtwork\nvirtwork/run-id=abc123"]
        VM2["VM: virtwork-disk-0\nlabels: managed-by=virtwork\nvirtwork/run-id=abc123"]
        SVC["Service: virtwork-iperf3-server\nlabels: managed-by=virtwork\nvirtwork/run-id=abc123"]
        SEC["Secret: virtwork-cpu-0-cloudinit\nlabels: managed-by=virtwork\nvirtwork/run-id=abc123"]
        NS["Namespace: virtwork"]
    end

    subgraph "Cleanup Query"
        SEL["client.MatchingLabels\nmanaged-by=virtwork\n(+ optional run-id filter)"]
    end

    subgraph "Delete + Audit"
        DEL["client.Delete each matched resource\nerrors logged, not fatal\ncollect unique run IDs"]
    end

    SEL --> VM1
    SEL --> VM2
    SEL --> SVC
    SEL --> SEC
    DEL --> NS
```

---

## SSH Credential Flow

SSH credentials are a cross-cutting concern that flows through every layer:

```mermaid
flowchart LR
    CLI["CLI flags\n--ssh-user, --ssh-password\n--ssh-key, --ssh-key-file"]
    ENV["Env vars\nVIRTWORK_SSH_USER\nVIRTWORK_SSH_PASSWORD\nVIRTWORK_SSH_AUTHORIZED_KEYS"]
    YAML["Config YAML\nssh_user, ssh_password\nssh_authorized_keys"]

    CLI --> CONFIG["Config struct\nSSHUser, SSHPassword\nSSHAuthorizedKeys"]
    ENV --> CONFIG
    YAML --> CONFIG

    CONFIG --> ORCH["Orchestration\npasses SSH opts to registry"]
    ORCH --> BASE["BaseWorkload\nstores SSH fields"]
    BASE --> HELPER["BuildCloudConfig helper\ninjects users block"]
    HELPER --> CI["cloud-init userdata\n#cloud-config with users block"]
    CI --> VM["VM spec\ncloudInitNoCloud.userData"]
```

List fields (`SSHAuthorizedKeys`) require special handling at each config layer: YAML passes lists directly, environment variables use comma separation, and CLI merges values from both `--ssh-key` (inline) and `--ssh-key-file` (file path) flags.

---

## Configuration Priority Chain

```mermaid
flowchart LR
    COBRA["Cobra flags\n--namespace virtwork-test"]
    ENV["Viper env vars\nVIRTWORK_NAMESPACE"]
    YAML["Viper config file\nnamespace: virtwork-prod"]
    DEFAULTS["Viper defaults\nnamespace: virtwork"]

    COBRA -->|highest priority| MERGE
    ENV -->|2nd| MERGE
    YAML -->|3rd| MERGE
    DEFAULTS -->|lowest| MERGE
    MERGE["Merged Config\nstruct"] --> RUN["Runtime"]
```

Viper's built-in priority chain handles this natively when bound to Cobra flags:
1. Cobra flag explicitly set by user
2. Environment variable (`VIRTWORK_` prefix, automatic binding)
3. Config file (YAML, loaded via `viper.ReadInConfig()`)
4. Default value (set via `viper.SetDefault()`)

---

## Key Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Boot disk | `containerDisk` | Fast kubelet image pull, cached on nodes. Ephemeral root is fine for workload VMs. |
| Data disk | Blank `DataVolume` | Formatted on first boot by cloud-init. Only needed for database and fio workloads. |
| Workload lifecycle | systemd services | Survive reboots, auto-restart on failure, proper logging via journald. |
| Network coordination | K8s Service + DNS | No IP polling from Go. Client retries via systemd `Restart=always`. |
| Cleanup tracking | Label selectors | No state file. Works even if tool crashed mid-creation. |
| Auth | In-cluster first, kubeconfig fallback | Works both inside pods (CI/CD) and from developer machines. |
| Concurrency | goroutines + `errgroup` | Native Go concurrency with structured error collection. No async/sync bridge needed. |
| K8s client | controller-runtime `client.Client` | Typed CRUD operations. Scheme-based serialization for KubeVirt/CDI types. Common in OpenShift ecosystem. |
| Idempotency | AlreadyExists = skip | Safe to re-run. Enables declarative approach. |
| Retry | Backoff for rate-limited/5xx | Handles transient cluster issues. NotFound/Unauthorized/Forbidden are fatal (configuration errors). |
| SSH credential injection | `BaseWorkload.BuildCloudConfig()` helper | Cross-cutting concern handled once in base struct. Workloads call one method. |
| Multi-VM orchestration | `MultiVMWorkload` interface + `VMCount() > 1` | Generic detection — future multi-VM workloads work without orchestration changes. |
| Network VM scaling | `VMCount() = count * 2` | Honors `--vm-count` to create N server/client pairs instead of a single hardcoded pair. |
| Cloud-init Secrets | `CloudInitSecretName` → `UserDataSecretRef` | For large userdata, stores cloud-init in a K8s Secret instead of inline in the VM spec. |
| Cleanup error semantics | Sequential per-resource deletion with error accumulation | Different from create-time error handling (which is fail-fast). Cleanup continues on individual failures. |
| Audit storage | SQLite (`virtwork.db`) with `Auditor` interface | Local file, zero infrastructure. `NoOpAuditor` when disabled. WAL mode for concurrent safety. See [audit-schema.md](audit-schema.md) for the full schema. |
| Run-to-cleanup linking | `virtwork/run-id` K8s label + `linked_run_ids` JSON array | Labels survive across CLI invocations. JSON array is PostgreSQL JSONB compatible. |
| Audit credential policy | No SSH credentials stored | Only `ssh_auth_configured` boolean tracked. Security by design. |
| In-VM disk discovery | `/dev/disk/by-id/virtio-<serial>` via the `Serial` field on KubeVirt `Disk` | `/dev/vdX` device ordering is not stable across VM reboots or migrations; the virtio serial provides a deterministic symlink. The shared `diskSetupScript` helper (`internal/workloads/workload.go`) waits for the symlink, formats if empty, mounts, and writes `/etc/fstab`. |
| DataVolume names per VM | DV template names are suffixed with the VM name via `namespaceDataVolumes` (`cmd/virtwork/main.go`) | DataVolume names are namespace-scoped; deploying multiple VMs of the same workload would otherwise collide on the template name. |
| Structured logging | `log/slog` JSON via `internal/logging.NewLogger(out, verbose)` | Machine-parseable logs for pipeline consumption; `--verbose` flips the level from `INFO` to `DEBUG`. New code uses the logger; `fmt.Fprintf` calls were removed from `cmd/virtwork/main.go`. |
| Chaos workload safety | Opt-in by name, namespace-scoped destructive behavior, no platform-level kill switch | Chaos workloads can fill a data PVC, shape egress traffic, or kill processes — all confined to the VM they run in. Namespace isolation is the safety boundary. See [chaos-workloads.md](chaos-workloads.md) for risk and operational guidance. |

---

## Project Structure

```
virtwork/
├── cmd/
│   └── virtwork/
│       └── main.go                # Cobra root + subcommands, orchestration
├── internal/
│   ├── constants/
│   │   └── constants.go           # API coords, labels, defaults
│   ├── config/
│   │   └── config.go              # Config struct, Viper priority chain
│   ├── cluster/
│   │   └── cluster.go             # controller-runtime client init + scheme registration
│   ├── cloudinit/
│   │   └── cloudinit.go           # Cloud-config YAML builder
│   ├── vm/
│   │   └── vm.go                  # VM spec construction + typed CRUD + retry
│   ├── resources/
│   │   └── resources.go           # Namespace + Service + Secret helpers
│   ├── wait/
│   │   └── wait.go                # VMI readiness polling (errgroup)
│   ├── cleanup/
│   │   └── cleanup.go             # Label-based teardown (VMs, Services, Secrets)
│   ├── audit/
│   │   ├── audit.go               # Auditor interface, SQLiteAuditor, NoOpAuditor
│   │   ├── schema.go              # DDL for 5 audit tables + indexes
│   │   └── records.go             # WorkloadRecord, VMRecord, ResourceRecord, EventRecord
│   ├── workloads/
│   │   ├── workload.go            # Workload interface + BaseWorkload
│   │   ├── registry.go            # Registry map + lookup
│   │   ├── cpu.go                 # stress-ng CPU continuous workload
│   │   ├── memory.go              # stress-ng VM memory pressure workload
│   │   ├── database.go            # PostgreSQL + pgbench loop
│   │   ├── network.go             # iperf3 server/client pair
│   │   └── disk.go                # fio mixed I/O profiles
│   └── testutil/
│       ├── testutil.go            # Shared test helpers (namespace, connect, cleanup)
│       └── binary.go              # Binary build/run helpers for E2E
├── tests/
│   └── e2e/                       # E2E acceptance tests (//go:build e2e)
├── docs/
│   ├── architecture.md            # This file
│   ├── development.md             # Developer guide
│   ├── implementation-plan.md     # Phased build plan
│   ├── openshift-virtualization-workload-automation.md  # Design plan
│   └── engineering-journals/      # Per-phase development journals
├── go.mod
├── go.sum
└── CLAUDE.md
```
