// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/opdev/virtwork/internal/config"
)

const tpsServerSystemdUnit = `[Unit]
Description=Virtwork TPS server (netperf + HTTP)
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/virtwork-tps-server.sh
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
`

const tpsClientSystemdUnit = `[Unit]
Description=Virtwork TPS client (netperf TCP_RR + HTTP file transfer)
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/virtwork-tps-client.sh
Restart=always
RestartSec=30

[Install]
WantedBy=multi-user.target
`

var ErrUnknownTPSRole = errors.New("unexpected tps role; expected 'server' or 'client'")

var ErrInvalidFileSize = errors.New(
	"invalid file-size format: must be a positive integer followed by K, M, or G (e.g. '10M')",
)

// TPSParamSchema declares the configurable params for the TPS workload.
var TPSParamSchema = ParamSchema{
	{Key: "file-size", Type: ParamString, Default: "10M", Desc: "Size of the test file for HTTP transfer (e.g. 10M, 1G)"},
	{Key: "iterations", Type: ParamInt, Default: "30", Desc: "Number of test iterations per cycle"},
	{Key: "duration", Type: ParamInt, Default: "60", Desc: "Duration in seconds per netperf test (-l)"},
}

// TPSWorkload generates cloud-init userdata for a combined TPS benchmark.
// It creates server/client VM pairs. The server runs netserver and python3
// http.server. The client runs TCP_RR tests and HTTP GET download loops,
// all within a single systemd unit for unified journalctl output.
type TPSWorkload struct {
	BaseWorkload
	Namespace string
}

// NewTPSWorkload creates a TPSWorkload with the given configuration,
// namespace, and SSH credentials.
func NewTPSWorkload(
	cfg config.WorkloadConfig,
	namespace, sshUser, sshPassword string,
	sshKeys []string,
) *TPSWorkload {
	return &TPSWorkload{
		BaseWorkload: BaseWorkload{
			Config:            cfg,
			ParamSchema:       TPSParamSchema,
			SSHUser:           sshUser,
			SSHPassword:       sshPassword,
			SSHAuthorizedKeys: sshKeys,
		},
		Namespace: namespace,
	}
}

// Name returns "tps".
func (w *TPSWorkload) Name() string {
	return "tps"
}

// RoleDistribution returns per-role VM counts — one server and one client
// per configured vm-count.
func (w *TPSWorkload) RoleDistribution() []RoleSpec {
	perRole := max(1, w.Config.VMCount)
	return []RoleSpec{
		{Role: "server", VMCount: perRole},
		{Role: "client", VMCount: perRole},
	}
}

// VMCount returns the total VM count across all roles.
func (w *TPSWorkload) VMCount() int {
	total := 0
	for _, rs := range w.RoleDistribution() {
		total += rs.VMCount
	}
	return total
}

// RequiresService returns true — the client needs a ClusterIP Service to reach
// the server by DNS.
func (w *TPSWorkload) RequiresService() bool {
	return true
}

// ServiceSpec returns a ClusterIP Service with ports for the netperf control
// channel (12865), a pinned data port (12866), and HTTP file transfer (8080).
func (w *TPSWorkload) ServiceSpec() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "virtwork-tps-server",
			Namespace: w.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "virtwork",
				"app.kubernetes.io/managed-by": "virtwork",
				"app.kubernetes.io/component":  "tps",
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"virtwork/role":               "server",
				"app.kubernetes.io/component": "tps",
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "netperf-ctrl",
					Port:       12865,
					TargetPort: intstr.FromInt32(12865),
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "netperf-data",
					Port:       12866,
					TargetPort: intstr.FromInt32(12866),
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "http-data",
					Port:       8080,
					TargetPort: intstr.FromInt32(8080),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
}

// CloudInitUserdata returns the server role userdata as the default.
func (w *TPSWorkload) CloudInitUserdata() (string, error) {
	return w.UserdataForRole("server", w.Namespace)
}

