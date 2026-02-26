// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads

import (
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/vm"
)

const dbSetupScript = `#!/bin/bash
set -euo pipefail

DATA_DIR="/var/lib/pgsql/data"
MARKER="${DATA_DIR}/.virtwork-initialized"

# Skip if already initialized
if [ -f "${MARKER}" ]; then
    echo "Database already initialized, skipping setup"
    exit 0
fi

# Format and mount the data disk
if ! mountpoint -q "${DATA_DIR}"; then
    mkfs.xfs /dev/vdc
    mount /dev/vdc "${DATA_DIR}"
    echo '/dev/vdc /var/lib/pgsql/data xfs defaults 0 0' >> /etc/fstab
fi

# Set ownership for postgres user
chown -R postgres:postgres "${DATA_DIR}"

# Initialize PostgreSQL
postgresql-setup --initdb

# Start PostgreSQL temporarily for pgbench init
systemctl start postgresql

# Create pgbench database with scale factor 50
sudo -u postgres createdb pgbench
sudo -u postgres pgbench -i -s 50 pgbench

# Stop PostgreSQL (systemd will manage it)
systemctl stop postgresql

# Mark as initialized
touch "${MARKER}"
chown postgres:postgres "${MARKER}"
`

const dbSystemdUnit = `[Unit]
Description=Virtwork database benchmark workload
After=network.target local-fs.target postgresql.service
Requires=postgresql.service

[Service]
Type=simple
User=postgres
ExecStartPre=/usr/local/bin/virtwork-db-setup.sh
ExecStart=/bin/bash -c 'while true; do pgbench -c 10 -j 2 -T 300 pgbench; sleep 10; done'
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

// Name returns "database".
func (w *DatabaseWorkload) Name() string {
	return "database"
}

// CloudInitUserdata returns cloud-init YAML that installs PostgreSQL, writes
// a setup script for one-time database initialization, and creates a systemd
// service that runs continuous pgbench benchmarks.
func (w *DatabaseWorkload) CloudInitUserdata() (string, error) {
	return w.BuildCloudConfig(CloudConfigOpts{
		Packages: []string{"postgresql-server"},
		WriteFiles: []WriteFile{
			{
				Path:        "/usr/local/bin/virtwork-db-setup.sh",
				Content:     dbSetupScript,
				Permissions: "0755",
			},
			{
				Path:        "/etc/systemd/system/virtwork-database.service",
				Content:     dbSystemdUnit,
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
func (w *DatabaseWorkload) DataVolumeTemplates() []kubevirtv1.DataVolumeTemplateSpec {
	return []kubevirtv1.DataVolumeTemplateSpec{
		vm.BuildDataVolumeTemplate("virtwork-database-data", w.DataDiskSize),
	}
}

// ExtraDisks returns the data disk definition for PostgreSQL storage.
func (w *DatabaseWorkload) ExtraDisks() []kubevirtv1.Disk {
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
func (w *DatabaseWorkload) ExtraVolumes() []kubevirtv1.Volume {
	return []kubevirtv1.Volume{
		{
			Name: "datadisk",
			VolumeSource: kubevirtv1.VolumeSource{
				DataVolume: &kubevirtv1.DataVolumeSource{
					Name: "virtwork-database-data",
				},
			},
		},
	}
}
