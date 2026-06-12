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

const dbSetupScriptTemplate = `#!/bin/bash
set -euo pipefail

DATA_DIR="/var/lib/pgsql/data"
MARKER="${DATA_DIR}/.virtwork-initialized"

# Skip if already initialized
if [ -f "${MARKER}" ]; then
    echo "Database already initialized, skipping setup"
    exit 0
fi

# Discover the data disk by virtio serial
DISK="/dev/disk/by-id/virtio-virtwork-dbdisk"
for i in $(seq 1 30); do
    [ -e "${DISK}" ] && break
    sleep 1
done
if [ ! -e "${DISK}" ]; then
    echo "ERROR: disk ${DISK} not found after 30s" >&2
    exit 1
fi
REAL_DEV=$(readlink -f "${DISK}")

# Format and mount the data disk
if ! mountpoint -q "${DATA_DIR}"; then
    if ! blkid -o value -s TYPE "${REAL_DEV}" > /dev/null 2>&1; then
        mkfs.xfs "${REAL_DEV}"
    fi
    mount "${REAL_DEV}" "${DATA_DIR}"
    if ! grep -q "${DATA_DIR}" /etc/fstab; then
        echo "${REAL_DEV} ${DATA_DIR} xfs defaults,nofail 0 0" >> /etc/fstab
    fi
fi

# Set ownership for postgres user
chown -R postgres:postgres "${DATA_DIR}"

# Initialize PostgreSQL
postgresql-setup --initdb

# Start PostgreSQL temporarily for pgbench init
systemctl start postgresql

# Create pgbench database with configured scale factor
sudo -u postgres createdb pgbench
sudo -u postgres pgbench -i -s %s pgbench

# Stop PostgreSQL (systemd will manage it)
systemctl stop postgresql

# Mark as initialized
touch "${MARKER}"
chown postgres:postgres "${MARKER}"
`

const dbSystemdUnitTemplate = `[Unit]
Description=Virtwork database benchmark workload
After=network.target local-fs.target postgresql.service
Requires=postgresql.service

[Service]
Type=simple
User=postgres
ExecStartPre=/usr/local/bin/virtwork-db-setup.sh
ExecStart=/bin/bash -c 'while true; do pgbench -c %s -j 2 -T %s pgbench; sleep 10; done'
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
`

// DatabaseWorkload generates cloud-init userdata for a PostgreSQL database
// benchmark workload using pgbench. It formats a data disk, initializes
// PostgreSQL, creates a pgbench database at scale 50, and runs continuous
// benchmark loops.
type DatabaseWorkload struct {
	BaseWorkload
	DataDiskSize string
}

// NewDatabaseWorkload creates a DatabaseWorkload with the given configuration,
// disk size, and SSH credentials.
func NewDatabaseWorkload(
	cfg config.WorkloadConfig,
	dataDiskSize, sshUser, sshPassword string,
	sshKeys []string,
) *DatabaseWorkload {
	return &DatabaseWorkload{
		BaseWorkload: BaseWorkload{
			Config:            cfg,
			SSHUser:           sshUser,
			SSHPassword:       sshPassword,
			SSHAuthorizedKeys: sshKeys,
		},
		DataDiskSize: dataDiskSize,
	}
}

func (w *DatabaseWorkload) scaleFactor() string {
	if w.Config.Params != nil {
		if val, ok := w.Config.Params["scale-factor"]; ok && val != "" {
			return val
		}
	}
	return "50"
}

func (w *DatabaseWorkload) clients() string {
	if w.Config.Params != nil {
		if val, ok := w.Config.Params["clients"]; ok && val != "" {
			return val
		}
	}
	return "10"
}

func (w *DatabaseWorkload) duration() string {
	if w.Config.Params != nil {
		if val, ok := w.Config.Params["duration"]; ok && val != "" {
			return val
		}
	}
	return "300"
}

// Name returns "database".
func (w *DatabaseWorkload) Name() string {
	return "database"
}

// CloudInitUserdata returns cloud-init YAML that installs PostgreSQL, writes
// a setup script for one-time database initialization, and creates a systemd
// service that runs continuous pgbench benchmarks.
func (w *DatabaseWorkload) CloudInitUserdata() (string, error) {
	setupScript := fmt.Sprintf(dbSetupScriptTemplate, w.scaleFactor())
	serviceUnit := fmt.Sprintf(dbSystemdUnitTemplate, w.clients(), w.duration())
	return w.BuildCloudConfig(CloudConfigOpts{
		Packages: []string{"postgresql-server"},
		WriteFiles: []WriteFile{
			{
				Path:        "/usr/local/bin/virtwork-db-setup.sh",
				Content:     setupScript,
				Permissions: "0755",
			},
			{
				Path:        "/etc/systemd/system/virtwork-database.service",
				Content:     serviceUnit,
				Permissions: "0644",
			},
		},
		RunCmd: [][]string{
			{"systemctl", "daemon-reload"},
			{"systemctl", "enable", "postgresql"},
			{"systemctl", "enable", "--now", "virtwork-database.service"},
		},
	})
}

// DataVolumeTemplates returns a DataVolumeTemplateSpec for the PostgreSQL data disk.
func (w *DatabaseWorkload) DataVolumeTemplates() ([]kubevirtv1.DataVolumeTemplateSpec, error) {
	dvt, err := vm.BuildDataVolumeTemplate("virtwork-database-data", w.DataDiskSize)
	if err != nil {
		return nil, err
	}
	return []kubevirtv1.DataVolumeTemplateSpec{dvt}, nil
}

// ExtraDisks returns the data disk definition for PostgreSQL storage.
func (w *DatabaseWorkload) ExtraDisks() []kubevirtv1.Disk {
	return []kubevirtv1.Disk{
		{
			Name:   constants.DiskNameData,
			Serial: "virtwork-dbdisk",
			DiskDevice: kubevirtv1.DiskDevice{
				Disk: &kubevirtv1.DiskTarget{
					Bus: constants.DiskBusVirtio,
				},
			},
		},
	}
}

// ExtraVolumes returns the data volume sourced from the DataVolume.
func (w *DatabaseWorkload) ExtraVolumes() []kubevirtv1.Volume {
	return []kubevirtv1.Volume{
		{
			Name: constants.DiskNameData,
			VolumeSource: kubevirtv1.VolumeSource{
				DataVolume: &kubevirtv1.DataVolumeSource{
					Name: "virtwork-database-data",
				},
			},
		},
	}
}
