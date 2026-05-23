// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads

import (
	"fmt"

	"github.com/opdev/virtwork/internal/config"
)

const chaosNetworkStartScript = `#!/bin/bash
set -euo pipefail

DEV=$(ip route show default | awk '{print $5; exit}')
if [ -z "$DEV" ]; then
  echo "no default route interface found" >&2
  exit 1
fi

modprobe sch_netem 2>/dev/null || true

exec /usr/sbin/tc qdisc add dev "$DEV" root netem delay %sms loss %s%% limit 1000
`

const chaosNetworkStopScript = `#!/bin/bash
DEV=$(ip route show default | awk '{print $5; exit}')
[ -n "$DEV" ] && /usr/sbin/tc qdisc del dev "$DEV" root 2>/dev/null
exit 0
`

const chaosNetworkSystemdUnit = `[Unit]
Description=Virtwork network chaos workload (tc/netem)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/virtwork-chaos-network-start.sh
ExecStop=/usr/local/bin/virtwork-chaos-network-stop.sh
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
`

// ChaosNetworkWorkload generates cloud-init userdata for network chaos injection
// using tc (traffic control) and netem (network emulation).
type ChaosNetworkWorkload struct {
	BaseWorkload
}

// NewChaosNetworkWorkload creates a ChaosNetworkWorkload with the given configuration
// and SSH credentials.
func NewChaosNetworkWorkload(
	cfg config.WorkloadConfig,
	sshUser, sshPassword string,
	sshKeys []string,
) *ChaosNetworkWorkload {
	return &ChaosNetworkWorkload{
		BaseWorkload: BaseWorkload{
			Config:            cfg,
			SSHUser:           sshUser,
			SSHPassword:       sshPassword,
			SSHAuthorizedKeys: sshKeys,
		},
	}
}

func (w *ChaosNetworkWorkload) latencyMs() string {
	if w.Config.Params != nil {
		if val, ok := w.Config.Params["latency-ms"]; ok && val != "" {
			return val
		}
	}
	return "100"
}

func (w *ChaosNetworkWorkload) packetLossPercent() string {
	if w.Config.Params != nil {
		if val, ok := w.Config.Params["packet-loss-percent"]; ok && val != "" {
			return val
		}
	}
	return "5.0"
}

// Name returns "chaos-network".
func (w *ChaosNetworkWorkload) Name() string {
	return "chaos-network"
}

// CloudInitUserdata returns cloud-init YAML that configures tc/netem for network chaos
// injection via systemd. The golden image pre-installs iproute-tc and kernel-modules-extra
// (sch_netem); the start script runs modprobe as a fallback for non-golden images.
func (w *ChaosNetworkWorkload) CloudInitUserdata() (string, error) {
	startScript := fmt.Sprintf(chaosNetworkStartScript,
		w.latencyMs(),
		w.packetLossPercent())

	return w.BuildCloudConfig(CloudConfigOpts{
		Packages: []string{"iproute-tc"},
		WriteFiles: []WriteFile{
			{
				Path:        "/usr/local/bin/virtwork-chaos-network-start.sh",
				Content:     startScript,
				Permissions: "0755",
			},
			{
				Path:        "/usr/local/bin/virtwork-chaos-network-stop.sh",
				Content:     chaosNetworkStopScript,
				Permissions: "0755",
			},
			{
				Path:        "/etc/systemd/system/virtwork-chaos-network.service",
				Content:     chaosNetworkSystemdUnit,
				Permissions: "0644",
			},
		},
		RunCmd: [][]string{
			{"bash", "-c", "dnf install -y kernel-modules-extra-$(uname -r)"},
			{"systemctl", "daemon-reload"},
			{"systemctl", "enable", "--now", "virtwork-chaos-network.service"},
		},
	})
}
