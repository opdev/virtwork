// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/workloads"
)

var _ = Describe("CPUWorkload", func() {
	var w *workloads.CPUWorkload

	BeforeEach(func() {
		w = workloads.NewCPUWorkload(config.WorkloadConfig{
			Enabled:  new(true),
			VMCount:  1,
			CPUCores: 2,
			Memory:   "2Gi",
		}, "virtwork", "", nil)
	})

	It("should return 'cpu' for Name", func() {
		Expect(w.Name()).To(Equal("cpu"))
	})

	It("should include stress-ng in packages", func() {
		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		pkgs, ok := parsed["packages"].([]interface{})
		Expect(ok).To(BeTrue())
		Expect(pkgs).To(ContainElement("stress-ng"))
	})

	It("should include systemd service in cloud-init", func() {
		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		Expect(parsed).To(HaveKey("write_files"))
		files := parsed["write_files"].([]interface{})
		Expect(files).To(HaveLen(1))

		file := files[0].(map[string]interface{})
		Expect(file["path"]).To(Equal("/etc/systemd/system/virtwork-cpu.service"))

		content := file["content"].(string)
		Expect(content).To(ContainSubstring("stress-ng"))
		Expect(content).To(ContainSubstring("--cpu 0"))
		Expect(content).To(ContainSubstring("--cpu-load 100"))
		Expect(content).To(ContainSubstring("--cpu-method all"))
		Expect(content).To(ContainSubstring("--timeout 0"))
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

	It("should have no service", func() {
		Expect(w.RequiresService()).To(BeFalse())
		Expect(w.ServiceSpec()).To(BeNil())
	})

	It("should reflect config in VMResources", func() {
		res := w.VMResources()
		Expect(res.CPUCores).To(Equal(2))
		Expect(res.Memory).To(Equal("2Gi"))
	})

	Context("param wiring", func() {
		It("should use default param values when Params is nil", func() {
			result, err := w.CloudInitUserdata()
			Expect(err).NotTo(HaveOccurred())

			parsed := parseYAML(result)
			files := parsed["write_files"].([]interface{})
			file := files[0].(map[string]interface{})
			content := file["content"].(string)

			Expect(content).To(ContainSubstring("--cpu-load 100"))
			Expect(content).To(ContainSubstring("--cpu-method all"))
		})

		It("should wire custom params from WorkloadConfig.Params", func() {
			custom := workloads.NewCPUWorkload(config.WorkloadConfig{
				Enabled:  new(true),
				VMCount:  1,
				CPUCores: 2,
				Memory:   "2Gi",
				Params: map[string]string{
					"cpu-load-percent": "50",
					"cpu-method":       "matrixprod",
				},
			}, "virtwork", "", nil)

			result, err := custom.CloudInitUserdata()
			Expect(err).NotTo(HaveOccurred())

			parsed := parseYAML(result)
			files := parsed["write_files"].([]interface{})
			file := files[0].(map[string]interface{})
			content := file["content"].(string)

			Expect(content).To(ContainSubstring("--cpu-load 50"))
			Expect(content).To(ContainSubstring("--cpu-method matrixprod"))
		})

		It("should use defaults for missing individual params", func() {
			partial := workloads.NewCPUWorkload(config.WorkloadConfig{
				Enabled:  new(true),
				VMCount:  1,
				CPUCores: 2,
				Memory:   "2Gi",
				Params: map[string]string{
					"cpu-load-percent": "75",
				},
			}, "virtwork", "", nil)

			result, err := partial.CloudInitUserdata()
			Expect(err).NotTo(HaveOccurred())

			parsed := parseYAML(result)
			files := parsed["write_files"].([]interface{})
			file := files[0].(map[string]interface{})
			content := file["content"].(string)

			Expect(content).To(ContainSubstring("--cpu-load 75"))
			Expect(content).To(ContainSubstring("--cpu-method all"))
		})
	})
})
