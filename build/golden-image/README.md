# Golden Container Disk Image

This directory produces a KubeVirt containerdisk OCI image with all
virtwork workload tools pre-installed, eliminating package installation
at VM boot time.

## How It Works

```
blueprint.toml          image-builder          Containerfile
  (packages)  ──────►  build qcow2  ──────►  FROM scratch
                        (Fedora 41)           COPY qcow2 → /disk/
                                                    │
                                              containerdisk
                                               OCI image
```

1. **`blueprint.toml`** — declarative package manifest consumed by osbuild.
2. **`image-builder build generic-qcow2`** — produces a Fedora qcow2 with all
   blueprint packages baked in.
3. **`podman build`** — wraps the qcow2 in a minimal `FROM scratch` OCI
   image at `/disk/disk.qcow2` (KubeVirt containerdisk format).

## Prerequisites

| Tool | Install | Purpose |
|------|---------|---------|
| `image-builder` | `sudo dnf install image-builder` | Builds the qcow2 from the blueprint |
| `podman` | `sudo dnf install podman` | Packages the qcow2 as an OCI image |

`image-builder` requires root for loopback device access — `build.sh`
re-executes itself with `sudo` automatically if needed.

## Pre-installed Tools

- **stress-ng** — CPU and memory stress testing (cpu, memory workloads)
- **fio** — Flexible I/O tester for disk benchmarking (disk workload)
- **iperf3** — Network performance testing (network workload)
- **netperf** — TCP_RR transaction performance (tps workload)
- **python3** — HTTP file server for the tps workload
- **postgresql-server** — PostgreSQL database with pgbench (database workload)
- **iproute-tc** — Traffic control for chaos-network latency/loss injection
- **kernel-modules-extra** — `sch_netem` kernel module for chaos-network
- **iptables-nft** — Firewall rules for future network partition scenarios
- **cloud-init** — VM first-boot configuration
- **qemu-guest-agent** — KubeVirt guest agent communication

## Building

### Build Locally

```bash
./build.sh
```

Produces `quay.io/opdev/virtwork-disk:latest` (local only, not pushed).

### Build and Push

```bash
PUSH=true ./build.sh
```

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `REGISTRY` | Container registry hostname | `quay.io/opdev` |
| `IMAGE_NAME` | Image name | `virtwork-disk` |
| `TAG` | Image tag | `latest` |
| `DISTRO` | Fedora version for image-builder | `fedora-42` |
| `IMAGE_TYPE` | osbuild image type | `generic-qcow2` |
| `PUSH` | Push to registry after building | `false` |

## Adding Packages

Edit `blueprint.toml` and add an entry to the `packages` list:

```toml
packages = [
  # ... existing packages ...
  { name = "your-new-package" },
]
```

Then rebuild with `./build.sh`. osbuild validates that packages exist in
the Fedora repos at build time — a typo or missing package fails the
build immediately.

## Using the Golden Image

The golden image is **optional**. To use it with virtwork:

```bash
# CLI flag
virtwork run --container-disk-image quay.io/opdev/virtwork-disk:latest

# Environment variable
export VIRTWORK_CONTAINER_DISK_IMAGE=quay.io/opdev/virtwork-disk:latest
virtwork run

# Config file (config.yaml)
container_disk_image: quay.io/opdev/virtwork-disk:latest
```

## Troubleshooting

| Problem | Fix |
|---------|-----|
| `image-builder: command not found` | `sudo dnf install image-builder` |
| Permission errors during build | Script auto-escalates to root; ensure `sudo` works |
| Package not found in blueprint | Check the package name exists in `dnf search <name>` for the target Fedora version |
| Build runs out of disk space | qcow2 assembly needs ~4 GB in `/tmp`; set `TMPDIR` to a larger partition if needed |

## Related Docs

- [docs/chaos-workloads.md](../../docs/chaos-workloads.md) — chaos workloads using `iproute-tc`, `procps-ng`, and `fallocate`
- [docs/deployment.md](../../docs/deployment.md) — setting the golden image as default container disk
- [docs/configuration.md](../../docs/configuration.md) — `--container-disk-image` flag, env var, and YAML key

## License

Apache License 2.0. See [LICENSE](../../LICENSE).
