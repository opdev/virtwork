// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/workloads"
)

var _ = Describe("Registry", func() {
	var reg workloads.Registry

	BeforeEach(func() {
		reg = workloads.DefaultRegistry()
	})

	It("should have 9 entries registered", func() {
		Expect(reg.List()).To(HaveLen(9))
	})

	It("should return chaos-disk workload by name", func() {
		w, err := reg.Get("chaos-disk", config.WorkloadConfig{
			Enabled:  config.BoolPtr(true),
			VMCount:  1,
			CPUCores: 2,
			Memory:   "2Gi",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(w.Name()).To(Equal("chaos-disk"))
	})

	It("should return CPU workload by name", func() {
		w, err := reg.Get("cpu", config.WorkloadConfig{
			Enabled:  config.BoolPtr(true),
			VMCount:  1,
			CPUCores: 2,
			Memory:   "2Gi",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(w.Name()).To(Equal("cpu"))
	})

	It("should return memory workload by name", func() {
		w, err := reg.Get("memory", config.WorkloadConfig{
			Enabled:  config.BoolPtr(true),
			VMCount:  1,
			CPUCores: 2,
			Memory:   "4Gi",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(w.Name()).To(Equal("memory"))
	})

	It("should return database workload by name", func() {
		w, err := reg.Get("database", config.WorkloadConfig{
			Enabled:  config.BoolPtr(true),
			VMCount:  1,
			CPUCores: 2,
			Memory:   "4Gi",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(w.Name()).To(Equal("database"))
	})

	It("should return network workload by name", func() {
		w, err := reg.Get("network", config.WorkloadConfig{
			Enabled:  config.BoolPtr(true),
			VMCount:  2,
			CPUCores: 2,
			Memory:   "2Gi",
		}, workloads.WithNamespace("virtwork"))
		Expect(err).NotTo(HaveOccurred())
		Expect(w.Name()).To(Equal("network"))
	})

	It("should return disk workload by name", func() {
		w, err := reg.Get("disk", config.WorkloadConfig{
			Enabled:  config.BoolPtr(true),
			VMCount:  1,
			CPUCores: 2,
			Memory:   "2Gi",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(w.Name()).To(Equal("disk"))
	})

	It("should return chaos-process workload by name", func() {
		w, err := reg.Get("chaos-process", config.WorkloadConfig{
			Enabled:  config.BoolPtr(true),
			VMCount:  1,
			CPUCores: 2,
			Memory:   "2Gi",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(w.Name()).To(Equal("chaos-process"))
	})

	It("should return chaos-network workload by name", func() {
		w, err := reg.Get("chaos-network", config.WorkloadConfig{
			Enabled:  config.BoolPtr(true),
			VMCount:  1,
			CPUCores: 1,
			Memory:   "1Gi",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(w.Name()).To(Equal("chaos-network"))
	})

	It("should return error for unknown name with available names", func() {
		_, err := reg.Get("unknown", config.WorkloadConfig{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unknown"))
		Expect(err.Error()).To(ContainSubstring("chaos-disk"))
		Expect(err.Error()).To(ContainSubstring("chaos-network"))
		Expect(err.Error()).To(ContainSubstring("chaos-process"))
		Expect(err.Error()).To(ContainSubstring("cpu"))
		Expect(err.Error()).To(ContainSubstring("database"))
		Expect(err.Error()).To(ContainSubstring("disk"))
		Expect(err.Error()).To(ContainSubstring("memory"))
		Expect(err.Error()).To(ContainSubstring("network"))
		Expect(err.Error()).To(ContainSubstring("tps"))
	})

	It("should list all names sorted alphabetically", func() {
		names := reg.List()
		Expect(names).To(Equal([]string{
			"chaos-disk",
			"chaos-network",
			"chaos-process",
			"cpu",
			"database",
			"disk",
			"memory",
			"network",
			"tps",
		}))
	})

	It("should return tps workload by name", func() {
		w, err := reg.Get("tps", config.WorkloadConfig{
			Enabled:  config.BoolPtr(true),
			VMCount:  2,
			CPUCores: 2,
			Memory:   "2Gi",
		}, workloads.WithNamespace("virtwork"))
		Expect(err).NotTo(HaveOccurred())
		Expect(w.Name()).To(Equal("tps"))
	})

	It("should create workloads with provided config", func() {
		cfg := config.WorkloadConfig{
			Enabled:  config.BoolPtr(true),
			VMCount:  1,
			CPUCores: 4,
			Memory:   "8Gi",
		}
		w, err := reg.Get("cpu", cfg)
		Expect(err).NotTo(HaveOccurred())

		res := w.VMResources()
		Expect(res.CPUCores).To(Equal(4))
		Expect(res.Memory).To(Equal("8Gi"))
	})

	It("should pass namespace option to network workload", func() {
		w, err := reg.Get("network", config.WorkloadConfig{
			Enabled:  config.BoolPtr(true),
			VMCount:  2,
			CPUCores: 2,
			Memory:   "2Gi",
		}, workloads.WithNamespace("custom-ns"))
		Expect(err).NotTo(HaveOccurred())

		// Verify the namespace was passed by checking the service spec
		svc := w.ServiceSpec()
		Expect(svc).NotTo(BeNil())
		Expect(svc.Namespace).To(Equal("custom-ns"))
	})

	It("should pass SSH credentials via options", func() {
		w, err := reg.Get("cpu", config.WorkloadConfig{
			Enabled:  config.BoolPtr(true),
			VMCount:  1,
			CPUCores: 2,
			Memory:   "2Gi",
		}, workloads.WithSSHCredentials("testuser", "testpass", []string{"ssh-rsa AAAA..."}))
		Expect(err).NotTo(HaveOccurred())

		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		Expect(parsed).To(HaveKey("users"))
		users := parsed["users"].([]interface{})
		user := users[0].(map[string]interface{})
		Expect(user["name"]).To(Equal("testuser"))
	})

	It("should pass data disk size via options", func() {
		w, err := reg.Get("disk", config.WorkloadConfig{
			Enabled:  config.BoolPtr(true),
			VMCount:  1,
			CPUCores: 2,
			Memory:   "2Gi",
		}, workloads.WithDataDiskSize("20Gi"))
		Expect(err).NotTo(HaveOccurred())

		dvts := w.DataVolumeTemplates()
		Expect(dvts).NotTo(BeEmpty())
	})
})

var _ = Describe("AllWorkloadNames", func() {
	It("should derive names from DefaultRegistry", func() {
		// Verify that AllWorkloadNames() always matches DefaultRegistry().List()
		Expect(workloads.AllWorkloadNames()).To(Equal(workloads.DefaultRegistry().List()))
	})
})
