# Demo: Deploying Workloads

This guide walks through nine realistic scenarios, from previewing what virtwork would create to SSH debugging inside a running VM. Each scenario shows the commands and the expected output.

## Prerequisites

- An OpenShift cluster with [OpenShift Virtualization](https://docs.openshift.com/container-platform/latest/virt/about_virt/about-virt.html) (CNV) installed
- `oc` CLI logged in with permissions to create/delete namespaces, VMs, services, and secrets
- virtwork binary built:

```bash
go build -o virtwork ./cmd/virtwork
```

Or use `go run ./cmd/virtwork` in place of `virtwork` for all commands below.

See the [README](../../README.md#prerequisites) for full prerequisites.

---

## Scenario 1: Preview with Dry Run

Dry run prints the Kubernetes manifests that virtwork *would* create, without touching the cluster. No cluster connection is needed.

### Single workload preview

```bash
virtwork run --dry-run --workloads cpu
```

Expected output:

```
--- Dry Run ---
Total VMs to create: 1

# VM: virtwork-cpu-0 (workload: cpu)
apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  labels:
    app.kubernetes.io/component: cpu
    app.kubernetes.io/managed-by: virtwork
    app.kubernetes.io/name: virtwork-cpu
    virtwork/run-id: <uuid>
  name: virtwork-cpu-0
  namespace: virtwork
spec:
  running: true
  template:
    metadata:
      labels:
        app.kubernetes.io/component: cpu
        app.kubernetes.io/managed-by: virtwork
        app.kubernetes.io/name: virtwork-cpu
        virtwork/run-id: <uuid>
    spec:
      domain:
        devices:
          disks:
          - disk:
              bus: virtio
            name: containerdisk
          - disk:
              bus: virtio
            name: cloudinitdisk
          interfaces:
          - masquerade: {}
            name: default
        resources:
          requests:
            cpu: "2"
            memory: 2Gi
      networks:
      - name: default
        pod: {}
      volumes:
      - containerDisk:
          image: quay.io/containerdisks/fedora:41
        name: containerdisk
      - cloudInitNoCloud:
          userData: |
            #cloud-config
            packages:
              - stress-ng
            write_files:
              ...
        name: cloudinitdisk
---
```

Key things to notice:
- **`running: true`** — The VM starts automatically on creation
- **`masquerade`** — Pod-level NAT networking (standard for KubeVirt)
- **`containerDisk`** — The OS image, pulled from a container registry
- **`cloudInitNoCloud`** — The cloud-init YAML that installs and starts the workload
- **Labels** — `managed-by: virtwork` and `run-id` for cleanup tracking

### Multiple workloads

```bash
virtwork run --dry-run --workloads cpu,disk
```

This outputs two VM specs. Notice that the disk VM has additional entries:
- A `datadisk` in the `disks` list
- A corresponding `datadisk` in the `volumes` list
- A `dataVolumeTemplates` section for persistent storage provisioning

### Custom resources

```bash
virtwork run --dry-run --workloads cpu --cpu-cores 4 --memory 4Gi
```

The `resources.requests` section in the output now shows `cpu: "4"` and `memory: 4Gi`.

---

## Scenario 2: Deploy a Single CPU Workload

### Deploy

```bash
virtwork run --workloads cpu --vm-count 1
```

Expected output:

```
Namespace virtwork ensured
Secret virtwork-cpu-0-cloudinit created
VM virtwork-cpu-0 created
Waiting for 1 VMs to become ready (timeout: 10m0s)...
All 1 VMs ready
==================================================
Deployment Summary
==================================================
Run ID:       a1b2c3d4-e5f6-7890-abcd-ef0123456789
Namespace:    virtwork
VMs created:  1
Services:     0
Secrets:      1
Image:        quay.io/containerdisks/fedora:41
==================================================
```

Save the **Run ID** — you'll need it if you want to clean up just this run later.

### Verify on the cluster

```bash
oc get vm -n virtwork
```

```
NAME              AGE    STATUS    READY
virtwork-cpu-0    2m     Running   True
```

```bash
oc get vmi -n virtwork
```

```
NAME              AGE    PHASE     IP             NODENAME
virtwork-cpu-0    2m     Running   10.128.2.15    worker-0
```

```bash
oc get secret -n virtwork -l app.kubernetes.io/managed-by=virtwork
```

```
NAME                          TYPE     DATA   AGE
virtwork-cpu-0-cloudinit      Opaque   1      2m
```

---

## Scenario 3: Deploy Multiple Workloads

### Three workloads together

```bash
virtwork run --workloads cpu,memory,disk
```

This creates 3 VMs, 3 cloud-init secrets, and 0 services:

| VM Name | Workload | Extra Resources |
|---------|----------|-----------------|
| `virtwork-cpu-0` | CPU stress | None |
| `virtwork-memory-0` | Memory pressure | None |
| `virtwork-disk-0` | Disk I/O | DataVolume for persistent storage |

### The full suite

```bash
virtwork run
```

With no `--workloads` flag, all nine workload types deploy. This creates **11 VMs** total — seven single-VM workloads plus the network and tps workloads, which each create a server/client pair:

| VM Name | Workload | Role |
|---------|----------|------|
| `virtwork-cpu-0` | CPU | — |
| `virtwork-memory-0` | Memory | — |
| `virtwork-disk-0` | Disk | — |
| `virtwork-database-0` | Database | — |
| `virtwork-network-server-0` | Network | server |
| `virtwork-network-client-0` | Network | client |
| `virtwork-tps-server-0` | TPS | server |
| `virtwork-tps-client-0` | TPS | client |
| `virtwork-chaos-disk-0` | Chaos-disk | — |
| `virtwork-chaos-network-0` | Chaos-network | — |
| `virtwork-chaos-process-0` | Chaos-process | — |

The network workload creates a `virtwork-iperf3-server` ClusterIP Service on port 5201. The tps workload creates a `virtwork-tps-server` ClusterIP Service on ports 12865 (netperf control), 12866 (netperf data), and 8080 (HTTP file transfer). The chaos workloads do not create Services — their behavior is confined to the VM they run in.

> ⚠️ The chaos workloads inject destructive behavior inside their VMs (disk fill, network latency/loss, random process kills). They are safe to run in an isolated namespace but should never share a namespace with workloads you care about. See [chaos-workloads.md](../chaos-workloads.md) for operational guidance.

### Scaling up

```bash
virtwork run --workloads cpu,memory --vm-count 3
```

Creates 6 VMs: `virtwork-cpu-0` through `virtwork-cpu-2`, and `virtwork-memory-0` through `virtwork-memory-2`.

---

## Scenario 4: SSH into a Running VM

SSH access is useful for debugging — inspecting workload logs, checking resource usage, or verifying cloud-init ran correctly.

### Deploy with SSH access

```bash
virtwork run --workloads cpu \
  --ssh-user virtwork \
  --ssh-key-file ~/.ssh/id_ed25519.pub
```

This adds a `users` section to the cloud-init config, creating a `virtwork` user with your SSH public key.

### Connect via virtctl

[virtctl](https://kubevirt.io/user-guide/user_workloads/virtctl_client_tool/) is the KubeVirt CLI tool for interacting with VMs:

```bash
virtctl ssh --ssh-key ~/.ssh/id_ed25519 virtwork@virtwork-cpu-0 -n virtwork
```

### Connect via port-forward

If you don't have virtctl:

```bash
oc port-forward -n virtwork vmi/virtwork-cpu-0 2222:22 &
ssh -o StrictHostKeyChecking=no -p 2222 virtwork@localhost
```

### Inspect the running workload

Once inside the VM:

```bash
# Check the workload service status
systemctl status virtwork-cpu.service
```

```
● virtwork-cpu.service - Virtwork CPU stress workload
     Loaded: loaded (/etc/systemd/system/virtwork-cpu.service; enabled)
     Active: active (running) since Mon 2026-02-09 14:30:00 UTC; 5min ago
   Main PID: 1234 (stress-ng)
      Tasks: 5 (limit: 4915)
     Memory: 12.3M
        CPU: 4min 58.123s
```

```bash
# Follow workload logs
journalctl -u virtwork-cpu.service -f
```

```bash
# See CPU usage
top
```

You should see `stress-ng` consuming all available CPU cores.

```bash
# Check cloud-init completed successfully
cloud-init status
```

```
status: done
```

If cloud-init failed, check the logs:

```bash
cat /var/log/cloud-init-output.log
```

---

## Scenario 5: Inspect the Audit Trail

Every `virtwork run` and `virtwork cleanup` execution is recorded in a local SQLite database (`virtwork.db` by default). The audit trail is useful for tracking what was deployed, when, and with what configuration.

### View recent executions

```bash
sqlite3 virtwork.db "SELECT id, run_id, command, status, started_at FROM audit_log ORDER BY id DESC LIMIT 5;"
```

```
3|f1e2d3c4-...|cleanup|success|2026-02-09T15:00:00Z
2|a1b2c3d4-...|run|success|2026-02-09T14:30:00Z
1|98765432-...|dry-run|success|2026-02-09T14:25:00Z
```

### View VMs from a specific run

```bash
sqlite3 virtwork.db "SELECT vm_name, component, cpu_cores, memory FROM vm_details WHERE audit_id = 2;"
```

```
virtwork-cpu-0|cpu|2|2Gi
```

### View the events timeline

```bash
sqlite3 virtwork.db "SELECT event_type, message, occurred_at FROM events WHERE audit_id = 2 ORDER BY occurred_at;"
```

```
execution_started|Starting run with run-id a1b2c3d4-...|2026-02-09T14:30:00Z
execution_started|Planned 1 VMs across 1 workloads|2026-02-09T14:30:00Z
service_created|...|...
vm_created|VM virtwork-cpu-0 created|2026-02-09T14:30:05Z
vm_ready|VM virtwork-cpu-0 is ready|2026-02-09T14:31:30Z
```

See the [README audit section](../../README.md#audit-tracking) for the full schema and more query examples.

---

## Scenario 6: Clean Up

### Clean up all resources

```bash
virtwork cleanup
```

```
Cleanup complete: 1 VMs deleted, 0 services deleted, 1 secrets deleted
```

This deletes every resource with the `app.kubernetes.io/managed-by: virtwork` label in the `virtwork` namespace.

### Targeted cleanup by run ID

If you ran virtwork multiple times and want to clean up only one run:

```bash
virtwork cleanup --run-id a1b2c3d4-e5f6-7890-abcd-ef0123456789
```

Only resources with the matching `virtwork/run-id` label are deleted. Other runs remain untouched.

### Clean up including the namespace

```bash
virtwork cleanup --delete-namespace
```

Deletes all managed resources and then removes the namespace itself.

### Verify cleanup

```bash
oc get vm -n virtwork
```

```
No resources found in virtwork namespace.
```

Cleanup is **error-tolerant** — if one VM fails to delete, the others are still cleaned up. Warnings are printed at the end:

```
Cleanup complete: 5 VMs deleted, 1 services deleted, 6 secrets deleted
Warnings (1):
  - failed to delete VM virtwork-cpu-0: context deadline exceeded
```

---

## Scenario 7: Deploy from Inside the Cluster

Virtwork can run as a pod on the cluster using the provided Kustomize manifests. This is useful for automated or scheduled deployments.

### Deploy the manifests

```bash
oc apply -k deploy/
```

This creates the `virtwork` namespace, a ServiceAccount with RBAC, a ConfigMap, a Secret, a PVC for the audit database, and a Deployment. See the [README OpenShift deployment section](../../README.md#openshift-deployment) for details.

### Run commands interactively

By default, the pod sleeps and waits for commands:

```bash
# Preview
oc exec -it deploy/virtwork -- virtwork run --dry-run

# Deploy
oc exec -it deploy/virtwork -- virtwork run --workloads cpu,memory

# Cleanup
oc exec -it deploy/virtwork -- virtwork cleanup
```

### Auto-run on pod start

Set the `VIRTWORK_COMMAND` and `VIRTWORK_ARGS` environment variables in the Deployment to have the pod execute virtwork automatically:

```bash
oc set env deploy/virtwork VIRTWORK_COMMAND=run VIRTWORK_ARGS="--workloads cpu,memory --vm-count 2"
```

See [deployment.md](../deployment.md) for a manifest-by-manifest deep dive on the Kustomize layout.

---

## Scenario 8: Inject Network Chaos

The chaos-network workload uses `tc` and `netem` to add latency and packet loss to the VM's egress interface — useful for testing how an application or monitoring stack reacts to a degraded network.

```bash
virtwork run --workloads chaos-network --ssh-user virtwork --ssh-key-file ~/.ssh/id_ed25519.pub
```

After the VM is ready, SSH in and confirm the qdisc is applied:

```bash
virtctl ssh --ssh-key ~/.ssh/id_ed25519 virtwork@virtwork-chaos-network-0 -n virtwork

# Inside the VM:
tc qdisc show dev eth0
```

You should see a `netem` qdisc with `delay 100ms` and `loss 5%` on the default-route interface. Verify with a ping to a known host:

```bash
ping -c 5 1.1.1.1
```

Expect ~100ms RTT and occasional packet loss in the output. The qdisc is managed by the `virtwork-chaos-network.service` systemd unit, so it will be reapplied on reboot.

For chaos-disk and chaos-process, plus the full risk model, see [chaos-workloads.md](../chaos-workloads.md).

---

## Scenario 9: Transaction Performance with tps

The tps workload measures transactions per second using two complementary tests on a server/client pair: `netperf TCP_RR` (small-packet round-trips at the transport layer) and an HTTP file-transfer loop (larger transfers at the application layer).

```bash
virtwork run --workloads tps --ssh-user virtwork --ssh-key-file ~/.ssh/id_ed25519.pub
```

Two VMs are created — `virtwork-tps-server-0` (runs `netserver` and a Python HTTP server) and `virtwork-tps-client-0` (loops through netperf and curl tests against the server's Service DNS name) — plus the `virtwork-tps-server` ClusterIP Service on ports 12865, 12866, and 8080.

Watch the client's output:

```bash
virtctl ssh --ssh-key ~/.ssh/id_ed25519 virtwork@virtwork-tps-client-0 -n virtwork

# Inside the VM:
journalctl -u virtwork-tps.service -f
```

You will see alternating TCP_RR iteration blocks (transactions/sec) and HTTP iteration blocks (transfers/sec, MB/s).

The defaults are 30 iterations × 60 seconds each, 10MB transfer file. To customize, set `workloads.tps.params` in a YAML config:

```yaml
workloads:
  tps:
    params:
      file-size: "50M"
      iterations: "10"
      duration: "30"
```

Or equivalently via the `--params` CLI flag (no config file needed):

```bash
virtwork run --workloads tps --params tps.file-size=50M,tps.iterations=10,tps.duration=30
```

---

## Troubleshooting

| Symptom | Likely Cause | Solution |
|---------|-------------|----------|
| `connecting to cluster` error | No kubeconfig available | Set `KUBECONFIG` env var or pass `--kubeconfig` |
| VMs stuck in `Scheduling` | Insufficient cluster resources | Reduce `--cpu-cores` or `--memory`, or add nodes |
| VMs stuck in `Provisioning` | CDI not installed or no default StorageClass | Install the OpenShift Virtualization operator and ensure a default StorageClass exists |
| Timeout waiting for readiness | Slow image pull or boot | Increase `--timeout` (default is 600s), or use `--no-wait` and check manually |
| Cloud-init failed inside VM | Package install failure or script error | SSH in and check `/var/log/cloud-init-output.log` |
| chaos-network: `tc` does nothing inside the VM | `sch_netem` kernel module not loaded | Confirm the VM image has `iproute-tc` and `kernel-modules-extra` available; check `journalctl -u virtwork-chaos-network.service` |
| `unknown workload` error | Typo in `--workloads` | Available workloads: `cpu`, `memory`, `disk`, `database`, `network`, `tps`, `chaos-disk`, `chaos-network`, `chaos-process` |