// UserdataForRole returns cloud-init YAML for the given role ("server" or "client").
func (w *TPSWorkload) UserdataForRole(role string, namespace string) (string, error) {
	switch role {
	case "server":
		return w.buildServerUserdata()
	case "client":
		return w.buildClientUserdata(namespace)
	default:
		return "", fmt.Errorf("unknown tps workload role: %q; %w", role, ErrUnknownTPSRole)
	}
}

func (w *TPSWorkload) serverDNSName(namespace string) string {
	return fmt.Sprintf("virtwork-tps-server.%s.svc.cluster.local", namespace)
}

func parseFileSize(raw string) (num int, suffix string, err error) {
	raw = strings.TrimSpace(raw)
	if len(raw) < 2 {
		return 0, "", fmt.Errorf("%w: %q", ErrInvalidFileSize, raw)
	}
	suffix = strings.ToUpper(raw[len(raw)-1:])
	numStr := raw[:len(raw)-1]

	switch suffix {
	case "K", "M", "G":
	default:
		return 0, "", fmt.Errorf("%w: %q (unrecognized suffix %q)", ErrInvalidFileSize, raw, raw[len(raw)-1:])
	}

	num, err = strconv.Atoi(numStr)
	if err != nil {
		return 0, "", fmt.Errorf("%w: %q (%q is not a valid integer)", ErrInvalidFileSize, raw, numStr)
	}
	if num <= 0 {
		return 0, "", fmt.Errorf("%w: %q (value must be positive)", ErrInvalidFileSize, raw)
	}

	return num, suffix, nil
}

func (w *TPSWorkload) fileSizeBytes() (string, error) {
	num, suffix, err := parseFileSize(w.GetParam("file-size"))
	if err != nil {
		return "", err
	}
	switch suffix {
	case "G":
		return strconv.Itoa(num * 1024 * 1024 * 1024), nil
	case "M":
		return strconv.Itoa(num * 1024 * 1024), nil
	case "K":
		return strconv.Itoa(num * 1024), nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidFileSize, w.GetParam("file-size"))
	}
}

func (w *TPSWorkload) fileSizeMBCount() (string, error) {
	num, suffix, err := parseFileSize(w.GetParam("file-size"))
	if err != nil {
		return "", err
	}
	switch suffix {
	case "G":
		return strconv.Itoa(num * 1024), nil
	case "M":
		return strconv.Itoa(num), nil
	case "K":
		return "1", nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidFileSize, w.GetParam("file-size"))
	}
}

func (w *TPSWorkload) buildServerUserdata() (string, error) {
	mbCount, err := w.fileSizeMBCount()
	if err != nil {
		return "", fmt.Errorf("tps server: %w", err)
	}

	serverScript := fmt.Sprintf(`#!/usr/bin/env bash
set -e

echo "=== Virtwork TPS Server ==="
echo "Starting netperf server on port 12865..."
netserver -D -p 12865 &
NETSERVER_PID=$!

echo "Creating test file (%s)..."
mkdir -p /srv/virtwork
dd if=/dev/urandom of=/srv/virtwork/testfile bs=1M count=%s iflag=fullblock 2>&1

echo "Starting HTTP file server on port 8080..."
python3 -m http.server 8080 --directory /srv/virtwork &
HTTP_PID=$!

echo "=== TPS Server ready ==="
echo "  netperf:  port 12865 (pid $NETSERVER_PID)"
echo "  HTTP:     port 8080  (pid $HTTP_PID)"
echo "  testfile: /srv/virtwork/testfile (%s)"

wait
`, w.GetParam("file-size"), mbCount, w.GetParam("file-size"))

	return w.BuildCloudConfig(CloudConfigOpts{
		Packages: []string{"netperf", "python3"},
		WriteFiles: []WriteFile{
			{
				Path:        "/usr/local/bin/virtwork-tps-server.sh",
				Content:     serverScript,
				Permissions: "0755",
			},
			{
				Path:        "/etc/systemd/system/virtwork-tps.service",
				Content:     tpsServerSystemdUnit,
				Permissions: "0644",
			},
		},
		RunCmd: [][]string{
			{"systemctl", "daemon-reload"},
			{"systemctl", "enable", "--now", "virtwork-tps.service"},
		},
	})
}

