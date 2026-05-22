// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/opdev/virtwork/internal/cloudinit"
	"github.com/opdev/virtwork/internal/config"
)

// CloudConfigOpts is re-exported from cloudinit for convenience.
type CloudConfigOpts = cloudinit.CloudConfigOpts

// WriteFile is re-exported from cloudinit for convenience.
type WriteFile = cloudinit.WriteFile

// Workload defines the contract for all workload types.
// Implementations are pure data producers — no I/O, no goroutines.
type Workload interface {
	// Name returns the workload identifier (e.g., "cpu", "memory", "disk").
	Name() string

	// CloudInitUserdata returns the cloud-init YAML for this workload.
	CloudInitUserdata() (string, error)

	// VMResources returns the CPU and memory requirements for each VM.
	VMResources() VMResourceSpec

	// ExtraVolumes returns additional KubeVirt volumes beyond the base containerDisk
	// and cloudInitNoCloud volumes. Returns nil if none needed.
	ExtraVolumes() []kubevirtv1.Volume

	// ExtraDisks returns additional KubeVirt disks beyond the base containerDisk
	// and cloudInitNoCloud disks. Returns nil if none needed.
	ExtraDisks() []kubevirtv1.Disk

	// DataVolumeTemplates returns CDI DataVolumeTemplateSpecs for persistent storage.
	// Returns nil if no data volumes needed.
	DataVolumeTemplates() []kubevirtv1.DataVolumeTemplateSpec

	// RequiresService returns true if this workload needs a K8s Service.
	RequiresService() bool

	// ServiceSpec returns the K8s Service definition, or nil if not needed.
	ServiceSpec() *corev1.Service

	// VMCount returns the number of VMs this workload requires.
	VMCount() int
}

// MultiVMWorkload extends Workload for workloads that need per-role userdata.
// The orchestration layer checks VMCount() > 1 and type-asserts to this
// interface to call UserdataForRole() for each VM role.
type MultiVMWorkload interface {
	Workload
	Roles() []string
	UserdataForRole(role string, namespace string) (string, error)
}

// VMResourceSpec holds CPU and memory requirements for a VM.
type VMResourceSpec struct {
	CPUCores int
	Memory   string
}

// BaseWorkload provides default implementations for optional Workload methods.
// Embed this struct in concrete workloads to inherit sensible defaults.
type BaseWorkload struct {
	Config            config.WorkloadConfig
	SSHUser           string
	SSHPassword       string
	SSHAuthorizedKeys []string
}

// VMResources returns the CPU and memory spec from the workload config.
func (b *BaseWorkload) VMResources() VMResourceSpec {
	return VMResourceSpec{
		CPUCores: b.Config.CPUCores,
		Memory:   b.Config.Memory,
	}
}

// ExtraVolumes returns nil — no additional volumes by default.
func (b *BaseWorkload) ExtraVolumes() []kubevirtv1.Volume {
	return nil
}

// ExtraDisks returns nil — no additional disks by default.
func (b *BaseWorkload) ExtraDisks() []kubevirtv1.Disk {
	return nil
}

// DataVolumeTemplates returns nil — no data volumes by default.
func (b *BaseWorkload) DataVolumeTemplates() []kubevirtv1.DataVolumeTemplateSpec {
	return nil
}

// RequiresService returns false — no K8s Service by default.
func (b *BaseWorkload) RequiresService() bool {
	return false
}

// ServiceSpec returns nil — no Service definition by default.
func (b *BaseWorkload) ServiceSpec() *corev1.Service {
	return nil
}

// VMCount returns the configured VM count, defaulting to 1.
func (b *BaseWorkload) VMCount() int {
	if b.Config.VMCount < 1 {
		return 1
	}
	return b.Config.VMCount
}

func diskSetupScript(serial, mountPoint string) string {
	return fmt.Sprintf(`#!/bin/bash
set -euo pipefail

DISK="/dev/disk/by-id/virtio-%s"
MOUNT_POINT="%s"

# Wait for the disk symlink to appear (udev may be slow)
for i in $(seq 1 30); do
    [ -e "${DISK}" ] && break
    sleep 1
done

if [ ! -e "${DISK}" ]; then
    echo "ERROR: disk ${DISK} not found after 30s" >&2
    exit 1
fi

# Resolve the symlink to the real block device
REAL_DEV=$(readlink -f "${DISK}")

mkdir -p "${MOUNT_POINT}"

# Format only if no filesystem exists
if ! blkid -o value -s TYPE "${REAL_DEV}" > /dev/null 2>&1; then
    mkfs.xfs "${REAL_DEV}"
fi

# Mount if not already mounted
if ! mountpoint -q "${MOUNT_POINT}"; then
    mount "${REAL_DEV}" "${MOUNT_POINT}"
fi

# Ensure fstab entry for persistence across reboots
if ! grep -q "${MOUNT_POINT}" /etc/fstab; then
    echo "${REAL_DEV} ${MOUNT_POINT} xfs defaults,nofail 0 0" >> /etc/fstab
fi
`, serial, mountPoint)
}

// BuildCloudConfig injects SSH credentials into the given options and delegates
// to cloudinit.BuildCloudConfig. Workloads should call this instead of the
// package-level function to ensure consistent SSH credential handling.
func (b *BaseWorkload) BuildCloudConfig(opts CloudConfigOpts) (string, error) {
	opts.SSHUser = b.SSHUser
	opts.SSHPassword = b.SSHPassword
	opts.SSHAuthorizedKeys = b.SSHAuthorizedKeys
	return cloudinit.BuildCloudConfig(opts)
}
