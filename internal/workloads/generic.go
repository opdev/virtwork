// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads

import (
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/constants"
	"github.com/opdev/virtwork/internal/vm"
)

// GenericWorkload implements Workload for single-role catalog entries.
type GenericWorkload struct {
	BaseWorkload
	entryName    string
	namespace    string
	serviceFiles map[string]string
	packages     []string
	storageSpecs []StorageDefinition
	serviceDef   *ServiceDefinition
}

// NewGenericWorkload creates a GenericWorkload from a loaded catalog entry.
func NewGenericWorkload(
	cfg config.WorkloadConfig,
	entry *CatalogEntry,
	namespace, sshUser, sshPassword string,
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
		namespace:    namespace,
		serviceFiles: entry.ServiceFiles,
		packages:     entry.Manifest.Packages,
		storageSpecs: entry.Manifest.Storage,
		serviceDef:   entry.Manifest.Service,
	}
}

// Name returns the catalog entry name.
func (w *GenericWorkload) Name() string {
	return w.entryName
}

// CloudInitUserdata returns cloud-init YAML with the entry's service files installed.
func (w *GenericWorkload) CloudInitUserdata() (string, error) {
	names := make([]string, 0, len(w.serviceFiles))
	for name := range w.serviceFiles {
		names = append(names, name)
	}
	sort.Strings(names)

	writeFiles := make([]WriteFile, 0, len(w.storageSpecs)+len(names))
	runcmd := make([][]string, 0, len(w.storageSpecs)+1+len(names))

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

	runcmd = append(runcmd, []string{"systemctl", "daemon-reload"})

	for _, name := range names {
		content := w.substituteParams(w.serviceFiles[name])
		writeFiles = append(writeFiles, WriteFile{
			Path:        "/etc/systemd/system/" + name,
			Content:     content,
			Permissions: "0644",
		})
		runcmd = append(runcmd, []string{"systemctl", "enable", "--now", name})
	}

	return w.BuildCloudConfig(CloudConfigOpts{
		Packages:   w.packages,
		WriteFiles: writeFiles,
		RunCmd:     runcmd,
	})
}

// DataVolumeTemplates returns CDI DataVolumeTemplateSpecs for declared storage.
func (w *GenericWorkload) DataVolumeTemplates() ([]kubevirtv1.DataVolumeTemplateSpec, error) {
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
func (w *GenericWorkload) ExtraDisks() []kubevirtv1.Disk {
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
func (w *GenericWorkload) ExtraVolumes() []kubevirtv1.Volume {
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
func (w *GenericWorkload) RequiresService() bool {
	return w.serviceDef != nil
}

// ServiceSpec returns the K8s Service, or nil if not declared.
func (w *GenericWorkload) ServiceSpec() *corev1.Service {
	return buildCatalogServiceSpec(w.Name(), w.namespace, w.serviceDef)
}

func (w *GenericWorkload) substituteParams(content string) string {
	for _, p := range w.ParamSchema {
		content = strings.ReplaceAll(content, "{{"+p.Key+"}}", w.GetParam(p.Key))
	}
	return content
}
