// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads

import (
	"fmt"

	"github.com/opdev/virtwork/internal/config"
)

const memorySystemdUnitTemplate = `[Unit]
Description=Virtwork memory stress workload
After=network.target

[Service]
Type=simple
ExecStart=/usr/bin/stress-ng --vm %s --vm-bytes %s%% --vm-method %s --timeout 0
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
`

// MemoryParamSchema declares the configurable params for the memory workload.
var MemoryParamSchema = ParamSchema{
	{
		Key:     "memory-percent",
		Type:    ParamInt,
		Default: "80",
		Desc:    "Target memory usage percentage for stress-ng (--vm-bytes)",
	},
	{Key: "vm-stressors", Type: ParamInt, Default: "1", Desc: "Number of VM stressor workers for stress-ng (--vm)"},
	{Key: "vm-method", Type: ParamString, Default: "all", Desc: "Memory stressor method for stress-ng (--vm-method)"},
}

// MemoryWorkload generates cloud-init userdata for a continuous memory pressure
// workload using stress-ng. It uses a single VM worker (--vm 1) targeting 80%
// of available memory to produce sustained pressure without triggering OOM kills.
type MemoryWorkload struct {
	BaseWorkload
}

// NewMemoryWorkload creates a MemoryWorkload with the given configuration and SSH credentials.
func NewMemoryWorkload(
	cfg config.WorkloadConfig,
	sshUser, sshPassword string,
	sshKeys []string,
) *MemoryWorkload {
	return &MemoryWorkload{
		BaseWorkload: BaseWorkload{
			Config:            cfg,
			ParamSchema:       MemoryParamSchema,
			SSHUser:           sshUser,
			SSHPassword:       sshPassword,
			SSHAuthorizedKeys: sshKeys,
		},
	}
}

// Name returns "memory".
func (w *MemoryWorkload) Name() string {
	return "memory"
}

// CloudInitUserdata returns cloud-init YAML that installs stress-ng and runs a
// continuous memory pressure workload via systemd.
func (w *MemoryWorkload) CloudInitUserdata() (string, error) {
	unit := fmt.Sprintf(
		memorySystemdUnitTemplate,
		w.GetParam("vm-stressors"),
		w.GetParam("memory-percent"),
		w.GetParam("vm-method"),
	)
	return w.BuildCloudConfig(CloudConfigOpts{
		Packages: []string{"stress-ng"},
		WriteFiles: []WriteFile{
			{
				Path:        "/etc/systemd/system/virtwork-memory.service",
				Content:     unit,
				Permissions: "0644",
			},
		},
		RunCmd: [][]string{
			{"systemctl", "daemon-reload"},
			{"systemctl", "enable", "--now", "virtwork-memory.service"},
		},
	})
}
