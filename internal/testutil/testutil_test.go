// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package testutil_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/opdev/virtwork/internal/constants"
	"github.com/opdev/virtwork/internal/testutil"
)

func TestTestutil(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Testutil Suite")
}

var _ = Describe("UniqueNamespace", func() {
	Context("when generating unique namespace names", func() {
		It("should include the prefix in the namespace name", func() {
			result := testutil.UniqueNamespace("myprefix")
			Expect(result).To(ContainSubstring("myprefix"))
		})

		It("should start with 'virtwork-test-' prefix", func() {
			result := testutil.UniqueNamespace("foo")
			Expect(result).To(HavePrefix("virtwork-test-"))
		})

		It("should generate unique names on subsequent calls", func() {
			name1 := testutil.UniqueNamespace("test")
			name2 := testutil.UniqueNamespace("test")
			Expect(name1).NotTo(Equal(name2))
		})

		It("should generate names with expected format", func() {
			result := testutil.UniqueNamespace("integration")
			Expect(result).To(MatchRegexp(`^virtwork-test-integration-[0-9a-f]{8}$`))
		})
	})
})

var _ = Describe("ManagedLabels", func() {
	Context("when retrieving managed labels", func() {
		It("should return a map with the managed-by label", func() {
			labels := testutil.ManagedLabels()
			Expect(labels).To(HaveKey(constants.LabelManagedBy))
		})

		It("should have the correct managed-by value", func() {
			labels := testutil.ManagedLabels()
			Expect(labels[constants.LabelManagedBy]).To(Equal(constants.ManagedByValue))
		})

		It("should return a non-empty map", func() {
			labels := testutil.ManagedLabels()
			Expect(labels).NotTo(BeEmpty())
		})
	})
})

var _ = Describe("DefaultVMOpts", func() {
	var (
		vmName      string
		vmNamespace string
	)

	BeforeEach(func() {
		vmName = "test-vm"
		vmNamespace = "test-namespace"
	})

	Context("when creating default VM options", func() {
		It("should set the name correctly", func() {
			opts := testutil.DefaultVMOpts(vmName, vmNamespace)
			Expect(opts.Name).To(Equal(vmName))
		})

		It("should set the namespace correctly", func() {
			opts := testutil.DefaultVMOpts(vmName, vmNamespace)
			Expect(opts.Namespace).To(Equal(vmNamespace))
		})

		It("should use the default container disk image", func() {
			opts := testutil.DefaultVMOpts(vmName, vmNamespace)
			Expect(opts.ContainerDiskImage).To(Equal(constants.DefaultContainerDiskImage))
		})

		It("should set 1 CPU core", func() {
			opts := testutil.DefaultVMOpts(vmName, vmNamespace)
			Expect(opts.CPUCores).To(Equal(1))
		})

		It("should set 512Mi memory", func() {
			opts := testutil.DefaultVMOpts(vmName, vmNamespace)
			Expect(opts.Memory).To(Equal("512Mi"))
		})

		It("should include managed-by label", func() {
			opts := testutil.DefaultVMOpts(vmName, vmNamespace)
			Expect(opts.Labels).To(HaveKeyWithValue(constants.LabelManagedBy, constants.ManagedByValue))
		})

		It("should include app name label", func() {
			opts := testutil.DefaultVMOpts(vmName, vmNamespace)
			Expect(opts.Labels).To(HaveKeyWithValue(constants.LabelAppName, "virtwork"))
		})

		It("should include component label set to 'test'", func() {
			opts := testutil.DefaultVMOpts(vmName, vmNamespace)
			Expect(opts.Labels).To(HaveKeyWithValue(constants.LabelComponent, "test"))
		})

		It("should have a simple cloud-init userdata", func() {
			opts := testutil.DefaultVMOpts(vmName, vmNamespace)
			Expect(opts.CloudInitUserdata).To(Equal("#cloud-config\n"))
		})
	})
})
