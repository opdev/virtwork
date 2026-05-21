// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads

import (
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/vm"
)

const chaosDiskScript = `#!/bin/bash
set -euo pipefail

MOUNT_POINT="${CHAOS_DISK_MOUNT:-/mnt/data}"
FILL_PERCENT="${CHAOS_DISK_FILL_PERCENT:-90}"
FILL_FILE="${MOUNT_POINT}/chaos-disk-fill"
RELEASE_SLEEP="${CHAOS_DISK_RELEASE_SLEEP:-30}"
FILL_SLEEP="${CHAOS_DISK_FILL_SLEEP:-60}"

while true; do
    TOTAL_KB=$(df -k "${MOUNT_POINT}" | awk 'NR==2 {print $2}')
    USED_KB=$(df -k "${MOUNT_POINT}" | awk 'NR==2 {print $3}')
    TARGET_KB=$(( TOTAL_KB * FILL_PERCENT / 100 ))
    FILL_KB=$(( TARGET_KB - USED_KB ))

    if [ "${FILL_KB}" -gt 0 ]; then
        fallocate -l "${FILL_KB}K" "${FILL_FILE}" 2>/dev/null || \
            dd if=/dev/zero of="${FILL_FILE}" bs=1K count="${FILL_KB}" status=none
    fi

    sleep "${FILL_SLEEP}"

    rm -f "${FILL_FILE}"
    sleep "${RELEASE_SLEEP}"
done
`

const chaosDiskSystemdUnit = `[Unit]
Description=Virtwork chaos-disk fill/release workload
After=network.target local-fs.target

[Service]
Type=simple
ExecStart=/usr/local/bin/chaos-disk.sh
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
`

// ChaosDiskWorkload generates cloud-init userdata for a disk fill/release chaos
// workload using fallocate and dd.
type ChaosDiskWorkload struct {
	BaseWorkload
	DataDiskSize string
}

// NewChaosDiskWorkload creates a ChaosDiskWorkload with the given configuration,
// disk size, and SSH credentials.
func NewChaosDiskWorkload(
	cfg config.WorkloadConfig,
	dataDiskSize, sshUser, sshPassword string,
	sshKeys []string,
) *ChaosDiskWorkload {
	return &ChaosDiskWorkload{
		BaseWorkload: BaseWorkload{
			Config:            cfg,
			SSHUser:           sshUser,
			SSHPassword:       sshPassword,
			SSHAuthorizedKeys: sshKeys,
		},
		DataDiskSize: dataDiskSize,
	}
}

// Name returns "chaos-disk".
func (w *ChaosDiskWorkload) Name() string {
	return "chaos-disk"
}

// CloudInitUserdata returns cloud-init YAML that writes a fill/release script
// and a systemd service that runs it in a loop.
func (w *ChaosDiskWorkload) CloudInitUserdata() (string, error) {
	return w.BuildCloudConfig(CloudConfigOpts{
		WriteFiles: []WriteFile{
			{
				Path:        "/usr/local/bin/chaos-disk.sh",
				Content:     chaosDiskScript,
				Permissions: "0755",
			},
			{
				Path:        "/etc/systemd/system/virtwork-chaos-disk.service",
				Content:     chaosDiskSystemdUnit,
				Permissions: "0644",
			},
		},
		RunCmd: [][]string{
			{"mkdir", "-p", "/mnt/data"},
			{"systemctl", "daemon-reload"},
			{"systemctl", "enable", "--now", "virtwork-chaos-disk.service"},
		},
	})
}

// DataVolumeTemplates returns a DataVolumeTemplateSpec for the data disk.
func (w *ChaosDiskWorkload) DataVolumeTemplates() []kubevirtv1.DataVolumeTemplateSpec {
	return []kubevirtv1.DataVolumeTemplateSpec{
		vm.BuildDataVolumeTemplate("virtwork-chaos-disk-data", w.DataDiskSize),
	}
}

// ExtraDisks returns the data disk definition.
func (w *ChaosDiskWorkload) ExtraDisks() []kubevirtv1.Disk {
	return []kubevirtv1.Disk{
		{
			Name: "datadisk",
			DiskDevice: kubevirtv1.DiskDevice{
				Disk: &kubevirtv1.DiskTarget{
					Bus: "virtio",
				},
			},
		},
	}
}

// ExtraVolumes returns the data volume sourced from the DataVolume.
func (w *ChaosDiskWorkload) ExtraVolumes() []kubevirtv1.Volume {
	return []kubevirtv1.Volume{
		{
			Name: "datadisk",
			VolumeSource: kubevirtv1.VolumeSource{
				DataVolume: &kubevirtv1.DataVolumeSource{
					Name: "virtwork-chaos-disk-data",
				},
			},
		},
	}
}
