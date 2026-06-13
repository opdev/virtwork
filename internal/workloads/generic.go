// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads

import (
	"github.com/opdev/virtwork/internal/config"
)

// GenericWorkload implements Workload for single-role catalog entries.
type GenericWorkload struct {
	BaseWorkload
	entryName    string
	serviceFiles map[string]string
	packages     []string
}

// NewGenericWorkload creates a GenericWorkload from a loaded catalog entry.
func NewGenericWorkload(
	cfg config.WorkloadConfig,
	entry *CatalogEntry,
	sshUser, sshPassword string,
	sshKeys []string,
) *GenericWorkload {
	return &GenericWorkload{
		BaseWorkload: BaseWorkload{
			Config:            cfg,
			ParamSchema:       entry.Schema(),
			SSHUser:           sshUser,
			SSHPassword:       sshPassword,
			SSHAuthorizedKeys: sshKeys,
		},
		entryName:    entry.Name,
		serviceFiles: entry.ServiceFiles,
		packages:     entry.Manifest.Packages,
	}
}

// Name returns the catalog entry name.
func (w *GenericWorkload) Name() string {
	return w.entryName
}

// CloudInitUserdata returns cloud-init YAML with the entry's service files installed.
func (w *GenericWorkload) CloudInitUserdata() (string, error) {
	return "", nil // stub — implemented in Phase 2
}
