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
			SSHUser:           sshUser,
			SSHPassword:       sshPassword,
			SSHAuthorizedKeys: sshKeys,
		},
	}
}

func (w *MemoryWorkload) memoryPercent() string {
	if w.Config.Params != nil {
		if val, ok := w.Config.Params["memory-percent"]; ok && val != "" {
			return val
		}
	}
	return "80"
}

func (w *MemoryWorkload) vmStressors() string {
	if w.Config.Params != nil {
		if val, ok := w.Config.Params["vm-stressors"]; ok && val != "" {
			return val
		}
	}
	return "1"
}

func (w *MemoryWorkload) vmMethod() string {
	if w.Config.Params != nil {
		if val, ok := w.Config.Params["vm-method"]; ok && val != "" {
			return val
		}
	}
	return "all"
}

// Name returns "memory".
func (w *MemoryWorkload) Name() string {
	return "memory"
}

// CloudInitUserdata returns cloud-init YAML that installs stress-ng and runs a
// continuous memory pressure workload via systemd.
func (w *MemoryWorkload) CloudInitUserdata() (string, error) {
	unit := fmt.Sprintf(memorySystemdUnitTemplate, w.vmStressors(), w.memoryPercent(), w.vmMethod())
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
