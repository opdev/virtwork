// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/workloads"
)

var _ = Describe("ChaosDiskWorkload", func() {
	var w *workloads.ChaosDiskWorkload

	BeforeEach(func() {
		w = workloads.NewChaosDiskWorkload(config.WorkloadConfig{
			Enabled:  true,
			VMCount:  1,
			CPUCores: 2,
			Memory:   "2Gi",
		}, "10Gi", "virtwork", "", nil)
	})

	It("should return 'chaos-disk' for Name", func() {
		Expect(w.Name()).To(Equal("chaos-disk"))
	})

	It("should include chaos-disk script in write_files", func() {
		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		Expect(parsed).To(HaveKey("write_files"))
		files := parsed["write_files"].([]interface{})

		paths := make([]string, len(files))
		for i, f := range files {
			paths[i] = f.(map[string]interface{})["path"].(string)
		}
		Expect(paths).To(ContainElement("/usr/local/bin/chaos-disk.sh"))
		Expect(paths).To(ContainElement("/etc/systemd/system/virtwork-chaos-disk.service"))
	})

	It("should include fallocate in the chaos-disk script", func() {
		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		files := parsed["write_files"].([]interface{})

		var scriptContent string
		for _, f := range files {
			fm := f.(map[string]interface{})
			if fm["path"] == "/usr/local/bin/chaos-disk.sh" {
				scriptContent = fm["content"].(string)
				break
			}
		}
		Expect(scriptContent).NotTo(BeEmpty())
		Expect(scriptContent).To(ContainSubstring("fallocate"))
		Expect(scriptContent).To(ContainSubstring("FILL_PERCENT"))
		Expect(scriptContent).To(ContainSubstring("MOUNT_POINT"))
	})

	It("should make the chaos-disk script executable", func() {
		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		files := parsed["write_files"].([]interface{})

		for _, f := range files {
			fm := f.(map[string]interface{})
			if fm["path"] == "/usr/local/bin/chaos-disk.sh" {
				Expect(fm["permissions"]).To(Equal("0755"))
				return
			}
		}
		Fail("chaos-disk.sh not found in write_files")
	})

	It("should include systemd unit referencing the script", func() {
		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		files := parsed["write_files"].([]interface{})

		for _, f := range files {
			fm := f.(map[string]interface{})
			if fm["path"] == "/etc/systemd/system/virtwork-chaos-disk.service" {
				content := fm["content"].(string)
				Expect(content).To(ContainSubstring("/usr/local/bin/chaos-disk.sh"))
				Expect(content).To(ContainSubstring("Restart=always"))
				return
			}
		}
		Fail("systemd unit not found in write_files")
	})

	It("should produce valid YAML with cloud-config header", func() {
		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(HavePrefix("#cloud-config\n"))

		parsed := parseYAML(result)
		Expect(parsed).NotTo(BeNil())
	})

	It("should have data volume template", func() {
		dvts := w.DataVolumeTemplates()
		Expect(dvts).To(HaveLen(1))
		Expect(dvts[0].Name).To(Equal("virtwork-chaos-disk-data"))
	})

	It("should have extra disk for data volume with serial", func() {
		disks := w.ExtraDisks()
		Expect(disks).To(HaveLen(1))
		Expect(disks[0].Name).To(Equal("datadisk"))
		Expect(disks[0].Serial).To(Equal("virtwork-chdisk"))

		volumes := w.ExtraVolumes()
		Expect(volumes).To(HaveLen(1))
		Expect(volumes[0].Name).To(Equal("datadisk"))
	})

	It("should not require a service", func() {
		Expect(w.RequiresService()).To(BeFalse())
		Expect(w.ServiceSpec()).To(BeNil())
	})

	It("should reflect config in VMResources", func() {
		res := w.VMResources()
		Expect(res.CPUCores).To(Equal(2))
		Expect(res.Memory).To(Equal("2Gi"))
	})

	It("should not require packages (tools pre-installed via golden image)", func() {
		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		_, hasPackages := parsed["packages"]
		Expect(hasPackages).To(BeFalse())
	})

	It("should include disk-setup script in write_files", func() {
		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		files := parsed["write_files"].([]interface{})

		var setupContent string
		for _, f := range files {
			fm := f.(map[string]interface{})
			if fm["path"] == "/usr/local/bin/virtwork-disk-setup.sh" {
				setupContent = fm["content"].(string)
				break
			}
		}
		Expect(setupContent).NotTo(BeEmpty())
		Expect(setupContent).To(ContainSubstring("virtio-virtwork-chdisk"))
		Expect(setupContent).To(ContainSubstring("/mnt/data"))
		Expect(setupContent).To(ContainSubstring("mkfs.xfs"))
	})

	It("should run disk-setup script and enable service via runcmd", func() {
		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		Expect(parsed).To(HaveKey("runcmd"))

		cmds := parsed["runcmd"].([]interface{})
		cmdStrings := make([]string, 0)
		for _, c := range cmds {
			parts := c.([]interface{})
			s := ""
			for _, p := range parts {
				if s != "" {
					s += " "
				}
				s += p.(string)
			}
			cmdStrings = append(cmdStrings, s)
		}

		Expect(cmdStrings).To(ContainElement(ContainSubstring("virtwork-disk-setup.sh")))
		Expect(cmdStrings).To(ContainElement(ContainSubstring("daemon-reload")))
		Expect(cmdStrings).To(ContainElement(ContainSubstring("virtwork-chaos-disk.service")))
	})
})
