// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads

import (
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/constants"
	"github.com/opdev/virtwork/internal/vm"
)

const fioMixedRWProfile = `[global]
ioengine=libaio
direct=1
directory=/mnt/data
size=1G

[mixed-rw]
rw=randrw
rwmixread=70
bs=4k
numjobs=4
runtime=300
time_based
group_reporting
`

const fioSeqWriteProfile = `[global]
ioengine=libaio
direct=1
directory=/mnt/data
size=1G

[seq-write]
rw=write
bs=128k
numjobs=2
runtime=300
time_based
group_reporting
`

const diskSystemdUnit = `[Unit]
Description=Virtwork disk I/O workload
After=network.target local-fs.target

[Service]
Type=simple
ExecStart=/bin/bash -c 'while true; do fio /etc/fio/mixed-rw.fio; sleep 10; fio /etc/fio/seq-write.fio; sleep 10; done'
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
`

// DiskWorkload generates cloud-init userdata for a disk I/O workload using fio.
// It alternates between a 4K random read/write mix and 128K sequential writes.
type DiskWorkload struct {
	BaseWorkload
	DataDiskSize string
}

// NewDiskWorkload creates a DiskWorkload with the given configuration, disk size,
// and SSH credentials.
func NewDiskWorkload(
	cfg config.WorkloadConfig,
	dataDiskSize, sshUser, sshPassword string,
	sshKeys []string,
) *DiskWorkload {
	return &DiskWorkload{
		BaseWorkload: BaseWorkload{
			Config:            cfg,
			SSHUser:           sshUser,
			SSHPassword:       sshPassword,
			SSHAuthorizedKeys: sshKeys,
		},
		DataDiskSize: dataDiskSize,
	}
}

// Name returns "disk".
func (w *DiskWorkload) Name() string {
	return "disk"
}

// CloudInitUserdata returns cloud-init YAML that installs fio, writes two job
// profiles, and creates a systemd service that alternates between them.
func (w *DiskWorkload) CloudInitUserdata() (string, error) {
	return w.BuildCloudConfig(CloudConfigOpts{
		Packages: []string{"fio"},
		WriteFiles: []WriteFile{
			{
				Path:        "/usr/local/bin/virtwork-disk-setup.sh",
				Content:     diskSetupScript("virtwork-disk", "/mnt/data"),
				Permissions: "0755",
			},
			{
				Path:        "/etc/fio/mixed-rw.fio",
				Content:     fioMixedRWProfile,
				Permissions: "0644",
			},
			{
				Path:        "/etc/fio/seq-write.fio",
				Content:     fioSeqWriteProfile,
				Permissions: "0644",
			},
			{
				Path:        "/etc/systemd/system/virtwork-disk.service",
				Content:     diskSystemdUnit,
				Permissions: "0644",
			},
		},
		RunCmd: [][]string{
			{"/usr/local/bin/virtwork-disk-setup.sh"},
			{"systemctl", "daemon-reload"},
			{"systemctl", "enable", "--now", "virtwork-disk.service"},
		},
	})
}

// DataVolumeTemplates returns a DataVolumeTemplateSpec for the data disk.
func (w *DiskWorkload) DataVolumeTemplates() ([]kubevirtv1.DataVolumeTemplateSpec, error) {
	dvt, err := vm.BuildDataVolumeTemplate("virtwork-disk-data", w.DataDiskSize)
	if err != nil {
		return nil, err
	}
	return []kubevirtv1.DataVolumeTemplateSpec{dvt}, nil
}

// ExtraDisks returns the data disk definition.
func (w *DiskWorkload) ExtraDisks() []kubevirtv1.Disk {
	return []kubevirtv1.Disk{
		{
			Name:   constants.DiskNameData,
			Serial: "virtwork-disk",
			DiskDevice: kubevirtv1.DiskDevice{
				Disk: &kubevirtv1.DiskTarget{
					Bus: constants.DiskBusVirtio,
				},
			},
		},
	}
}

// ExtraVolumes returns the data volume sourced from the DataVolume.
func (w *DiskWorkload) ExtraVolumes() []kubevirtv1.Volume {
	return []kubevirtv1.Volume{
		{
			Name: constants.DiskNameData,
			VolumeSource: kubevirtv1.VolumeSource{
				DataVolume: &kubevirtv1.DataVolumeSource{
					Name: "virtwork-disk-data",
				},
			},
		},
	}
}
