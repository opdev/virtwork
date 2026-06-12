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

var (
	ErrWorkloadUnknown = errors.New("workload not found")
	ErrParamUnknown    = errors.New("unknown param")
)

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

// RegistryEntry pairs a workload factory with its param schema.
type RegistryEntry struct {
	Factory     WorkloadFactory
	ParamSchema ParamSchema
}

// Registry maps workload names to their registry entries.
type Registry map[string]RegistryEntry

// AllWorkloadNames returns a sorted list of all built-in workload names,
// derived from the default registry.
func AllWorkloadNames() []string {
	return DefaultRegistry().List()
}

// DefaultRegistry returns a Registry pre-populated with all built-in workloads.
func DefaultRegistry() Registry {
	return Registry{
		"chaos-process": {
			Factory: func(cfg config.WorkloadConfig, opts *RegistryOpts) Workload {
				return NewChaosProcessWorkload(
					cfg,
					opts.SSHUser,
					opts.SSHPassword,
					opts.SSHAuthorizedKeys,
				)
			},
			ParamSchema: ChaosProcessParamSchema,
		},
		"chaos-network": {
			Factory: func(cfg config.WorkloadConfig, opts *RegistryOpts) Workload {
				return NewChaosNetworkWorkload(
					cfg,
					opts.SSHUser,
					opts.SSHPassword,
					opts.SSHAuthorizedKeys,
				)
			},
			ParamSchema: ChaosNetworkParamSchema,
		},
		"chaos-disk": {
			Factory: func(cfg config.WorkloadConfig, opts *RegistryOpts) Workload {
				return NewChaosDiskWorkload(
					cfg,
					opts.DataDiskSize,
					opts.SSHUser,
					opts.SSHPassword,
					opts.SSHAuthorizedKeys,
				)
			},
			ParamSchema: ChaosDiskParamSchema,
		},
		"cpu": {
			Factory: func(cfg config.WorkloadConfig, opts *RegistryOpts) Workload {
				return NewCPUWorkload(cfg, opts.SSHUser, opts.SSHPassword, opts.SSHAuthorizedKeys)
			},
			ParamSchema: CPUParamSchema,
		},
		"memory": {
			Factory: func(cfg config.WorkloadConfig, opts *RegistryOpts) Workload {
				return NewMemoryWorkload(
					cfg,
					opts.SSHUser,
					opts.SSHPassword,
					opts.SSHAuthorizedKeys,
				)
			},
			ParamSchema: MemoryParamSchema,
		},
		"disk": {
			Factory: func(cfg config.WorkloadConfig, opts *RegistryOpts) Workload {
				return NewDiskWorkload(
					cfg,
					opts.DataDiskSize,
					opts.SSHUser,
					opts.SSHPassword,
					opts.SSHAuthorizedKeys,
				)
			},
			ParamSchema: DiskParamSchema,
		},
		"database": {
			Factory: func(cfg config.WorkloadConfig, opts *RegistryOpts) Workload {
				return NewDatabaseWorkload(
					cfg,
					opts.DataDiskSize,
					opts.SSHUser,
					opts.SSHPassword,
					opts.SSHAuthorizedKeys,
				)
			},
			ParamSchema: DatabaseParamSchema,
		},
		"network": {
			Factory: func(cfg config.WorkloadConfig, opts *RegistryOpts) Workload {
				return NewNetworkWorkload(
					cfg,
					opts.Namespace,
					opts.SSHUser,
					opts.SSHPassword,
					opts.SSHAuthorizedKeys,
				)
			},
			ParamSchema: NetworkParamSchema,
		},
		"tps": {
			Factory: func(cfg config.WorkloadConfig, opts *RegistryOpts) Workload {
				return NewTPSWorkload(
					cfg,
					opts.Namespace,
					opts.SSHUser,
					opts.SSHPassword,
					opts.SSHAuthorizedKeys,
				)
			},
			ParamSchema: TPSParamSchema,
		},
	}
}

// ValidateWorkloadNames checks that all names are valid workload names from the
// default registry. Returns an error listing invalid names with "did you mean?"
// suggestions for close matches.
func ValidateWorkloadNames(names []string) error {
	if len(names) == 0 {
		return fmt.Errorf("no workloads specified; available: %s; %w",
			strings.Join(AllWorkloadNames(), ", "), ErrWorkloadUnknown)
	}

	valid := DefaultRegistry()
	var invalid []string
	for _, name := range names {
		if _, ok := valid[name]; !ok {
			suggestion := closestMatch(name, valid.List())
			if suggestion != "" {
				invalid = append(invalid, fmt.Sprintf("%q (did you mean %q?)", name, suggestion))
			} else {
				invalid = append(invalid, fmt.Sprintf("%q", name))
			}
		}
	}
	if len(invalid) > 0 {
		return fmt.Errorf(
			"unknown workload(s): %s; available: %s; %w",
			strings.Join(invalid, ", "),
			strings.Join(valid.List(), ", "),
			ErrWorkloadUnknown,
		)
	}
	return nil
}

func closestMatch(input string, candidates []string) string {
	best := ""
	bestDist := len(input)/2 + 1
	for _, c := range candidates {
		d := levenshtein(input, c)
		if d < bestDist {
			bestDist = d
			best = c
		}
	}
	return best
}

func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := range prev {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(curr[j-1]+1, min(prev[j]+1, prev[j-1]+cost))
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

// Get retrieves a workload by name, constructing it with the given config and options.
// Returns an error listing available names if the workload is not found.
func (r Registry) Get(name string, cfg config.WorkloadConfig, opts ...Option) (Workload, error) {
	entry, ok := r[name]
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

	return entry.Factory(cfg, resolved), nil
}

// ValidateParams checks that all param keys are declared in the workload's
// schema and that values conform to their declared types. Returns an error
// with "did you mean?" suggestions for unknown keys.
func (r Registry) ValidateParams(workload string, params map[string]string) error {
	entry, ok := r[workload]
	if !ok {
		return fmt.Errorf("unknown workload %q; %w", workload, ErrWorkloadUnknown)
	}
	schema := entry.ParamSchema
	for key, val := range params {
		def := schema.Find(key)
		if def == nil {
			keys := make([]string, len(schema))
			for i, d := range schema {
				keys[i] = d.Key
			}
			suggestion := closestMatch(key, keys)
			if suggestion != "" {
				return fmt.Errorf(
					"%w %q for workload %q (did you mean %q?)",
					ErrParamUnknown, key, workload, suggestion,
				)
			}
			return fmt.Errorf("%w %q for workload %q", ErrParamUnknown, key, workload)
		}
		if err := def.Validate(val); err != nil {
			return fmt.Errorf("param %q for workload %q: %w", key, workload, err)
		}
	}
	return nil
}

// AllParamSchemas returns the param schema for every registered workload.
func (r Registry) AllParamSchemas() map[string]ParamSchema {
	schemas := make(map[string]ParamSchema, len(r))
	for name, entry := range r {
		if len(entry.ParamSchema) > 0 {
			schemas[name] = entry.ParamSchema
		}
	}
	return schemas
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
