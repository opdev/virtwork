// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads

import (
	"fmt"

	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/constants"
	"github.com/opdev/virtwork/internal/vm"
)

const fioMixedRWProfileTemplate = `[global]
ioengine=libaio
direct=1
directory=/mnt/data
size=1G

[mixed-rw]
rw=randrw
rwmixread=%s
bs=%s
numjobs=%s
runtime=%s
time_based
group_reporting
`

const fioSeqWriteProfileTemplate = `[global]
ioengine=libaio
direct=1
directory=/mnt/data
size=1G

[seq-write]
rw=write
bs=%s
numjobs=2
runtime=%s
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

// DiskParamSchema declares the configurable params for the disk workload.
var DiskParamSchema = ParamSchema{
	{Key: "block-size-rw", Type: ParamString, Default: "4k", Desc: "Block size for random read/write fio profile"},
	{Key: "block-size-seq", Type: ParamString, Default: "128k", Desc: "Block size for sequential write fio profile"},
	{Key: "rwmixread", Type: ParamInt, Default: "70", Desc: "Read percentage in mixed read/write fio profile"},
	{Key: "numjobs", Type: ParamInt, Default: "4", Desc: "Number of parallel fio jobs"},
	{Key: "runtime", Type: ParamInt, Default: "300", Desc: "Runtime in seconds per fio profile run"},
}

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
			ParamSchema:       DiskParamSchema,
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
	mixedRW := fmt.Sprintf(fioMixedRWProfileTemplate,
		w.GetParam("rwmixread"), w.GetParam("block-size-rw"), w.GetParam("numjobs"), w.GetParam("runtime"))
	seqWrite := fmt.Sprintf(fioSeqWriteProfileTemplate,
		w.GetParam("block-size-seq"), w.GetParam("runtime"))
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
				Content:     mixedRW,
				Permissions: "0644",
			},
			{
				Path:        "/etc/fio/seq-write.fio",
				Content:     seqWrite,
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
