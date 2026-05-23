// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads

import (
	"fmt"

	"github.com/opdev/virtwork/internal/config"
)

const chaosProcessScript = `#!/bin/bash
# Chaos process workload - randomly kills non-essential processes
set -euo pipefail

# Configuration via environment variables with defaults
CHAOS_SIGNAL="${CHAOS_SIGNAL:-SIGTERM}"
CHAOS_INTERVAL="${CHAOS_INTERVAL:-30}"
CHAOS_MIN_PID="${CHAOS_MIN_PID:-1000}"

# List of essential processes to exclude from chaos
# These are critical system processes that should never be killed
EXCLUDED_PATTERNS=(
	"systemd"
	"sshd"
	"dbus"
	"agetty"
	"auditd"
	"rsyslogd"
	"chronyd"
	"NetworkManager"
	"bash"
	"sh"
	"cloud-init"
	"virtwork-"
)

log() {
	echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"
}

build_exclusion_filter() {
	local filter=""
	for pattern in "${EXCLUDED_PATTERNS[@]}"; do
		filter="${filter}${pattern}|"
	done
	# Remove trailing pipe and return
	echo "${filter%|}"
}

select_random_process() {
	local exclusion_filter
	exclusion_filter=$(build_exclusion_filter)

	# Get list of PIDs for non-essential processes
	# - Exclude PID 1 (init/systemd)
	# - Exclude PIDs below CHAOS_MIN_PID (kernel threads and core system processes)
	# - Exclude processes matching EXCLUDED_PATTERNS
	# - Exclude this script itself
	local pids
	pids=$(ps -eo pid,comm --no-headers | \
		awk -v min_pid="$CHAOS_MIN_PID" '$1 >= min_pid && $1 != '"$$"' {print $1, $2}' | \
		grep -Ev "($exclusion_filter)" | \
		awk '{print $1}' || true)

	if [ -z "$pids" ]; then
		log "No eligible processes found for chaos action"
		return 1
	fi

	# Convert to array and select random PID
	local pid_array=($pids)
	local count=${#pid_array[@]}
	local random_index=$((RANDOM % count))
	echo "${pid_array[$random_index]}"
}

kill_random_process() {
	local target_pid
	target_pid=$(select_random_process) || return 0

	# Get process info before killing
	local process_info
	process_info=$(ps -p "$target_pid" -o pid,comm,args --no-headers 2>/dev/null || echo "unknown")

	log "Sending $CHAOS_SIGNAL to PID $target_pid: $process_info"

	if kill -"$CHAOS_SIGNAL" "$target_pid" 2>/dev/null; then
		log "Successfully sent $CHAOS_SIGNAL to PID $target_pid"
	else
		log "Failed to send signal to PID $target_pid (process may have already exited)"
	fi
}

main() {
	log "Starting chaos-process workload"
	log "Configuration: SIGNAL=$CHAOS_SIGNAL INTERVAL=${CHAOS_INTERVAL}s MIN_PID=$CHAOS_MIN_PID"

	while true; do
		kill_random_process
		sleep "$CHAOS_INTERVAL"
	done
}

main "$@"
`

const chaosProcessSystemdUnitTemplate = `[Unit]
Description=Virtwork chaos-process workload
After=network.target

[Service]
Type=simple
Environment="CHAOS_SIGNAL=%s"
Environment="CHAOS_INTERVAL=%s"
Environment="CHAOS_MIN_PID=%s"
ExecStart=/usr/local/bin/chaos-process.sh
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
`

// ChaosProcessWorkload generates cloud-init userdata for a process chaos workload
// that randomly kills non-essential processes to test resilience.
type ChaosProcessWorkload struct {
	BaseWorkload
}

// NewChaosProcessWorkload creates a ChaosProcessWorkload with the given configuration and SSH credentials.
func NewChaosProcessWorkload(
	cfg config.WorkloadConfig,
	sshUser, sshPassword string,
	sshKeys []string,
) *ChaosProcessWorkload {
	return &ChaosProcessWorkload{
		BaseWorkload: BaseWorkload{
			Config:            cfg,
			SSHUser:           sshUser,
			SSHPassword:       sshPassword,
			SSHAuthorizedKeys: sshKeys,
		},
	}
}

func (w *ChaosProcessWorkload) signal() string {
	if w.Config.Params != nil {
		if val, ok := w.Config.Params["signal"]; ok && val != "" {
			return val
		}
	}
	return "SIGTERM"
}

func (w *ChaosProcessWorkload) interval() string {
	if w.Config.Params != nil {
		if val, ok := w.Config.Params["interval"]; ok && val != "" {
			return val
		}
	}
	return "30"
}

func (w *ChaosProcessWorkload) minPid() string {
	if w.Config.Params != nil {
		if val, ok := w.Config.Params["min-pid"]; ok && val != "" {
			return val
		}
	}
	return "1000"
}

// Name returns "chaos-process".
func (w *ChaosProcessWorkload) Name() string {
	return "chaos-process"
}

// CloudInitUserdata returns cloud-init YAML that installs the chaos-process script
// and runs it as a systemd service.
func (w *ChaosProcessWorkload) CloudInitUserdata() (string, error) {
	unit := fmt.Sprintf(chaosProcessSystemdUnitTemplate,
		w.signal(),
		w.interval(),
		w.minPid())

	return w.BuildCloudConfig(CloudConfigOpts{
		Packages: []string{"procps-ng"},
		WriteFiles: []WriteFile{
			{
				Path:        "/usr/local/bin/chaos-process.sh",
				Content:     chaosProcessScript,
				Permissions: "0755",
			},
			{
				Path:        "/etc/systemd/system/virtwork-chaos-process.service",
				Content:     unit,
				Permissions: "0644",
			},
		},
		RunCmd: [][]string{
			{"systemctl", "daemon-reload"},
			{"systemctl", "enable", "--now", "virtwork-chaos-process.service"},
		},
	})
}
