// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads

import (
	"fmt"

	"github.com/opdev/virtwork/internal/config"
)

const cpuSystemdUnitTemplate = `[Unit]
Description=Virtwork CPU stress workload
After=network.target

[Service]
Type=simple
ExecStart=/usr/bin/stress-ng --cpu 0 --cpu-load %s --cpu-method %s --timeout 0
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
`

// CPUParamSchema declares the configurable params for the CPU workload.
var CPUParamSchema = ParamSchema{
	{
		Key:     "cpu-load-percent",
		Type:    ParamInt,
		Default: "100",
		Desc:    "Target CPU load percentage for stress-ng (--cpu-load)",
	},
	{Key: "cpu-method", Type: ParamString, Default: "all", Desc: "CPU stressor method for stress-ng (--cpu-method)"},
}

// CPUWorkload generates cloud-init userdata for a continuous CPU stress workload
// using stress-ng.
type CPUWorkload struct {
	BaseWorkload
}

// NewCPUWorkload creates a CPUWorkload with the given configuration and SSH credentials.
func NewCPUWorkload(
	cfg config.WorkloadConfig,
	sshUser, sshPassword string,
	sshKeys []string,
) *CPUWorkload {
	return &CPUWorkload{
		BaseWorkload: BaseWorkload{
			Config:            cfg,
			ParamSchema:       CPUParamSchema,
			SSHUser:           sshUser,
			SSHPassword:       sshPassword,
			SSHAuthorizedKeys: sshKeys,
		},
	}
}

// Name returns "cpu".
func (w *CPUWorkload) Name() string {
	return "cpu"
}

// CloudInitUserdata returns cloud-init YAML that installs stress-ng and runs a
// continuous CPU stress workload via systemd.
func (w *CPUWorkload) CloudInitUserdata() (string, error) {
	unit := fmt.Sprintf(cpuSystemdUnitTemplate, w.GetParam("cpu-load-percent"), w.GetParam("cpu-method"))
	return w.BuildCloudConfig(CloudConfigOpts{
		Packages: []string{"stress-ng"},
		WriteFiles: []WriteFile{
			{
				Path:        "/etc/systemd/system/virtwork-cpu.service",
				Content:     unit,
				Permissions: "0644",
			},
		},
		RunCmd: [][]string{
			{"systemctl", "daemon-reload"},
			{"systemctl", "enable", "--now", "virtwork-cpu.service"},
		},
	})
}
