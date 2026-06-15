// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package orchestrator_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/opdev/virtwork/internal/orchestrator"
)

var _ = Describe("NamespaceDataVolumes", func() {
	It("should return inputs unchanged when there are no templates", func() {
		var templates []kubevirtv1.DataVolumeTemplateSpec
		volumes := []kubevirtv1.Volume{
			{Name: "containerdisk"},
		}

		outTemplates, outVolumes := orchestrator.NamespaceDataVolumes(templates, volumes, "vm-0")
		Expect(outTemplates).To(BeEmpty())
		Expect(outVolumes).To(Equal(volumes))
	})

	It("should append VM name to DataVolume template names", func() {
		templates := []kubevirtv1.DataVolumeTemplateSpec{
			{ObjectMeta: metav1.ObjectMeta{Name: "data"}},
		}
		volumes := []kubevirtv1.Volume{
			{
				Name: "datadisk",
				VolumeSource: kubevirtv1.VolumeSource{
					DataVolume: &kubevirtv1.DataVolumeSource{Name: "data"},
				},
			},
		}

		outTemplates, outVolumes := orchestrator.NamespaceDataVolumes(templates, volumes, "virtwork-disk-0")
		Expect(outTemplates).To(HaveLen(1))
		Expect(outTemplates[0].Name).To(Equal("data-virtwork-disk-0"))
		Expect(outVolumes[0].DataVolume.Name).To(Equal("data-virtwork-disk-0"))
	})

	It("should handle multiple DataVolume templates", func() {
		templates := []kubevirtv1.DataVolumeTemplateSpec{
			{ObjectMeta: metav1.ObjectMeta{Name: "wal"}},
			{ObjectMeta: metav1.ObjectMeta{Name: "pgdata"}},
		}
		volumes := []kubevirtv1.Volume{
			{
				Name: "wal-vol",
				VolumeSource: kubevirtv1.VolumeSource{
					DataVolume: &kubevirtv1.DataVolumeSource{Name: "wal"},
				},
			},
			{
				Name: "pgdata-vol",
				VolumeSource: kubevirtv1.VolumeSource{
					DataVolume: &kubevirtv1.DataVolumeSource{Name: "pgdata"},
				},
			},
		}

		outTemplates, outVolumes := orchestrator.NamespaceDataVolumes(templates, volumes, "virtwork-db-1")
		Expect(outTemplates).To(HaveLen(2))
		Expect(outTemplates[0].Name).To(Equal("wal-virtwork-db-1"))
		Expect(outTemplates[1].Name).To(Equal("pgdata-virtwork-db-1"))
		Expect(outVolumes[0].DataVolume.Name).To(Equal("wal-virtwork-db-1"))
		Expect(outVolumes[1].DataVolume.Name).To(Equal("pgdata-virtwork-db-1"))
	})

	It("should not modify volumes without DataVolume source", func() {
		templates := []kubevirtv1.DataVolumeTemplateSpec{
			{ObjectMeta: metav1.ObjectMeta{Name: "data"}},
		}
		volumes := []kubevirtv1.Volume{
			{Name: "containerdisk"},
			{
				Name: "datadisk",
				VolumeSource: kubevirtv1.VolumeSource{
					DataVolume: &kubevirtv1.DataVolumeSource{Name: "data"},
				},
			},
		}

		_, outVolumes := orchestrator.NamespaceDataVolumes(templates, volumes, "vm-0")
		Expect(outVolumes[0].DataVolume).To(BeNil())
		Expect(outVolumes[1].DataVolume.Name).To(Equal("data-vm-0"))
	})

	It("should not modify volumes referencing unknown DataVolumes", func() {
		templates := []kubevirtv1.DataVolumeTemplateSpec{
			{ObjectMeta: metav1.ObjectMeta{Name: "data"}},
		}
		volumes := []kubevirtv1.Volume{
			{
				Name: "other",
				VolumeSource: kubevirtv1.VolumeSource{
					DataVolume: &kubevirtv1.DataVolumeSource{Name: "not-in-templates"},
				},
			},
		}

		_, outVolumes := orchestrator.NamespaceDataVolumes(templates, volumes, "vm-0")
		Expect(outVolumes[0].DataVolume.Name).To(Equal("not-in-templates"))
	})
})

var _ = Describe("VMPlan", func() {
	It("should be constructible with all fields", func() {
		plan := orchestrator.VMPlan{
			WorkloadName: "cpu",
			VMName:       "virtwork-cpu-0",
			Component:    "cpu",
			Role:         "",
			VMSpec: &orchestrator.VMSpecInput{
				Name:               "virtwork-cpu-0",
				Namespace:          "virtwork",
				ContainerDiskImage: "quay.io/containerdisks/fedora:42",
				CloudInitUserdata:  "#cloud-config\n",
				CPUCores:           2,
				Memory:             "2Gi",
				Labels: map[string]string{
					"app.kubernetes.io/name": "virtwork-cpu",
				},
			},
		}
		Expect(plan.VMName).To(Equal("virtwork-cpu-0"))
		Expect(plan.Component).To(Equal("cpu"))
		Expect(plan.VMSpec.CPUCores).To(Equal(2))
	})

	It("should support role field for multi-VM workloads", func() {
		plan := orchestrator.VMPlan{
			WorkloadName: "network",
			VMName:       "virtwork-network-server-0",
			Component:    "network",
			Role:         "server",
			VMSpec: &orchestrator.VMSpecInput{
				Name:      "virtwork-network-server-0",
				Namespace: "virtwork",
				Labels: map[string]string{
					"virtwork/role": "server",
				},
			},
		}
		Expect(plan.Role).To(Equal("server"))
		Expect(plan.VMSpec.Labels["virtwork/role"]).To(Equal("server"))
	})
})

var _ = Describe("RunResult", func() {
	It("should hold summary counts", func() {
		result := orchestrator.RunResult{
			RunID:        "abc-123",
			VMCount:      5,
			ServiceCount: 1,
			SecretCount:  5,
		}
		Expect(result.RunID).To(Equal("abc-123"))
		Expect(result.VMCount).To(Equal(5))
		Expect(result.ServiceCount).To(Equal(1))
		Expect(result.SecretCount).To(Equal(5))
	})
})

var _ = Describe("NewRunOrchestrator", func() {
	It("should construct with all dependencies", func() {
		ro := orchestrator.NewRunOrchestrator(nil, nil, nil, nil, nil)
		Expect(ro).NotTo(BeNil())
	})
})

var _ = Describe("NewCleanupOrchestrator", func() {
	It("should construct with all dependencies", func() {
		co := orchestrator.NewCleanupOrchestrator(nil, nil, nil, nil, nil)
		Expect(co).NotTo(BeNil())
	})
})

// Ensure ObjectMeta Name field is accessible in test context.
var _ = fmt.Sprintf
