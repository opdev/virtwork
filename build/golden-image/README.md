# Golden Container Disk Image

This directory contains the Containerfile and build scripts for the virtwork golden container disk image.

## Overview

The golden image is based on `quay.io/containerdisks/fedora:41` and includes all tools needed for virtwork workloads pre-installed. This eliminates package installation at VM boot time, reducing startup time by 2-5 minutes.

### Pre-installed Tools

- **stress-ng** — CPU and memory stress testing (cpu, memory workloads)
- **fio** — Flexible I/O tester for disk benchmarking (disk workload)
- **iperf3** — Network performance testing (network workload)
- **netperf** — TCP_RR transaction performance (tps workload)
- **python3** — HTTP file server for the tps workload's application-layer transfers
- **postgresql-server** — PostgreSQL database with pgbench (database workload)
- **procps-ng** — `ps`, `pkill`, `kill` for chaos-process
- **iproute-tc** — Traffic control (`tc`) and the `sch_netem` kernel module hooks used by the chaos-network workload to inject latency and packet loss
- **iptables-nft** — Firewall rules; reserved for future network partition / blackhole scenarios

Additional tools like `fallocate` and `dd` (used by chaos-disk) are already present in the base Fedora image.

Workloads that need persistent storage (disk, database, chaos-disk) discover their data volume through `/dev/disk/by-id/virtio-<serial>` using the shared `diskSetupScript` helper — they do not depend on any tools beyond the standard userspace utilities already in the base image (`blkid`, `mkfs.xfs`, `mount`, `readlink`).

## Building

### Prerequisites

- Podman or Docker installed
- Network access to pull `quay.io/containerdisks/fedora:41`

### Build Locally

```bash
./build.sh
```

This builds the image as `quay.io/opdev/virtwork-disk:latest` (local copy, not pushed to registry).

### Build and Push

```bash
PUSH=true ./build.sh
```

This builds and pushes to `quay.io/opdev/virtwork-disk:latest`. Requires authentication to the registry.

### Build with Custom Registry

```bash
REGISTRY=my.registry.io ./build.sh
```

### Build with Custom Tag

```bash
TAG=1.0.0 ./build.sh
```

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `REGISTRY` | Container registry hostname | `quay.io/opdev` |
| `IMAGE_NAME` | Image name | `virtwork-disk` |
| `TAG` | Image tag | `latest` |
| `PUSH` | Push to registry after building | `false` |

## Testing

After building, verify tools are present:

```bash
podman run --rm quay.io/opdev/virtwork-disk:latest /bin/bash -c "stress-ng --version"
podman run --rm quay.io/opdev/virtwork-disk:latest /bin/bash -c "fio --version"
podman run --rm quay.io/opdev/virtwork-disk:latest /bin/bash -c "iperf3 --version"
podman run --rm quay.io/opdev/virtwork-disk:latest /bin/bash -c "psql --version"
```

## Using the Golden Image

The golden image is **optional**. To use it with virtwork:

### CLI Flag

```bash
virtwork run --container-disk-image quay.io/opdev/virtwork-disk:latest
```

### Environment Variable

```bash
export VIRTWORK_CONTAINER_DISK_IMAGE=quay.io/opdev/virtwork-disk:latest
virtwork run
```

### Config File

```yaml
# config.yaml
container_disk_image: quay.io/opdev/virtwork-disk:latest
```

```bash
virtwork run --config config.yaml
```

## Image Size

The golden image adds approximately **34MB** to the base Fedora 41 container disk image:

- Base Fedora 41 container disk: ~600MB
- Golden image: ~634MB

## Design Decisions

### Why Keep Package Installation in Cloud-Init?

DNF is **idempotent** — when it tries to install a package that's already present, it completes instantly with "Package X is already installed" messages.

This means:
- **Golden image users**: Get instant package "installation" (already there) — saves 2-5 minutes
- **Default Fedora users**: Get normal package installation at boot
- **Same workload code**: Works for both images without conditional logic

### Why Not Remove Packages from Cloud-Init?

Removing package installation from workload code would require:
- Changes to all 5 workload implementations
- Changes to all workload tests
- Conditional logic to detect which image is in use
- Maintaining two code paths

The current approach is simpler and maintains backward compatibility.

## Future Enhancements

1. **Multi-architecture support**: Build for arm64 in addition to amd64
2. **Automated builds**: CI workflow on schedule
3. **Image scanning**: Add Trivy security scanning
4. **Semantic versioning**: Pin specific package versions for reproducibility
5. **Image variants**: Create minimal/full variants for different use cases

## Related Docs

- [docs/chaos-workloads.md](../../docs/chaos-workloads.md) — operator guide for the chaos workloads that use `iproute-tc`, `procps-ng`, and `fallocate`
- [docs/deployment.md](../../docs/deployment.md) — how to set the golden image as the default container disk in your deployment
- [docs/configuration.md](../../docs/configuration.md) — `--container-disk-image` flag, `VIRTWORK_CONTAINER_DISK_IMAGE` env var, and `container_disk_image` YAML key

## License

Apache License 2.0. See [LICENSE](../../LICENSE).
