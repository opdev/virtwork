// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/workloads"
)

var _ = Describe("GenericWorkload", func() {
	var (
		catalogDir string
		w          *workloads.GenericWorkload
	)

	writeFile := func(dir, name, content string) {
		err := os.MkdirAll(dir, 0o750)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		err = os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
	}

	BeforeEach(func() {
		var err error
		catalogDir, err = os.MkdirTemp("", "virtwork-generic-*")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		Expect(os.RemoveAll(catalogDir)).To(Succeed())
	})

	Context("basic single-service entry with no manifest", func() {
		BeforeEach(func() {
			entryDir := filepath.Join(catalogDir, "my-stress")
			writeFile(entryDir, "workload.service", `[Unit]
Description=My stress test

[Service]
ExecStart=/usr/bin/stress-ng --cpu 0
Restart=always

[Install]
WantedBy=multi-user.target
`)
			entry, err := workloads.LoadCatalogEntry(catalogDir, "my-stress")
			Expect(err).NotTo(HaveOccurred())
			w = workloads.NewGenericWorkload(
				config.WorkloadConfig{
					Enabled:  new(true),
					VMCount:  1,
					CPUCores: 2,
					Memory:   "2Gi",
				},
				entry, "test-ns", "virtwork", "", nil,
			)
		})

		It("should return the entry name for Name", func() {
			Expect(w.Name()).To(Equal("my-stress"))
		})

		It("should produce valid cloud-init userdata", func() {
			result, err := w.CloudInitUserdata()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HavePrefix("#cloud-config\n"))
		})

		It("should include service file as write_files", func() {
			result, err := w.CloudInitUserdata()
			Expect(err).NotTo(HaveOccurred())
			parsed := parseYAML(result)
			files := parsed["write_files"].([]interface{})
			found := false
			for _, f := range files {
				fm := f.(map[string]interface{})
				if fm["path"] == "/etc/systemd/system/workload.service" {
					found = true
					Expect(fm["content"]).To(ContainSubstring("stress-ng --cpu 0"))
				}
			}
			Expect(found).To(BeTrue(), "expected workload.service in write_files")
		})

		It("should enable the service via runcmd", func() {
			result, err := w.CloudInitUserdata()
			Expect(err).NotTo(HaveOccurred())
			parsed := parseYAML(result)
			runcmd := parsed["runcmd"].([]interface{})
			Expect(runcmd).To(ContainElement(
				ConsistOf("systemctl", "daemon-reload"),
			))
			Expect(runcmd).To(ContainElement(
				ConsistOf("systemctl", "enable", "--now", "workload.service"),
			))
		})

		It("should reflect config in VMResources", func() {
			res := w.VMResources()
			Expect(res.CPUCores).To(Equal(2))
			Expect(res.Memory).To(Equal("2Gi"))
		})

		It("should not require a service", func() {
			Expect(w.RequiresService()).To(BeFalse())
		})

		It("should return nil for ExtraVolumes", func() {
			Expect(w.ExtraVolumes()).To(BeNil())
		})

		It("should return nil for ExtraDisks", func() {
			Expect(w.ExtraDisks()).To(BeNil())
		})

		It("should return nil for DataVolumeTemplates", func() {
			dvts, err := w.DataVolumeTemplates()
			Expect(err).NotTo(HaveOccurred())
			Expect(dvts).To(BeNil())
		})

		It("should return configured VMCount", func() {
			Expect(w.VMCount()).To(Equal(1))
		})
	})

	Context("entry with manifest packages", func() {
		BeforeEach(func() {
			entryDir := filepath.Join(catalogDir, "with-pkgs")
			writeFile(entryDir, "workload.yaml", `description: "With packages"
packages:
  - stress-ng
  - htop
`)
			writeFile(entryDir, "workload.service", "[Service]\nExecStart=/usr/bin/stress-ng\n")
			entry, err := workloads.LoadCatalogEntry(catalogDir, "with-pkgs")
			Expect(err).NotTo(HaveOccurred())
			w = workloads.NewGenericWorkload(
				config.WorkloadConfig{CPUCores: 2, Memory: "2Gi"},
				entry, "test-ns", "", "", nil,
			)
		})

		It("should include packages in cloud-init", func() {
			result, err := w.CloudInitUserdata()
			Expect(err).NotTo(HaveOccurred())
			parsed := parseYAML(result)
			pkgs, ok := parsed["packages"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(pkgs).To(ContainElement("stress-ng"))
			Expect(pkgs).To(ContainElement("htop"))
		})
	})

	Context("param substitution", func() {
		BeforeEach(func() {
			entryDir := filepath.Join(catalogDir, "param-sub")
			writeFile(entryDir, "workload.yaml", `params:
  - key: cpu-load
    type: int
    default: "50"
    desc: "CPU load"
  - key: method
    type: string
    default: "all"
    desc: "Method"
`)
			writeFile(
				entryDir,
				"workload.service",
				"[Service]\nExecStart=/usr/bin/stress-ng --cpu-load {{cpu-load}} --method {{method}}\n",
			)
		})

		It("should substitute default param values", func() {
			entry, err := workloads.LoadCatalogEntry(catalogDir, "param-sub")
			Expect(err).NotTo(HaveOccurred())
			w = workloads.NewGenericWorkload(
				config.WorkloadConfig{CPUCores: 2, Memory: "2Gi"},
				entry, "test-ns", "", "", nil,
			)
			result, err := w.CloudInitUserdata()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("--cpu-load 50"))
			Expect(result).To(ContainSubstring("--method all"))
		})

		It("should substitute user-supplied param values", func() {
			entry, err := workloads.LoadCatalogEntry(catalogDir, "param-sub")
			Expect(err).NotTo(HaveOccurred())
			w = workloads.NewGenericWorkload(
				config.WorkloadConfig{
					CPUCores: 2,
					Memory:   "2Gi",
					Params: map[string]string{
						"cpu-load": "75",
						"method":   "matrixprod",
					},
				},
				entry, "test-ns", "", "", nil,
			)
			result, err := w.CloudInitUserdata()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("--cpu-load 75"))
			Expect(result).To(ContainSubstring("--method matrixprod"))
		})

		It("should use defaults for missing individual params", func() {
			entry, err := workloads.LoadCatalogEntry(catalogDir, "param-sub")
			Expect(err).NotTo(HaveOccurred())
			w = workloads.NewGenericWorkload(
				config.WorkloadConfig{
					CPUCores: 2,
					Memory:   "2Gi",
					Params:   map[string]string{"cpu-load": "90"},
				},
				entry, "test-ns", "", "", nil,
			)
			result, err := w.CloudInitUserdata()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("--cpu-load 90"))
			Expect(result).To(ContainSubstring("--method all"))
		})
	})

	Context("multiple service files", func() {
		BeforeEach(func() {
			entryDir := filepath.Join(catalogDir, "multi-svc")
			writeFile(entryDir, "app.service", "[Service]\nExecStart=/bin/app\n")
			writeFile(entryDir, "monitor.service", "[Service]\nExecStart=/bin/monitor\n")
			entry, err := workloads.LoadCatalogEntry(catalogDir, "multi-svc")
			Expect(err).NotTo(HaveOccurred())
			w = workloads.NewGenericWorkload(
				config.WorkloadConfig{CPUCores: 2, Memory: "2Gi"},
				entry, "test-ns", "", "", nil,
			)
		})

		It("should include all service files in write_files", func() {
			result, err := w.CloudInitUserdata()
			Expect(err).NotTo(HaveOccurred())
			parsed := parseYAML(result)
			files := parsed["write_files"].([]interface{})
			paths := make([]string, 0, len(files))
			for _, f := range files {
				fm := f.(map[string]interface{})
				paths = append(paths, fm["path"].(string))
			}
			Expect(paths).To(ContainElement("/etc/systemd/system/app.service"))
			Expect(paths).To(ContainElement("/etc/systemd/system/monitor.service"))
		})

		It("should enable all services via runcmd", func() {
			result, err := w.CloudInitUserdata()
			Expect(err).NotTo(HaveOccurred())
			parsed := parseYAML(result)
			runcmd := parsed["runcmd"].([]interface{})
			Expect(runcmd).To(ContainElement(
				ConsistOf("systemctl", "enable", "--now", "app.service"),
			))
			Expect(runcmd).To(ContainElement(
				ConsistOf("systemctl", "enable", "--now", "monitor.service"),
			))
		})
	})
})
