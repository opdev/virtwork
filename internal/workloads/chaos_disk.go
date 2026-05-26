// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads

import (
	"fmt"

	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/vm"
)

const chaosDiskScriptTemplate = `#!/bin/bash
set -euo pipefail

MOUNT_POINT="%s"
FILL_PERCENT="%s"
FILL_FILE="${MOUNT_POINT}/chaos-disk-fill"
RELEASE_SLEEP="%s"
FILL_SLEEP="%s"

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

func (w *ChaosDiskWorkload) mount() string {
	if w.Config.Params != nil {
		if val, ok := w.Config.Params["mount"]; ok && val != "" {
			return val
		}
	}
	return "/mnt/data"
}

func (w *ChaosDiskWorkload) fillPercent() string {
	if w.Config.Params != nil {
		if val, ok := w.Config.Params["fill-percent"]; ok && val != "" {
			return val
		}
	}
	return "90"
}

func (w *ChaosDiskWorkload) fillSleep() string {
	if w.Config.Params != nil {
		if val, ok := w.Config.Params["fill-sleep"]; ok && val != "" {
			return val
		}
	}
	return "60"
}

func (w *ChaosDiskWorkload) releaseSleep() string {
	if w.Config.Params != nil {
		if val, ok := w.Config.Params["release-sleep"]; ok && val != "" {
			return val
		}
	}
	return "30"
}

// Name returns "chaos-disk".
func (w *ChaosDiskWorkload) Name() string {
	return "chaos-disk"
}

// CloudInitUserdata returns cloud-init YAML that writes a fill/release script
// and a systemd service that runs it in a loop.
func (w *ChaosDiskWorkload) CloudInitUserdata() (string, error) {
	mountPoint := w.mount()
	script := fmt.Sprintf(chaosDiskScriptTemplate,
		mountPoint,
		w.fillPercent(),
		w.releaseSleep(),
		w.fillSleep())
	unit := chaosDiskSystemdUnit

	return w.BuildCloudConfig(CloudConfigOpts{
		WriteFiles: []WriteFile{
			{
				Path:        "/usr/local/bin/virtwork-disk-setup.sh",
				Content:     diskSetupScript("virtwork-chdisk", mountPoint),
				Permissions: "0755",
			},
			{
				Path:        "/usr/local/bin/chaos-disk.sh",
				Content:     script,
				Permissions: "0755",
			},
			{
				Path:        "/etc/systemd/system/virtwork-chaos-disk.service",
				Content:     unit,
				Permissions: "0644",
			},
		},
		RunCmd: [][]string{
			{"/usr/local/bin/virtwork-disk-setup.sh"},
			{"systemctl", "daemon-reload"},
			{"systemctl", "enable", "--now", "virtwork-chaos-disk.service"},
		},
	})
}

// DataVolumeTemplates returns a DataVolumeTemplateSpec for the data disk.
func (w *ChaosDiskWorkload) DataVolumeTemplates() ([]kubevirtv1.DataVolumeTemplateSpec, error) {
	dvt, err := vm.BuildDataVolumeTemplate("virtwork-chaos-disk-data", w.DataDiskSize)
	if err != nil {
		return nil, err
	}
	return []kubevirtv1.DataVolumeTemplateSpec{dvt}, nil
}

// ExtraDisks returns the data disk definition.
func (w *ChaosDiskWorkload) ExtraDisks() []kubevirtv1.Disk {
	return []kubevirtv1.Disk{
		{
			Name:   "datadisk",
			Serial: "virtwork-chdisk",
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
