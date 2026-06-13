// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads

import (
	"errors"
	"fmt"
	"strings"

	"github.com/opdev/virtwork/internal/config"
)

var ErrUnknownCatalogRole = errors.New("unknown catalog role")

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
	if len(w.roles) == 0 {
		return w.BuildCloudConfig(CloudConfigOpts{})
	}
	return w.UserdataForRole(w.roles[0].Name, w.namespace)
}

// RoleDistribution returns per-role VM counts from the manifest.
func (w *GenericMultiWorkload) RoleDistribution() []RoleSpec {
	specs := make([]RoleSpec, len(w.roles))
	for i, rd := range w.roles {
		count := rd.VMCount
		if count < 1 {
			count = max(1, w.Config.VMCount)
		}
		specs[i] = RoleSpec{Role: rd.Name, VMCount: count}
	}
	return specs
}

// VMCount returns the total VM count across all roles.
func (w *GenericMultiWorkload) VMCount() int {
	total := 0
	for _, rs := range w.RoleDistribution() {
		total += rs.VMCount
	}
	return total
}

// UserdataForRole returns cloud-init YAML for a specific role.
func (w *GenericMultiWorkload) UserdataForRole(role string, _ string) (string, error) {
	svcContent, ok := w.serviceFiles[role]
	if !ok {
		return "", fmt.Errorf("role %q: %w", role, ErrUnknownCatalogRole)
	}

	content := w.substituteParams(svcContent)
	svcFilename := role + ".service"

	return w.BuildCloudConfig(CloudConfigOpts{
		Packages: w.packages,
		WriteFiles: []WriteFile{
			{
				Path:        "/etc/systemd/system/" + svcFilename,
				Content:     content,
				Permissions: "0644",
			},
		},
		RunCmd: [][]string{
			{"systemctl", "daemon-reload"},
			{"systemctl", "enable", "--now", svcFilename},
		},
	})
}

func (w *GenericMultiWorkload) substituteParams(content string) string {
	for _, p := range w.ParamSchema {
		content = strings.ReplaceAll(content, "{{"+p.Key+"}}", w.GetParam(p.Key))
	}
	return content
}
