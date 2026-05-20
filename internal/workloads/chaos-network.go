// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads

import (
	"fmt"

	"github.com/opdev/virtwork/internal/config"
)

const chaosNetworkSystemdUnit = `[Unit]
Description=Virtwork network chaos workload (tc/netem)
After=network.target

[Service]
Type=simple
ExecStart=/usr/sbin/tc qdisc add dev eth0 root netem delay %sms loss %s%% limit 1000
ExecStop=/usr/sbin/tc qdisc del dev eth0 root
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
func NewChaosNetworkWorkload(cfg config.WorkloadConfig, sshUser, sshPassword string, sshKeys []string) *ChaosNetworkWorkload {
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
	// Format the systemd unit with latency and packet loss parameters
	systemdContent := fmt.Sprintf(chaosNetworkSystemdUnit,
		fmt.Sprintf("%d", w.Latency),
		fmt.Sprintf("%.1f", w.PacketLoss))

	return w.BuildCloudConfig(CloudConfigOpts{
		Packages: nil, // iproute-tc assumed pre-installed via golden image
		WriteFiles: []WriteFile{
			{
				Path:        "/etc/systemd/system/virtwork-chaos-network.service",
				Content:     systemdContent,
				Permissions: "0644",
			},
		},
		RunCmd: [][]string{
			{"systemctl", "daemon-reload"},
			{"systemctl", "enable", "--now", "virtwork-chaos-network.service"},
		},
	})
}
