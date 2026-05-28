// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package constants_test

import (
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/opdev/virtwork/internal/constants"
)

var _ = Describe("Constants", func() {
	Context("KubeVirt API coordinates", func() {
		It("should have correct KubeVirt API group", func() {
			Expect(constants.KubevirtAPIGroup).To(Equal("kubevirt.io"))
		})

		It("should have correct KubeVirt API version", func() {
			Expect(constants.KubevirtAPIVersion).To(Equal("v1"))
		})

		It("should have correct KubeVirt VM plural", func() {
			Expect(constants.KubevirtVMPlural).To(Equal("virtualmachines"))
		})

		It("should have correct KubeVirt VMI plural", func() {
			Expect(constants.KubevirtVMIPlural).To(Equal("virtualmachineinstances"))
		})
	})

	Context("CDI API coordinates", func() {
		It("should have correct CDI API group", func() {
			Expect(constants.CDIAPIGroup).To(Equal("cdi.kubevirt.io"))
		})

		It("should have correct CDI API version", func() {
			Expect(constants.CDIAPIVersion).To(Equal("v1beta1"))
		})

		It("should have correct CDI DataVolume plural", func() {
			Expect(constants.CDIDVPlural).To(Equal("datavolumes"))
		})
	})

	Context("Defaults", func() {
		It("should have correct default container disk image", func() {
			Expect(constants.DefaultContainerDiskImage).To(Equal("quay.io/containerdisks/fedora:41"))
		})

		It("should have correct default namespace", func() {
			Expect(constants.DefaultNamespace).To(Equal("virtwork"))
		})

		It("should have correct default CPU cores", func() {
			Expect(constants.DefaultCPUCores).To(Equal(2))
		})

		It("should have correct default memory", func() {
			Expect(constants.DefaultMemory).To(Equal("2Gi"))
		})

		It("should have correct default disk size", func() {
			Expect(constants.DefaultDiskSize).To(Equal("10Gi"))
		})

		It("should have correct default SSH user", func() {
			Expect(constants.DefaultSSHUser).To(Equal("virtwork"))
		})
	})

	Context("Labels", func() {
		It("should have valid label key format for app name", func() {
			Expect(constants.LabelAppName).To(Equal("app.kubernetes.io/name"))
			Expect(constants.LabelAppName).To(ContainSubstring("/"))
		})

		It("should have valid label key format for managed-by", func() {
			Expect(constants.LabelManagedBy).To(Equal("app.kubernetes.io/managed-by"))
			Expect(constants.LabelManagedBy).To(ContainSubstring("/"))
		})

		It("should have valid label key format for component", func() {
			Expect(constants.LabelComponent).To(Equal("app.kubernetes.io/component"))
			Expect(constants.LabelComponent).To(ContainSubstring("/"))
		})

		It("should have correct managed-by value", func() {
			Expect(constants.ManagedByValue).To(Equal("virtwork"))
		})

		It("should use kubernetes.io label domain", func() {
			for _, label := range []string{
				constants.LabelAppName,
				constants.LabelManagedBy,
				constants.LabelComponent,
			} {
				Expect(strings.HasPrefix(label, "app.kubernetes.io/")).To(BeTrue(),
					"label %q should use app.kubernetes.io/ prefix", label)
			}
		})
	})

	Context("KubeVirt disk and volume names", func() {
		It("should have correct container disk name", func() {
			Expect(constants.DiskNameContainerDisk).To(Equal("containerdisk"))
		})

		It("should have correct cloud-init disk name", func() {
			Expect(constants.DiskNameCloudInit).To(Equal("cloudinitdisk"))
		})

		It("should have correct data disk name", func() {
			Expect(constants.DiskNameData).To(Equal("datadisk"))
		})

		It("should have correct virtio bus type", func() {
			Expect(constants.DiskBusVirtio).To(Equal("virtio"))
		})

		It("should have correct default network name", func() {
			Expect(constants.NetworkNameDefault).To(Equal("default"))
		})
	})

	Context("Secret keys", func() {
		It("should have correct userdata secret key", func() {
			Expect(constants.SecretKeyUserdata).To(Equal("userdata"))
		})
	})

	Context("SSH configuration", func() {
		It("should have correct sudo rule", func() {
			Expect(constants.SSHSudoRule).To(Equal("ALL=(ALL) NOPASSWD:ALL"))
		})

		It("should have correct default shell", func() {
			Expect(constants.SSHDefaultShell).To(Equal("/bin/bash"))
		})
	})

	Context("Polling", func() {
		It("should have correct default ready timeout", func() {
			Expect(constants.DefaultReadyTimeout).To(Equal(600 * time.Second))
		})

		It("should have correct default poll interval", func() {
			Expect(constants.DefaultPollInterval).To(Equal(15 * time.Second))
		})

		It("should have timeout greater than poll interval", func() {
			Expect(constants.DefaultReadyTimeout).To(BeNumerically(">", constants.DefaultPollInterval))
		})
	})
})
