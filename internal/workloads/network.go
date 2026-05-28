// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads

import (
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/constants"
)

const iperf3ServerSystemdUnit = `[Unit]
Description=Virtwork iperf3 server
After=network.target

[Service]
Type=simple
ExecStart=/usr/bin/iperf3 -s
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
`

var ErrUnknownNetworkRole = errors.New("unexpected network role; expected 'server' or 'client'")

// NetworkWorkload generates cloud-init userdata for an iperf3 network benchmark.
// It creates two VMs: a server running iperf3 in listen mode, and a client that
// runs bidirectional tests against the server via DNS. A K8s Service routes
// traffic to the server VM.
type NetworkWorkload struct {
	BaseWorkload
	Namespace string
}

// NewNetworkWorkload creates a NetworkWorkload with the given configuration,
// namespace, and SSH credentials.
func NewNetworkWorkload(
	cfg config.WorkloadConfig,
	namespace, sshUser, sshPassword string,
	sshKeys []string,
) *NetworkWorkload {
	return &NetworkWorkload{
		BaseWorkload: BaseWorkload{
			Config:            cfg,
			SSHUser:           sshUser,
			SSHPassword:       sshPassword,
			SSHAuthorizedKeys: sshKeys,
		},
		Namespace: namespace,
	}
}

// Name returns "network".
func (w *NetworkWorkload) Name() string {
	return "network"
}

// VMCount returns the total VM count — one server and one client per
// configured vm-count.
func (w *NetworkWorkload) VMCount() int {
	count := w.Config.VMCount
	count = max(1, count)
	return count * 2
}

// Roles returns the supported VM roles for this multi-VM workload.
func (w *NetworkWorkload) Roles() []string {
	return []string{"server", "client"}
}

// RequiresService returns true — the client needs a ClusterIP Service to reach
// the server by DNS.
func (w *NetworkWorkload) RequiresService() bool {
	return true
}

// ServiceSpec returns a ClusterIP Service on port 5201 targeting the server VM
// by the virtwork/role: server label.
func (w *NetworkWorkload) ServiceSpec() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "virtwork-iperf3-server",
			Namespace: w.Namespace,
			Labels: map[string]string{
				constants.LabelAppName:   constants.ManagedByValue,
				constants.LabelManagedBy: constants.ManagedByValue,
				constants.LabelComponent: "network",
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"virtwork/role": "server",
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "iperf3",
					Port:       5201,
					TargetPort: intstr.FromInt32(5201),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
}

// CloudInitUserdata returns the server role userdata as the default.
func (w *NetworkWorkload) CloudInitUserdata() (string, error) {
	return w.UserdataForRole("server", w.Namespace)
}

// UserdataForRole returns cloud-init YAML for the given role ("server" or "client").
// The server runs iperf3 in listen mode. The client runs bidirectional tests
// against the server's DNS name.
func (w *NetworkWorkload) UserdataForRole(role string, namespace string) (string, error) {
	switch role {
	case "server":
		return w.buildServerUserdata()
	case "client":
		return w.buildClientUserdata(namespace)
	default:
		return "", fmt.Errorf("unknown network workload role: %q; %w", role, ErrUnknownNetworkRole)
	}
}

func (w *NetworkWorkload) buildServerUserdata() (string, error) {
	return w.BuildCloudConfig(CloudConfigOpts{
		Packages: []string{"iperf3"},
		WriteFiles: []WriteFile{
			{
				Path:        "/etc/systemd/system/virtwork-network.service",
				Content:     iperf3ServerSystemdUnit,
				Permissions: "0644",
			},
		},
		RunCmd: [][]string{
			{"systemctl", "daemon-reload"},
			{"systemctl", "enable", "--now", "virtwork-network.service"},
		},
	})
}

func (w *NetworkWorkload) buildClientUserdata(namespace string) (string, error) {
	dnsName := fmt.Sprintf("virtwork-iperf3-server.%s.svc.cluster.local", namespace)
	clientUnit := fmt.Sprintf(`[Unit]
Description=Virtwork iperf3 client
After=network.target

[Service]
Type=simple
ExecStart=/bin/bash -c 'while true; do iperf3 -c %s -t 60 -P 4 --bidir; sleep 10; done'
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
`, dnsName)

	return w.BuildCloudConfig(CloudConfigOpts{
		Packages: []string{"iperf3"},
		WriteFiles: []WriteFile{
			{
				Path:        "/etc/systemd/system/virtwork-network.service",
				Content:     clientUnit,
				Permissions: "0644",
			},
		},
		RunCmd: [][]string{
			{"systemctl", "daemon-reload"},
			{"systemctl", "enable", "--now", "virtwork-network.service"},
		},
	})
}
