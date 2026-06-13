// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads

import (
	"errors"
	"fmt"
	"strings"

	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/constants"
	"github.com/opdev/virtwork/internal/vm"

	corev1 "k8s.io/api/core/v1"
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
	storageSpecs []StorageDefinition
	serviceDef   *ServiceDefinition
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
		storageSpecs: entry.Manifest.Storage,
		serviceDef:   entry.Manifest.Service,
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

	writeFiles := make([]WriteFile, 0, len(w.storageSpecs)+1)
	runcmd := make([][]string, 0, len(w.storageSpecs)+2)

	for _, spec := range w.storageSpecs {
		writeFiles = append(writeFiles, WriteFile{
			Path:        fmt.Sprintf("/usr/local/bin/virtwork-disk-setup-%s.sh", spec.Name),
			Content:     diskSetupScript(spec.Serial, spec.Mount),
			Permissions: "0755",
		})
		runcmd = append(runcmd, []string{
			fmt.Sprintf("/usr/local/bin/virtwork-disk-setup-%s.sh", spec.Name),
		})
	}

	writeFiles = append(writeFiles, WriteFile{
		Path:        "/etc/systemd/system/" + svcFilename,
		Content:     content,
		Permissions: "0644",
	})

	runcmd = append(runcmd,
		[]string{"systemctl", "daemon-reload"},
		[]string{"systemctl", "enable", "--now", svcFilename},
	)

	return w.BuildCloudConfig(CloudConfigOpts{
		Packages:   w.packages,
		WriteFiles: writeFiles,
		RunCmd:     runcmd,
	})
}

// DataVolumeTemplates returns CDI DataVolumeTemplateSpecs for declared storage.
func (w *GenericMultiWorkload) DataVolumeTemplates() ([]kubevirtv1.DataVolumeTemplateSpec, error) {
	if len(w.storageSpecs) == 0 {
		return nil, nil
	}
	dvts := make([]kubevirtv1.DataVolumeTemplateSpec, len(w.storageSpecs))
	for i, spec := range w.storageSpecs {
		dvt, err := vm.BuildDataVolumeTemplate(spec.Name, spec.Size)
		if err != nil {
			return nil, fmt.Errorf("building data volume for storage %q: %w", spec.Name, err)
		}
		dvts[i] = dvt
	}
	return dvts, nil
}

// ExtraDisks returns disk definitions for all declared storage.
func (w *GenericMultiWorkload) ExtraDisks() []kubevirtv1.Disk {
	if len(w.storageSpecs) == 0 {
		return nil
	}
	disks := make([]kubevirtv1.Disk, len(w.storageSpecs))
	for i, spec := range w.storageSpecs {
		disks[i] = kubevirtv1.Disk{
			Name:   spec.Name,
			Serial: spec.Serial,
			DiskDevice: kubevirtv1.DiskDevice{
				Disk: &kubevirtv1.DiskTarget{
					Bus: constants.DiskBusVirtio,
				},
			},
		}
	}
	return disks
}

// ExtraVolumes returns volume definitions for all declared storage.
func (w *GenericMultiWorkload) ExtraVolumes() []kubevirtv1.Volume {
	if len(w.storageSpecs) == 0 {
		return nil
	}
	volumes := make([]kubevirtv1.Volume, len(w.storageSpecs))
	for i, spec := range w.storageSpecs {
		volumes[i] = kubevirtv1.Volume{
			Name: spec.Name,
			VolumeSource: kubevirtv1.VolumeSource{
				DataVolume: &kubevirtv1.DataVolumeSource{
					Name: spec.Name,
				},
			},
		}
	}
	return volumes
}

// RequiresService returns true if a service is declared in the manifest.
func (w *GenericMultiWorkload) RequiresService() bool {
	return w.serviceDef != nil
}

// ServiceSpec returns the K8s Service, or nil if not declared.
func (w *GenericMultiWorkload) ServiceSpec() *corev1.Service {
	return buildCatalogServiceSpec(w.Name(), w.namespace, w.serviceDef)
}

func (w *GenericMultiWorkload) substituteParams(content string) string {
	for _, p := range w.ParamSchema {
		content = strings.ReplaceAll(content, "{{"+p.Key+"}}", w.GetParam(p.Key))
	}
	return content
}
