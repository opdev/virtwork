// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/workloads"
)

var _ = Describe("DatabaseWorkload", func() {
	var w *workloads.DatabaseWorkload

	BeforeEach(func() {
		w = workloads.NewDatabaseWorkload(config.WorkloadConfig{
			Enabled:  true,
			VMCount:  1,
			CPUCores: 2,
			Memory:   "4Gi",
		}, "10Gi", "virtwork", "", nil)
	})

	It("should return 'database' for Name", func() {
		Expect(w.Name()).To(Equal("database"))
	})

	It("should include postgresql-server in packages", func() {
		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		pkgs, ok := parsed["packages"].([]interface{})
		Expect(ok).To(BeTrue())
		Expect(pkgs).To(ContainElement("postgresql-server"))
	})

	It("should include setup script in write_files", func() {
		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		files := parsed["write_files"].([]interface{})

		paths := make([]string, len(files))
		for i, f := range files {
			paths[i] = f.(map[string]interface{})["path"].(string)
		}
		Expect(paths).To(ContainElement("/usr/local/bin/virtwork-db-setup.sh"))
	})

	It("should include setup script with serial discovery and initdb", func() {
		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		files := parsed["write_files"].([]interface{})

		var setupContent string
		for _, f := range files {
			file := f.(map[string]interface{})
			if file["path"].(string) == "/usr/local/bin/virtwork-db-setup.sh" {
				setupContent = file["content"].(string)
				break
			}
		}
		Expect(setupContent).NotTo(BeEmpty())
		Expect(setupContent).To(ContainSubstring("virtio-virtwork-dbdisk"))
		Expect(setupContent).To(ContainSubstring("postgresql-setup --initdb"))
		Expect(setupContent).To(ContainSubstring("pgbench"))
		Expect(setupContent).To(ContainSubstring("scale"))
	})

	It("should include pgbench systemd service", func() {
		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		files := parsed["write_files"].([]interface{})

		paths := make([]string, len(files))
		for i, f := range files {
			paths[i] = f.(map[string]interface{})["path"].(string)
		}
		Expect(paths).To(ContainElement("/etc/systemd/system/virtwork-database.service"))
	})

	It("should include pgbench loop in systemd service", func() {
		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		files := parsed["write_files"].([]interface{})

		var serviceContent string
		for _, f := range files {
			file := f.(map[string]interface{})
			if file["path"].(string) == "/etc/systemd/system/virtwork-database.service" {
				serviceContent = file["content"].(string)
				break
			}
		}
		Expect(serviceContent).NotTo(BeEmpty())
		Expect(serviceContent).To(ContainSubstring("pgbench"))
		Expect(serviceContent).To(ContainSubstring("-c 10"))
		Expect(serviceContent).To(ContainSubstring("-j 2"))
		Expect(serviceContent).To(ContainSubstring("-T 300"))
	})

	It("should use ExecStartPre for setup script", func() {
		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		files := parsed["write_files"].([]interface{})

		var serviceContent string
		for _, f := range files {
			file := f.(map[string]interface{})
			if file["path"].(string) == "/etc/systemd/system/virtwork-database.service" {
				serviceContent = file["content"].(string)
				break
			}
		}
		Expect(serviceContent).To(ContainSubstring("ExecStartPre"))
		Expect(serviceContent).To(ContainSubstring("virtwork-db-setup.sh"))
	})

	It("should produce valid YAML", func() {
		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(HavePrefix("#cloud-config\n"))

		parsed := parseYAML(result)
		Expect(parsed).NotTo(BeNil())
	})

	It("should have data volume template", func() {
		dvts := w.DataVolumeTemplates()
		Expect(dvts).To(HaveLen(1))
		Expect(dvts[0].Name).To(Equal("virtwork-database-data"))
	})

	It("should have extra disk for data volume with serial", func() {
		disks := w.ExtraDisks()
		Expect(disks).To(HaveLen(1))
		Expect(disks[0].Name).To(Equal("datadisk"))
		Expect(disks[0].Serial).To(Equal("virtwork-dbdisk"))

		volumes := w.ExtraVolumes()
		Expect(volumes).To(HaveLen(1))
		Expect(volumes[0].Name).To(Equal("datadisk"))
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
})
