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
	Latency    int     // Latency in milliseconds (default: 100)
	PacketLoss float64 // Packet loss percentage (default: 5.0)
}

// NewChaosNetworkWorkload creates a ChaosNetworkWorkload with the given configuration
// and SSH credentials.
func NewChaosNetworkWorkload(
	cfg config.WorkloadConfig,
	sshUser, sshPassword string,
	sshKeys []string,
) *ChaosNetworkWorkload {
	latency := 100
	packetLoss := 5.0

	// Allow configuration via WorkloadConfig if needed in future
	// For now, use sensible defaults as specified in the issue

	return &ChaosNetworkWorkload{
		BaseWorkload: BaseWorkload{
			Config:            cfg,
			SSHUser:           sshUser,
			SSHPassword:       sshPassword,
			SSHAuthorizedKeys: sshKeys,
		},
		Latency:    latency,
		PacketLoss: packetLoss,
	}
}

// Name returns "chaos-network".
func (w *ChaosNetworkWorkload) Name() string {
	return "chaos-network"
}

// CloudInitUserdata returns cloud-init YAML that configures tc/netem for network chaos
// injection via systemd. Assumes iproute-tc is pre-installed (golden image dependency).
func (w *ChaosNetworkWorkload) CloudInitUserdata() (string, error) {
	startScript := fmt.Sprintf(chaosNetworkStartScript,
		fmt.Sprintf("%d", w.Latency),
		fmt.Sprintf("%.1f", w.PacketLoss))

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
