# Tutorial: Adding a Catalog Workload

This tutorial walks through creating a new workload for virtwork **without writing any Go code**. By the end, you'll have a catalog entry that deploys VMs running a continuous sysbench CPU stress test — built entirely from a systemd service file and a YAML manifest.

## What We're Building

A "sysbench" catalog workload that:

- Installs **sysbench** from Fedora's default repos
- Runs continuous CPU stress with configurable threads, duration, and method
- Deploys on one or more VMs with no custom Go code
- Validates parameters at deploy time using the same schema system as built-in workloads

The catalog system is virtwork's extension mechanism for operators. Where [Tutorial 03](03-adding-a-workload.md) builds a workload by writing Go code, registering it in the binary, and recompiling — this tutorial creates a workload by writing two files in a directory. The trade-off: catalog workloads can't implement custom Go logic, but they cover the common case (install packages, write a systemd unit, run it) without a development environment.

## Before You Start

- The `virtwork` binary built or available via `go run ./cmd/virtwork`
- A text editor
- For deployment steps: an OpenShift cluster with [OpenShift Virtualization](https://docs.openshift.com/container-platform/latest/virt/about_virt/about-virt.html) (CNV) installed
- Read [How Virtwork Works](01-overview.md) to understand the workload interface and cloud-init pipeline

No Go toolchain is required for catalog workloads. If you can write a systemd unit file and basic YAML, you can build a catalog workload.

## Step 1: Create the Simplest Catalog Entry

A catalog entry is a directory containing at least one `.service` file. Let's start with the absolute minimum — no manifest, just a service file.

### Create the directory

```bash
mkdir -p ~/.virtwork/catalog/sysbench
```

The directory name (`sysbench`) becomes the workload name. The default catalog location is `~/.virtwork/catalog/`; override it with `--catalog-dir`.

### Write the service file

Create `~/.virtwork/catalog/sysbench/workload.service`:

```ini
[Unit]
Description=Virtwork sysbench CPU stress
After=network-online.target

[Service]
Type=simple
ExecStart=/usr/bin/sysbench cpu --threads=4 --time=0 run
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

This is a standard systemd unit. The key directives:

- **`ExecStart`** — Runs sysbench in CPU mode with 4 threads indefinitely (`--time=0`)
- **`Restart=always`** — Restarts if the process crashes
- **`RestartSec=10`** — Waits 10 seconds before restart
- **`WantedBy=multi-user.target`** — Starts on boot

### Validate with dry run

```bash
virtwork run --from-catalog sysbench --dry-run
```

The output shows a VM spec with cloud-init that writes your service file to `/etc/systemd/system/workload.service`, runs `systemctl daemon-reload`, and enables it. This works with no `workload.yaml` — the manifest is optional for single-role entries.

There's a problem though: sysbench isn't installed on the base Fedora container disk image. The service will fail because `/usr/bin/sysbench` doesn't exist. To fix this, we need a manifest.

## Step 2: Add a Manifest with Packages and Params

Create `~/.virtwork/catalog/sysbench/workload.yaml`:

```yaml
description: "Continuous CPU stress test using sysbench"

packages:
  - sysbench

params:
  - key: threads
    type: int
    default: "4"
    desc: "Number of sysbench worker threads"
  - key: time
    type: int
    default: "0"
    desc: "Test duration in seconds (0 = infinite)"
  - key: cpu-method
    type: string
    default: "sum"
    desc: "CPU test method (sum, sqrt, pi, rand, all)"
```

The manifest declares:

- **`packages`** — Installed via cloud-init before the service starts. `sysbench` is in Fedora's default repos.
- **`params`** — Tunable values that users can override at deploy time. Each param has a key, type, default, and description. The same five types from Go workloads are supported: `string`, `int`, `bool`, `list`, and `dict`. See [development.md](../development.md#configurable-params) for the full type reference.

### Update the service file to use parameters

Replace the hardcoded values with `{{key}}` placeholders:

```ini
[Unit]
Description=Virtwork sysbench CPU stress
After=network-online.target

[Service]
Type=simple
ExecStart=/usr/bin/sysbench cpu --threads={{threads}} --time={{time}} --cpu-method={{cpu-method}} run
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

At deploy time, each `{{key}}` placeholder is replaced with the user-supplied value or the default from the manifest. This is the catalog equivalent of calling `w.GetParam("key")` in a Go workload.

### Validate with defaults

```bash
virtwork run --from-catalog sysbench --dry-run
```

In the cloud-init output, look for:

- `packages:` includes `sysbench`
- The service file content shows `--threads=4 --time=0 --cpu-method=sum` (defaults substituted)
- `runcmd` includes `systemctl daemon-reload` and `systemctl enable --now`

### Validate with overrides

```bash
virtwork run --from-catalog sysbench --dry-run \
  --params sysbench.threads=8,sysbench.time=300
```

Now the service file shows `--threads=8 --time=300 --cpu-method=sum`. The `threads` and `time` params used the override values; `cpu-method` fell back to the default.

Parameters are prefixed with the entry name (`sysbench.threads`, not just `threads`) so they don't collide when multiple workloads run together. Unknown keys are rejected with a "did you mean?" suggestion:

```bash
virtwork run --from-catalog sysbench --params sysbench.thread=8
# Error: unknown param "thread" for workload "sysbench"; did you mean "threads"?
```

## Step 3: Deploy and Verify

This step requires an OpenShift cluster with OpenShift Virtualization.

### Deploy

```bash
virtwork run --from-catalog sysbench \
  --ssh-user virtwork \
  --ssh-key-file ~/.ssh/id_ed25519.pub
```

Behind the scenes, the orchestrator loads your catalog entry, creates a `GenericWorkload` instance (the catalog's runtime type for single-role entries), generates cloud-init from your manifest and params, and creates a VM — the same pipeline as built-in workloads.

### SSH in and verify

```bash
virtctl ssh --ssh-key ~/.ssh/id_ed25519 virtwork@virtwork-sysbench-0 -n virtwork
```

Inside the VM:

```bash
# Verify sysbench is installed
which sysbench

# Check the workload service
systemctl status workload.service

# Watch benchmark output
journalctl -u workload.service -f
```

You should see sysbench CPU output: operations per second, latency percentiles, and thread counts matching your parameters.

### Scaling up

Deploy multiple identical VMs with `--vm-count`:

```bash
virtwork run --from-catalog sysbench --vm-count 3 --dry-run
```

This creates `virtwork-sysbench-0`, `virtwork-sysbench-1`, and `virtwork-sysbench-2` — three VMs running the same workload. Useful for stress-testing cluster capacity across multiple nodes.

### Mixing with built-in workloads

Catalog entries run alongside built-in workloads:

```bash
virtwork run --workloads cpu,memory --from-catalog sysbench --dry-run
```

If `--from-catalog` is set but `--workloads` is not, the default built-in workloads are cleared — only catalog entries run. When both flags are set, everything runs together. A catalog entry cannot shadow a built-in workload name (naming a catalog entry `cpu` produces an error).

### Clean up

```bash
virtwork cleanup
```

## Going Further: Persistent Storage

Some workloads need persistent storage — a database data directory, benchmark result files, or scratch space. Catalog entries declare storage in the manifest, and virtwork handles the rest: creating a CDI DataVolume, attaching it as a virtio disk, and injecting a setup script that waits for the device, formats it with XFS, mounts it, and writes `/etc/fstab` for persistence across reboots.

### Add storage to the sysbench entry

Update `workload.yaml` to include a `storage:` block:

```yaml
description: "Continuous CPU and fileio stress test using sysbench"

packages:
  - sysbench

params:
  - key: threads
    type: int
    default: "4"
    desc: "Number of sysbench worker threads"
  - key: time
    type: int
    default: "0"
    desc: "Test duration in seconds (0 = infinite)"
  - key: cpu-method
    type: string
    default: "sum"
    desc: "CPU test method (sum, sqrt, pi, rand, all)"
  - key: file-total-size
    type: string
    default: "1G"
    desc: "Total size of test files for fileio mode"

storage:
  - name: sysbench-data
    size: 10Gi
    serial: vw-sysbench
    mount: /mnt/data
```

**Storage fields:**

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | DataVolume base name (auto-suffixed with VM name to avoid collisions). Must not be `containerdisk`, `cloudinitdisk`, or `datadisk`. |
| `size` | Yes | Volume size in Kubernetes quantity format (e.g., `10Gi`, `500Mi`) |
| `serial` | Yes | Virtio serial (max 20 chars). The in-VM device appears at `/dev/disk/by-id/virtio-<serial>`, which is deterministic across reboots — unlike `/dev/vdX`. |
| `mount` | Yes | Absolute path mount point inside the VM |

### Update the service to use storage

Replace the service file with one that runs both CPU and fileio benchmarks:

```ini
[Unit]
Description=Virtwork sysbench CPU and fileio stress
After=network-online.target

[Service]
Type=simple
ExecStart=/bin/bash -c '\
  while true; do \
    sysbench cpu --threads={{threads}} --time=60 --cpu-method={{cpu-method}} run; \
    sysbench fileio --file-total-size={{file-total-size}} --file-test-mode=rndrw \
      --threads={{threads}} --time=60 prepare && \
    sysbench fileio --file-total-size={{file-total-size}} --file-test-mode=rndrw \
      --threads={{threads}} --time=60 run; \
    sysbench fileio --file-total-size={{file-total-size}} cleanup; \
    sleep 5; \
  done'
WorkingDirectory=/mnt/data
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

The `WorkingDirectory=/mnt/data` ensures fileio test files land on the persistent volume. The disk setup script runs before the service starts (injected into cloud-init `runcmd` automatically), so `/mnt/data` is mounted by the time systemd starts the workload.

### Validate

```bash
virtwork run --from-catalog sysbench --dry-run
```

In the output, verify:

- A `dataVolumeTemplates` section with size `10Gi`
- An extra disk entry with `serial: vw-sysbench`
- A disk setup script in `write_files` that waits for `/dev/disk/by-id/virtio-vw-sysbench`
- The setup script runs before `systemctl enable --now` in `runcmd`

This is the catalog equivalent of overriding `DataVolumeTemplates()`, `ExtraDisks()`, and `ExtraVolumes()` in a Go workload. The same serial-based discovery pattern used by the built-in `disk`, `database`, and `chaos-disk` workloads is applied automatically. See [development.md](../development.md#storage-backed-workloads) for the Go equivalent.

## Going Further: Multi-Role with Service

Some workloads need multiple VM roles — a server and one or more clients. Catalog entries support this through the `roles:` field in the manifest. When roles are declared, the entry becomes **multi-role**: each role gets its own `.service` file and its own cloud-init, and a Kubernetes Service can route traffic between them.

This section uses a different example — an iperf3 bandwidth test — because it naturally splits into server and client roles. This mirrors the built-in `network` workload but requires no Go code.

### Create the catalog entry

```bash
mkdir -p ~/.virtwork/catalog/my-iperf
```

Create `~/.virtwork/catalog/my-iperf/workload.yaml`:

```yaml
description: "iperf3 bandwidth test (server/client)"

packages:
  - iperf3

params:
  - key: duration
    type: int
    default: "60"
    desc: "Test duration in seconds per iteration"

roles:
  - name: server
    vm-count: 1
  - name: client
    vm-count: 2

service:
  ports:
    - name: iperf
      port: 5201
      protocol: TCP
  selector-role: server
```

**Key differences from single-role:**

- **`roles:`** declares the entry as multi-role. Each role needs a matching `.service` file named `{role}.service`.
- **`vm-count:`** per role. If set to `0` (or omitted), the global `--vm-count` flag is used with a minimum of 1.
- **`service:`** creates a ClusterIP Service so clients can reach the server via DNS.
- **`selector-role: server`** — the Service selects only server VMs (by the `virtwork/role: server` label that the orchestrator applies automatically).

Create `~/.virtwork/catalog/my-iperf/server.service`:

```ini
[Unit]
Description=iperf3 server
After=network-online.target

[Service]
Type=simple
ExecStart=/usr/bin/iperf3 -s
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Create `~/.virtwork/catalog/my-iperf/client.service`:

```ini
[Unit]
Description=iperf3 client
After=network-online.target

[Service]
Type=simple
ExecStart=/bin/bash -c 'while true; do iperf3 -c virtwork-my-iperf -t {{duration}} --forceflush; sleep 10; done'
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

The client reaches the server at `virtwork-my-iperf` — the Service name derived from the entry name (`virtwork-` prefix + entry name). In a real deployment, the full DNS name is `virtwork-my-iperf.<namespace>.svc.cluster.local`, but short names work within the same namespace.

### Validate

```bash
virtwork run --from-catalog my-iperf --dry-run
```

The output shows:

- **3 VM specs** — `virtwork-my-iperf-server-0`, `virtwork-my-iperf-client-0`, `virtwork-my-iperf-client-1`
- **A Service** — `virtwork-my-iperf` with port 5201/TCP, selecting pods with `virtwork/role: server`
- **Per-role cloud-init** — the server VM gets `server.service`, client VMs get `client.service`
- **Parameter substitution** — `{{duration}}` replaced with `60` in the client service

The orchestrator detects multi-role entries and creates a `GenericMultiWorkload` instead of a `GenericWorkload`. It iterates the role distribution, calls `UserdataForRole()` for each role/instance, and creates the Service before the VMs so DNS resolves when clients boot.

See [development.md](../development.md#multi-vm-workloads) for the Go equivalent of this pattern.

## Troubleshooting

### Validation errors

These errors are caught at load time, before any cluster interaction:

| Error | Cause | Fix |
|-------|-------|-----|
| catalog entry not found | Directory doesn't exist | Check path and spelling; verify `--catalog-dir` |
| catalog entry has no .service files | No `*.service` files in directory | Add at least one `.service` file |
| missing .service file for declared role | Multi-role manifest declares a role but no matching `{role}.service` | Create `{role}.service` for every role in the manifest |
| storage name conflicts with reserved disk name | Name is `containerdisk`, `cloudinitdisk`, or `datadisk` | Choose a different name |
| storage size must be a valid quantity | Size like `10` instead of `10Gi` | Use Kubernetes quantity format: `10Gi`, `500Mi`, `1Ti` |
| storage serial must be at most 20 characters | Serial string too long | Shorten to 20 chars or fewer |
| storage mount must be an absolute path | Mount like `mnt/data` | Use absolute path: `/mnt/data` |
| service must declare at least one port | `service:` present but `ports:` empty | Add at least one port or remove the `service:` block |
| service port must be between 1 and 65535 | Port number out of range | Use a valid port number |

### Runtime issues

| Symptom | Likely Cause | Solution |
|---------|-------------|----------|
| Service fails immediately | Package not installed (typo in `packages:`) | SSH in, check `dnf list installed \| grep <pkg>` and `/var/log/cloud-init-output.log` |
| Parameters not substituted | Key mismatch between manifest and service file | Verify `{{key}}` placeholders match `params[].key` exactly |
| Storage not mounted | Disk setup script failed | SSH in, check `/dev/disk/by-id/` for serial symlink; check `dmesg \| tail` |
| Client can't reach server | Service not created or DNS not resolving | Check `oc get svc -n <namespace>`; verify `selector-role` matches a declared role |

### Debugging with dry run

Always validate with `--dry-run` before deploying. Check:

1. `packages:` list is correct
2. Service file content has substituted values (no `{{key}}` remaining)
3. Storage entries produce `dataVolumeTemplates`, extra disks with serials, and setup scripts
4. Multi-role entries produce separate VM specs per role with the correct service file

## Checklist

Before deploying a catalog workload, verify:

**Single-role entry:**

- [ ] Directory exists under `--catalog-dir` (default: `~/.virtwork/catalog/`)
- [ ] At least one `*.service` file with valid systemd syntax
- [ ] Packages in `workload.yaml` exist in Fedora's default repos
- [ ] Param keys in manifest match `{{key}}` placeholders in service files exactly
- [ ] `--dry-run` produces valid cloud-init with correct parameter substitution
- [ ] Deployed and verified: workload service is active (`systemctl status`)

**If using storage:**

- [ ] `name` is not a reserved disk name (`containerdisk`, `cloudinitdisk`, `datadisk`)
- [ ] `size` is a valid Kubernetes quantity (`10Gi`, `500Mi`)
- [ ] `serial` is 1-20 characters
- [ ] `mount` is an absolute path (`/mnt/data`, not `mnt/data`)
- [ ] Service file's `WorkingDirectory` or paths reference the mount point

**If multi-role:**

- [ ] `roles:` declared in manifest with at least 2 roles
- [ ] Each role has a matching `{role}.service` file (not `workload.service`)
- [ ] `service.selector-role` matches a declared role name
- [ ] Client service file uses the correct Service DNS name (`virtwork-<entry-name>`)
- [ ] `--dry-run` shows separate VM specs per role and the Service object

**If mixing with built-in workloads:**

- [ ] Entry name does not shadow a built-in workload name (`cpu`, `memory`, `disk`, etc.)
- [ ] `--params` keys are prefixed with the entry name (`myentry.key=value`)

---

## What's Next

- [development.md](../development.md#catalog-workloads) — Full catalog schema reference (workload.yaml fields, param types, storage/service details)
- [configuration.md](../configuration.md) — Complete CLI flag and environment variable reference
- [Tutorial 03](03-adding-a-workload.md) — Build a workload in Go when you need custom logic beyond systemd units
- [Demo: Deploying Workloads](02-deploying-workloads.md) — Hands-on deployment scenarios with built-in workloads
