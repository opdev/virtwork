// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/workloads"
)

var _ = Describe("ChaosProcessWorkload", func() {
	var w *workloads.ChaosProcessWorkload

	BeforeEach(func() {
		w = workloads.NewChaosProcessWorkload(config.WorkloadConfig{
			Enabled:  config.BoolPtr(true),
			VMCount:  1,
			CPUCores: 2,
			Memory:   "2Gi",
		}, "virtwork", "", nil)
	})

	It("should return 'chaos-process' for Name", func() {
		Expect(w.Name()).To(Equal("chaos-process"))
	})

	It("should include procps-ng in packages", func() {
		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		pkgs, ok := parsed["packages"].([]interface{})
		Expect(ok).To(BeTrue())
		Expect(pkgs).To(ContainElement("procps-ng"))
	})

	It("should include chaos script in cloud-init", func() {
		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		Expect(parsed).To(HaveKey("write_files"))
		files := parsed["write_files"].([]interface{})
		Expect(files).To(HaveLen(2))

		var scriptFile map[string]interface{}
		for _, f := range files {
			file := f.(map[string]interface{})
			if file["path"] == "/usr/local/bin/chaos-process.sh" {
				scriptFile = file
				break
			}
		}
		Expect(scriptFile).NotTo(BeNil())

		content := scriptFile["content"].(string)
		Expect(content).To(ContainSubstring("#!/bin/bash"))
		Expect(content).To(ContainSubstring("CHAOS_SIGNAL"))
		Expect(content).To(ContainSubstring("CHAOS_INTERVAL"))
		Expect(content).To(ContainSubstring("kill_random_process"))
		Expect(scriptFile["permissions"]).To(Equal("0755"))
	})

	It("should include systemd service in cloud-init", func() {
		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		Expect(parsed).To(HaveKey("write_files"))
		files := parsed["write_files"].([]interface{})
		Expect(files).To(HaveLen(2))

		var serviceFile map[string]interface{}
		for _, f := range files {
			file := f.(map[string]interface{})
			if file["path"] == "/etc/systemd/system/virtwork-chaos-process.service" {
				serviceFile = file
				break
			}
		}
		Expect(serviceFile).NotTo(BeNil())

		content := serviceFile["content"].(string)
		Expect(content).To(ContainSubstring("Virtwork chaos-process workload"))
		Expect(content).To(ContainSubstring("ExecStart=/usr/local/bin/chaos-process.sh"))
		Expect(content).To(ContainSubstring("Restart=always"))
		Expect(content).To(ContainSubstring("CHAOS_SIGNAL"))
		Expect(content).To(ContainSubstring("CHAOS_INTERVAL"))
		Expect(serviceFile["permissions"]).To(Equal("0644"))
	})

	It("should enable and start the systemd service", func() {
		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		Expect(parsed).To(HaveKey("runcmd"))
		runcmd := parsed["runcmd"].([]interface{})

		// Check for systemctl commands
		hasReload := false
		hasEnable := false
		for _, cmd := range runcmd {
			cmdSlice := cmd.([]interface{})
			if len(cmdSlice) >= 2 && cmdSlice[0] == "systemctl" && cmdSlice[1] == "daemon-reload" {
				hasReload = true
			}
			if len(cmdSlice) >= 4 && cmdSlice[0] == "systemctl" &&
				cmdSlice[1] == "enable" && cmdSlice[2] == "--now" &&
				cmdSlice[3] == "virtwork-chaos-process.service" {
				hasEnable = true
			}
		}
		Expect(hasReload).To(BeTrue())
		Expect(hasEnable).To(BeTrue())
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

	It("should have no service", func() {
		Expect(w.RequiresService()).To(BeFalse())
		Expect(w.ServiceSpec()).To(BeNil())
	})

	It("should reflect config in VMResources", func() {
		res := w.VMResources()
		Expect(res.CPUCores).To(Equal(2))
		Expect(res.Memory).To(Equal("2Gi"))
	})

	It("should default to 1 VM", func() {
		Expect(w.VMCount()).To(Equal(1))
	})

	Context("param wiring", func() {
		It("should use default param values when Params is nil", func() {
			result, err := w.CloudInitUserdata()
			Expect(err).NotTo(HaveOccurred())

			parsed := parseYAML(result)
			files := parsed["write_files"].([]interface{})

			var unitContent string
			for _, f := range files {
				fm := f.(map[string]interface{})
				if fm["path"] == "/etc/systemd/system/virtwork-chaos-process.service" {
					unitContent = fm["content"].(string)
					break
				}
			}

			Expect(unitContent).To(ContainSubstring("CHAOS_SIGNAL=SIGTERM"))
			Expect(unitContent).To(ContainSubstring("CHAOS_INTERVAL=30"))
			Expect(unitContent).To(ContainSubstring("CHAOS_MIN_PID=1000"))
		})

		It("should wire custom params from WorkloadConfig.Params", func() {
			custom := workloads.NewChaosProcessWorkload(config.WorkloadConfig{
				Enabled:  config.BoolPtr(true),
				VMCount:  1,
				CPUCores: 2,
				Memory:   "2Gi",
				Params: map[string]string{
					"signal":   "SIGKILL",
					"interval": "10",
					"min-pid":  "500",
				},
			}, "virtwork", "", nil)

			result, err := custom.CloudInitUserdata()
			Expect(err).NotTo(HaveOccurred())

			parsed := parseYAML(result)
			files := parsed["write_files"].([]interface{})

			var unitContent string
			for _, f := range files {
				fm := f.(map[string]interface{})
				if fm["path"] == "/etc/systemd/system/virtwork-chaos-process.service" {
					unitContent = fm["content"].(string)
					break
				}
			}

			Expect(unitContent).To(ContainSubstring("CHAOS_SIGNAL=SIGKILL"))
			Expect(unitContent).To(ContainSubstring("CHAOS_INTERVAL=10"))
			Expect(unitContent).To(ContainSubstring("CHAOS_MIN_PID=500"))
		})

		It("should use defaults for missing individual params", func() {
			partial := workloads.NewChaosProcessWorkload(config.WorkloadConfig{
				Enabled:  config.BoolPtr(true),
				VMCount:  1,
				CPUCores: 2,
				Memory:   "2Gi",
				Params: map[string]string{
					"signal": "SIGUSR1",
				},
			}, "virtwork", "", nil)

			result, err := partial.CloudInitUserdata()
			Expect(err).NotTo(HaveOccurred())

			parsed := parseYAML(result)
			files := parsed["write_files"].([]interface{})

			var unitContent string
			for _, f := range files {
				fm := f.(map[string]interface{})
				if fm["path"] == "/etc/systemd/system/virtwork-chaos-process.service" {
					unitContent = fm["content"].(string)
					break
				}
			}

			Expect(unitContent).To(ContainSubstring("CHAOS_SIGNAL=SIGUSR1"))
			Expect(unitContent).To(ContainSubstring("CHAOS_INTERVAL=30"))
			Expect(unitContent).To(ContainSubstring("CHAOS_MIN_PID=1000"))
		})
	})
})
