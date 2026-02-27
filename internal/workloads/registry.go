// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/constants"
)

var ErrWorkloadUnknown = errors.New("workload not found")

// RegistryOpts holds optional parameters for workload construction.
// Fields are populated via functional Option values.
type RegistryOpts struct {
	Namespace         string
	DataDiskSize      string
	SSHUser           string
	SSHPassword       string
	SSHAuthorizedKeys []string
}

// Option is a functional option for workload construction.
type Option func(*RegistryOpts)

// WithNamespace sets the namespace used by workloads that need it (e.g., network).
func WithNamespace(ns string) Option {
	return func(o *RegistryOpts) { o.Namespace = ns }
}

// WithSSHCredentials sets SSH user, password, and authorized keys for the workload VMs.
func WithSSHCredentials(user, password string, keys []string) Option {
	return func(o *RegistryOpts) {
		o.SSHUser = user
		o.SSHPassword = password
		o.SSHAuthorizedKeys = keys
	}
}

// WithDataDiskSize sets the data disk size for workloads that use persistent storage.
func WithDataDiskSize(size string) Option {
	return func(o *RegistryOpts) { o.DataDiskSize = size }
}

// WorkloadFactory creates a Workload from a WorkloadConfig and resolved options.
type WorkloadFactory func(config.WorkloadConfig, *RegistryOpts) Workload

// Registry maps workload names to their factory functions.
type Registry map[string]WorkloadFactory

// AllWorkloadNames is a sorted list of all built-in workload names.
var AllWorkloadNames = []string{"cpu", "database", "disk", "memory", "network"}

// DefaultRegistry returns a Registry pre-populated with all built-in workloads.
func DefaultRegistry() Registry {
	return Registry{
		"cpu": func(cfg config.WorkloadConfig, opts *RegistryOpts) Workload {
			return NewCPUWorkload(cfg, opts.SSHUser, opts.SSHPassword, opts.SSHAuthorizedKeys)
		},
		"memory": func(cfg config.WorkloadConfig, opts *RegistryOpts) Workload {
			return NewMemoryWorkload(cfg, opts.SSHUser, opts.SSHPassword, opts.SSHAuthorizedKeys)
		},
		"disk": func(cfg config.WorkloadConfig, opts *RegistryOpts) Workload {
			return NewDiskWorkload(cfg, opts.DataDiskSize, opts.SSHUser, opts.SSHPassword, opts.SSHAuthorizedKeys)
		},
		"database": func(cfg config.WorkloadConfig, opts *RegistryOpts) Workload {
			return NewDatabaseWorkload(cfg, opts.DataDiskSize, opts.SSHUser, opts.SSHPassword, opts.SSHAuthorizedKeys)
		},
		"network": func(cfg config.WorkloadConfig, opts *RegistryOpts) Workload {
			return NewNetworkWorkload(cfg, opts.Namespace, opts.SSHUser, opts.SSHPassword, opts.SSHAuthorizedKeys)
		},
	}
}

// Get retrieves a workload by name, constructing it with the given config and options.
// Returns an error listing available names if the workload is not found.
func (r Registry) Get(name string, cfg config.WorkloadConfig, opts ...Option) (Workload, error) {
	factory, ok := r[name]
	if !ok {
		return nil, fmt.Errorf(
			"workload %q not found; available: %s; %w",
			name,
			strings.Join(r.List(), ", "),
			ErrWorkloadUnknown,
		)
	}

	resolved := &RegistryOpts{
		DataDiskSize: constants.DefaultDiskSize,
	}
	for _, opt := range opts {
		opt(resolved)
	}

	return factory(cfg, resolved), nil
}

// List returns all registered workload names in sorted order.
func (r Registry) List() []string {
	names := make([]string, 0, len(r))
	for name := range r {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
