// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package orchestrator

import (
	"fmt"

	kubevirtv1 "kubevirt.io/api/core/v1"
)

// VMSpecInput holds the inputs needed to build a KubeVirt VirtualMachine spec.
type VMSpecInput struct {
	Name                string
	Namespace           string
	ContainerDiskImage  string
	CloudInitUserdata   string
	CloudInitSecretName string
	CPUCores            int
	Memory              string
	Labels              map[string]string
	ExtraDisks          []kubevirtv1.Disk
	ExtraVolumes        []kubevirtv1.Volume
	DataVolumeTemplates []kubevirtv1.DataVolumeTemplateSpec
}

// VMPlan describes a single VM to be created during orchestration.
type VMPlan struct {
	WorkloadName string
	VMName       string
	Component    string
	Role         string
	VMSpec       *VMSpecInput
}

// RunResult holds the summary of a completed run.
type RunResult struct {
	RunID        string
	VMCount      int
	ServiceCount int
	SecretCount  int
}

// NamespaceDataVolumes appends the VM name to DataVolume template names and
// updates corresponding volume references to prevent name collisions when
// deploying multiple VMs of the same workload type.
func NamespaceDataVolumes(
	baseTemplates []kubevirtv1.DataVolumeTemplateSpec,
	baseVolumes []kubevirtv1.Volume,
	vmName string,
) ([]kubevirtv1.DataVolumeTemplateSpec, []kubevirtv1.Volume) {
	if len(baseTemplates) == 0 {
		return baseTemplates, baseVolumes
	}

	nameMap := make(map[string]string, len(baseTemplates))
	templates := make([]kubevirtv1.DataVolumeTemplateSpec, len(baseTemplates))
	for i, tmpl := range baseTemplates {
		oldName := tmpl.Name
		newName := fmt.Sprintf("%s-%s", oldName, vmName)
		nameMap[oldName] = newName

		templates[i] = tmpl
		templates[i].Name = newName
	}

	volumes := make([]kubevirtv1.Volume, len(baseVolumes))
	for i, vol := range baseVolumes {
		volumes[i] = vol
		if vol.DataVolume != nil {
			if newName, ok := nameMap[vol.DataVolume.Name]; ok {
				volumes[i].DataVolume = &kubevirtv1.DataVolumeSource{
					Name: newName,
				}
			}
		}
	}

	return templates, volumes
}
