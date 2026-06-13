// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads

import (
	"github.com/opdev/virtwork/internal/config"
)

// GenericMultiWorkload implements MultiVMWorkload for multi-role catalog entries.
type GenericMultiWorkload struct {
	BaseWorkload
	entryName    string
	namespace    string
	roles        []RoleDefinition
	serviceFiles map[string]string
	packages     []string
}

// NewGenericMultiWorkload creates a GenericMultiWorkload from a loaded catalog entry.
func NewGenericMultiWorkload(
	cfg config.WorkloadConfig,
	entry *CatalogEntry,
	namespace, sshUser, sshPassword string,
	sshKeys []string,
) *GenericMultiWorkload {
	return &GenericMultiWorkload{
		BaseWorkload: BaseWorkload{
			Config:            cfg,
			ParamSchema:       entry.Schema(),
			SSHUser:           sshUser,
			SSHPassword:       sshPassword,
			SSHAuthorizedKeys: sshKeys,
		},
		entryName:    entry.Name,
		namespace:    namespace,
		roles:        entry.Manifest.Roles,
		serviceFiles: entry.ServiceFiles,
		packages:     entry.Manifest.Packages,
	}
}

// Name returns the catalog entry name.
func (w *GenericMultiWorkload) Name() string {
	return w.entryName
}

// CloudInitUserdata returns the first role's userdata as the default.
func (w *GenericMultiWorkload) CloudInitUserdata() (string, error) {
	return "", nil // stub — implemented in Phase 3
}

// RoleDistribution returns per-role VM counts from the manifest.
func (w *GenericMultiWorkload) RoleDistribution() []RoleSpec {
	return nil // stub — implemented in Phase 3
}

// UserdataForRole returns cloud-init YAML for a specific role.
func (w *GenericMultiWorkload) UserdataForRole(role string, namespace string) (string, error) {
	return "", nil // stub — implemented in Phase 3
}
