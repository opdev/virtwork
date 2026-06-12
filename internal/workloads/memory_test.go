// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/workloads"
)

var _ = Describe("MemoryWorkload", func() {
	var w *workloads.MemoryWorkload

	BeforeEach(func() {
		w = workloads.NewMemoryWorkload(config.WorkloadConfig{
			Enabled:  new(true),
			VMCount:  1,
			CPUCores: 2,
			Memory:   "4Gi",
		}, "virtwork", "", nil)
	})

	It("should return 'memory' for Name", func() {
		Expect(w.Name()).To(Equal("memory"))
	})

	It("should include stress-ng in packages", func() {
		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		pkgs, ok := parsed["packages"].([]interface{})
		Expect(ok).To(BeTrue())
		Expect(pkgs).To(ContainElement("stress-ng"))
	})

	It("should include systemd service with --vm flag", func() {
		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		files := parsed["write_files"].([]interface{})
		file := files[0].(map[string]interface{})
		content := file["content"].(string)
		Expect(content).To(ContainSubstring("--vm 1"))
	})

	It("should include --vm-bytes 80% in stress-ng args", func() {
		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		files := parsed["write_files"].([]interface{})
		file := files[0].(map[string]interface{})
		content := file["content"].(string)
		Expect(content).To(ContainSubstring("--vm-bytes 80%"))
	})

	It("should include --vm-method all in stress-ng args", func() {
		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		files := parsed["write_files"].([]interface{})
		file := files[0].(map[string]interface{})
		content := file["content"].(string)
		Expect(content).To(ContainSubstring("--vm-method all"))
	})

	It("should produce valid YAML", func() {
		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(HavePrefix("#cloud-config\n"))

		parsed := parseYAML(result)
		Expect(parsed).NotTo(BeNil())
	})

	It("should have no extra disks", func() {
		Expect(w.ExtraDisks()).To(BeNil())
	})

	It("should have no extra volumes", func() {
		Expect(w.ExtraVolumes()).To(BeNil())
	})

	It("should have no data volume templates", func() {
		Expect(w.DataVolumeTemplates()).To(BeNil())
	})

	It("should not require service", func() {
		Expect(w.RequiresService()).To(BeFalse())
		Expect(w.ServiceSpec()).To(BeNil())
	})

	It("should reflect config in VMResources", func() {
		res := w.VMResources()
		Expect(res.CPUCores).To(Equal(2))
		Expect(res.Memory).To(Equal("4Gi"))
	})

	Context("param wiring", func() {
		It("should use default param values when Params is nil", func() {
			result, err := w.CloudInitUserdata()
			Expect(err).NotTo(HaveOccurred())

			parsed := parseYAML(result)
			files := parsed["write_files"].([]interface{})
			file := files[0].(map[string]interface{})
			content := file["content"].(string)

			Expect(content).To(ContainSubstring("--vm 1"))
			Expect(content).To(ContainSubstring("--vm-bytes 80%"))
			Expect(content).To(ContainSubstring("--vm-method all"))
		})

		It("should wire custom params from WorkloadConfig.Params", func() {
			custom := workloads.NewMemoryWorkload(config.WorkloadConfig{
				Enabled:  new(true),
				VMCount:  1,
				CPUCores: 2,
				Memory:   "4Gi",
				Params: map[string]string{
					"memory-percent": "60",
					"vm-stressors":   "2",
					"vm-method":      "flip",
				},
			}, "virtwork", "", nil)

			result, err := custom.CloudInitUserdata()
			Expect(err).NotTo(HaveOccurred())

			parsed := parseYAML(result)
			files := parsed["write_files"].([]interface{})
			file := files[0].(map[string]interface{})
			content := file["content"].(string)

			Expect(content).To(ContainSubstring("--vm 2"))
			Expect(content).To(ContainSubstring("--vm-bytes 60%"))
			Expect(content).To(ContainSubstring("--vm-method flip"))
		})

		It("should use defaults for missing individual params", func() {
			partial := workloads.NewMemoryWorkload(config.WorkloadConfig{
				Enabled:  new(true),
				VMCount:  1,
				CPUCores: 2,
				Memory:   "4Gi",
				Params: map[string]string{
					"memory-percent": "50",
				},
			}, "virtwork", "", nil)

			result, err := partial.CloudInitUserdata()
			Expect(err).NotTo(HaveOccurred())

			parsed := parseYAML(result)
			files := parsed["write_files"].([]interface{})
			file := files[0].(map[string]interface{})
			content := file["content"].(string)

			Expect(content).To(ContainSubstring("--vm 1"))
			Expect(content).To(ContainSubstring("--vm-bytes 50%"))
			Expect(content).To(ContainSubstring("--vm-method all"))
		})
	})
})