func (w *TPSWorkload) buildClientUserdata(namespace string) (string, error) {
	fileSizeBytes, err := w.fileSizeBytes()
	if err != nil {
		return "", fmt.Errorf("tps client: %w", err)
	}

	dnsName := w.serverDNSName(namespace)

	clientScript := fmt.Sprintf(`#!/usr/bin/env bash
SERVER=%s
ITERATIONS=%s
MSG_SIZE=64
DURATION=%s
FILE_SIZE_BYTES=%s

echo "=== Virtwork TPS Client ==="
echo "Server:     $SERVER"
echo "Iterations: $ITERATIONS"
echo "Duration:   ${DURATION}s per iteration"
echo ""

# Wait for server to be reachable
echo "Waiting for server..."
until curl -s -o /dev/null -w '' "http://${SERVER}:8080/" 2>/dev/null; do
  sleep 2
done
echo "Server is ready."
echo ""

# --- TCP_RR Tests ---
echo "=========================================="
echo "  TCP_RR TPS Test"
echo "=========================================="
echo "Msg size: ${MSG_SIZE} bytes"
echo ""

i=0
while [ $i -lt $ITERATIONS ]; do
  i=$((i + 1))
  echo "--- TCP_RR iteration $i/$ITERATIONS ---"
  netperf \
    -H "$SERVER" \
    -p 12865 \
    -t TCP_RR \
    -l "$DURATION" \
    -- \
    -r "$MSG_SIZE,$MSG_SIZE" \
    -P ,12866
  echo ""
  sleep 2
done

# --- HTTP File Transfer Tests ---
echo "=========================================="
echo "  HTTP File Transfer TPS Test"
echo "=========================================="
echo "File size: %s"
echo ""

i=0
while [ $i -lt $ITERATIONS ]; do
  i=$((i + 1))
  echo "--- HTTP iteration $i/$ITERATIONS ---"

  COUNT=0
  BYTES=0
  START=$(date +%%s)
  END=$((START + DURATION))

  while [ "$(date +%%s)" -lt "$END" ]; do
    if curl -s -o /dev/null -w '' "http://${SERVER}:8080/testfile"; then
      COUNT=$((COUNT + 1))
      BYTES=$((BYTES + FILE_SIZE_BYTES))
    fi
  done

  ELAPSED=$(($(date +%%s) - START))
  if [ "$ELAPSED" -gt 0 ]; then
    TPS=$((COUNT / ELAPSED))
    MBPS=$((BYTES / ELAPSED / 1048576))
  else
    TPS=0
    MBPS=0
  fi

  echo "Completed:  $COUNT transfers in ${ELAPSED}s"
  echo "Throughput: ${TPS} transfers/sec, ${MBPS} MB/s"
  echo ""
  sleep 2
done

echo "=========================================="
echo "  TPS testing complete"
echo "=========================================="
`, dnsName, w.GetParam("iterations"), w.GetParam("duration"), fileSizeBytes, w.GetParam("file-size"))

	return w.BuildCloudConfig(CloudConfigOpts{
		Packages: []string{"netperf", "curl"},
		WriteFiles: []WriteFile{
			{
				Path:        "/usr/local/bin/virtwork-tps-client.sh",
				Content:     clientScript,
				Permissions: "0755",
			},
			{
				Path:        "/etc/systemd/system/virtwork-tps.service",
				Content:     tpsClientSystemdUnit,
				Permissions: "0644",
			},
		},
		RunCmd: [][]string{
			{"systemctl", "daemon-reload"},
			{"systemctl", "enable", "--now", "virtwork-tps.service"},
		},
	})
}
