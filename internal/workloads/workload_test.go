// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/workloads"
)

var _ = Describe("BaseWorkload", func() {
	var base workloads.BaseWorkload

	BeforeEach(func() {
		base = workloads.BaseWorkload{
			Config: config.WorkloadConfig{
				Enabled:  config.BoolPtr(true),
				VMCount:  1,
				CPUCores: 4,
				Memory:   "4Gi",
			},
		}
	})

	It("should return nil for ExtraVolumes", func() {
		Expect(base.ExtraVolumes()).To(BeNil())
	})

	It("should return nil for ExtraDisks", func() {
		Expect(base.ExtraDisks()).To(BeNil())
	})

	It("should return nil for DataVolumeTemplates", func() {
		Expect(base.DataVolumeTemplates()).To(BeNil())
	})

	It("should return false for RequiresService", func() {
		Expect(base.RequiresService()).To(BeFalse())
	})

	It("should return nil for ServiceSpec", func() {
		Expect(base.ServiceSpec()).To(BeNil())
	})

	It("should return 1 for VMCount", func() {
		Expect(base.VMCount()).To(Equal(1))
	})

	It("should return correct VMResources from config", func() {
		res := base.VMResources()
		Expect(res.CPUCores).To(Equal(4))
		Expect(res.Memory).To(Equal("4Gi"))
	})

	Context("BuildCloudConfig helper", func() {
		It("should inject SSH credentials into cloud config", func() {
			base.SSHUser = "testuser"
			base.SSHPassword = "testpass"
			base.SSHAuthorizedKeys = []string{"ssh-rsa AAAA"}

			result, err := base.BuildCloudConfig(workloads.CloudConfigOpts{
				Packages: []string{"vim"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HavePrefix("#cloud-config\n"))

			parsed := parseYAML(result)
			Expect(parsed).To(HaveKey("users"))
			Expect(parsed).To(HaveKey("packages"))
			Expect(parsed).To(HaveKey("ssh_pwauth"))
		})

		It("should not inject SSH user when SSHUser is empty", func() {
			result, err := base.BuildCloudConfig(workloads.CloudConfigOpts{
				Packages: []string{"curl"},
			})
			Expect(err).NotTo(HaveOccurred())

			parsed := parseYAML(result)
			Expect(parsed).NotTo(HaveKey("users"))
			Expect(parsed).To(HaveKey("packages"))
		})
	})
})
