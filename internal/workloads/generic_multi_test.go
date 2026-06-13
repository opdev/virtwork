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

var _ = Describe("GenericMultiWorkload", func() {
	var (
		catalogDir string
		w          *workloads.GenericMultiWorkload
	)

	writeFile := func(dir, name, content string) {
		err := os.MkdirAll(dir, 0o750)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		err = os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
	}

	BeforeEach(func() {
		var err error
		catalogDir, err = os.MkdirTemp("", "virtwork-generic-multi-*")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		Expect(os.RemoveAll(catalogDir)).To(Succeed())
	})

	Context("two-role server/client entry", func() {
		BeforeEach(func() {
			entryDir := filepath.Join(catalogDir, "my-bench")
			writeFile(entryDir, "workload.yaml", `description: "Server/client benchmark"
packages:
  - iperf3
params:
  - key: duration
    type: int
    default: "60"
    desc: "Test duration in seconds"
roles:
  - name: server
    vm-count: 1
  - name: client
    vm-count: 2
`)
			writeFile(entryDir, "server.service", `[Unit]
Description=Benchmark server

[Service]
ExecStart=/usr/bin/iperf3 -s -t {{duration}}
Restart=always

[Install]
WantedBy=multi-user.target
`)
			writeFile(entryDir, "client.service", `[Unit]
Description=Benchmark client

[Service]
ExecStart=/usr/bin/iperf3 -c server -t {{duration}}
Restart=always

[Install]
WantedBy=multi-user.target
`)
			entry, err := workloads.LoadCatalogEntry(catalogDir, "my-bench")
			Expect(err).NotTo(HaveOccurred())
			w = workloads.NewGenericMultiWorkload(
				config.WorkloadConfig{CPUCores: 4, Memory: "4Gi"},
				entry, "test-ns", "", "", nil,
			)
		})

		It("should return the entry name for Name", func() {
			Expect(w.Name()).To(Equal("my-bench"))
		})

		It("should return role distribution matching manifest", func() {
			dist := w.RoleDistribution()
			Expect(dist).To(HaveLen(2))
			Expect(dist[0].Role).To(Equal("server"))
			Expect(dist[0].VMCount).To(Equal(1))
			Expect(dist[1].Role).To(Equal("client"))
			Expect(dist[1].VMCount).To(Equal(2))
		})

		It("should return total VMCount across all roles", func() {
			Expect(w.VMCount()).To(Equal(3))
		})

		It("should return first role's userdata for CloudInitUserdata", func() {
			result, err := w.CloudInitUserdata()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HavePrefix("#cloud-config\n"))
			Expect(result).To(ContainSubstring("iperf3 -s"))
		})

		It("should produce server userdata with UserdataForRole", func() {
			result, err := w.UserdataForRole("server", "test-ns")
			Expect(err).NotTo(HaveOccurred())
			parsed := parseYAML(result)

			files := parsed["write_files"].([]interface{})
			Expect(files).To(HaveLen(1))
			fm := files[0].(map[string]interface{})
			Expect(fm["path"]).To(Equal("/etc/systemd/system/server.service"))
			Expect(fm["content"]).To(ContainSubstring("iperf3 -s"))
		})

		It("should produce client userdata with UserdataForRole", func() {
			result, err := w.UserdataForRole("client", "test-ns")
			Expect(err).NotTo(HaveOccurred())
			parsed := parseYAML(result)

			files := parsed["write_files"].([]interface{})
			Expect(files).To(HaveLen(1))
			fm := files[0].(map[string]interface{})
			Expect(fm["path"]).To(Equal("/etc/systemd/system/client.service"))
			Expect(fm["content"]).To(ContainSubstring("iperf3 -c"))
		})

		It("should substitute default param values in role userdata", func() {
			result, err := w.UserdataForRole("server", "test-ns")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("-t 60"))
			Expect(result).NotTo(ContainSubstring("{{duration}}"))
		})

		It("should return ErrUnknownCatalogRole for unknown role", func() {
			_, err := w.UserdataForRole("unknown", "test-ns")
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring(workloads.ErrUnknownCatalogRole.Error())))
		})

		It("should include packages in role userdata", func() {
			result, err := w.UserdataForRole("server", "test-ns")
			Expect(err).NotTo(HaveOccurred())
			parsed := parseYAML(result)
			pkgs, ok := parsed["packages"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(pkgs).To(ContainElement("iperf3"))
		})

		It("should include daemon-reload and enable in runcmd", func() {
			result, err := w.UserdataForRole("client", "test-ns")
			Expect(err).NotTo(HaveOccurred())
			parsed := parseYAML(result)
			runcmd := parsed["runcmd"].([]interface{})
			Expect(runcmd).To(ContainElement(
				ConsistOf("systemctl", "daemon-reload"),
			))
			Expect(runcmd).To(ContainElement(
				ConsistOf("systemctl", "enable", "--now", "client.service"),
			))
		})

		It("should not require a service", func() {
			Expect(w.RequiresService()).To(BeFalse())
		})

		It("should reflect config in VMResources", func() {
			res := w.VMResources()
			Expect(res.CPUCores).To(Equal(4))
			Expect(res.Memory).To(Equal("4Gi"))
		})
	})

	Context("user-supplied param values", func() {
		BeforeEach(func() {
			entryDir := filepath.Join(catalogDir, "param-multi")
			writeFile(entryDir, "workload.yaml", `params:
  - key: duration
    type: int
    default: "60"
    desc: "Duration"
roles:
  - name: server
    vm-count: 1
  - name: client
    vm-count: 1
`)
			writeFile(entryDir, "server.service", "[Service]\nExecStart=/bin/s -t {{duration}}\n")
			writeFile(entryDir, "client.service", "[Service]\nExecStart=/bin/c -t {{duration}}\n")
		})

		It("should substitute user params in role userdata", func() {
			entry, err := workloads.LoadCatalogEntry(catalogDir, "param-multi")
			Expect(err).NotTo(HaveOccurred())
			w = workloads.NewGenericMultiWorkload(
				config.WorkloadConfig{
					CPUCores: 2,
					Memory:   "2Gi",
					Params:   map[string]string{"duration": "120"},
				},
				entry, "ns", "", "", nil,
			)
			result, err := w.UserdataForRole("server", "ns")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("-t 120"))
		})
	})

	Context("role with vm-count 0 uses Config.VMCount", func() {
		BeforeEach(func() {
			entryDir := filepath.Join(catalogDir, "zero-count")
			writeFile(entryDir, "workload.yaml", `roles:
  - name: worker
    vm-count: 0
  - name: controller
    vm-count: 1
`)
			writeFile(entryDir, "worker.service", "[Service]\nExecStart=/bin/w\n")
			writeFile(entryDir, "controller.service", "[Service]\nExecStart=/bin/c\n")
		})

		It("should use max(1, Config.VMCount) when manifest vm-count is 0", func() {
			entry, err := workloads.LoadCatalogEntry(catalogDir, "zero-count")
			Expect(err).NotTo(HaveOccurred())
			w = workloads.NewGenericMultiWorkload(
				config.WorkloadConfig{CPUCores: 2, Memory: "2Gi", VMCount: 3},
				entry, "ns", "", "", nil,
			)
			dist := w.RoleDistribution()
			Expect(dist).To(HaveLen(2))

			var workerCount, controllerCount int
			for _, rs := range dist {
				if rs.Role == "worker" {
					workerCount = rs.VMCount
				}
				if rs.Role == "controller" {
					controllerCount = rs.VMCount
				}
			}
			Expect(workerCount).To(Equal(3))
			Expect(controllerCount).To(Equal(1))
		})

		It("should default to 1 when both manifest and config are 0", func() {
			entry, err := workloads.LoadCatalogEntry(catalogDir, "zero-count")
			Expect(err).NotTo(HaveOccurred())
			w = workloads.NewGenericMultiWorkload(
				config.WorkloadConfig{CPUCores: 2, Memory: "2Gi", VMCount: 0},
				entry, "ns", "", "", nil,
			)
			dist := w.RoleDistribution()
			var workerCount int
			for _, rs := range dist {
				if rs.Role == "worker" {
					workerCount = rs.VMCount
				}
			}
			Expect(workerCount).To(Equal(1))
		})
	})

	Context("interface compliance", func() {
		It("should implement MultiVMWorkload interface", func() {
			entryDir := filepath.Join(catalogDir, "iface-check")
			writeFile(entryDir, "workload.yaml", `roles:
  - name: a
    vm-count: 1
  - name: b
    vm-count: 1
`)
			writeFile(entryDir, "a.service", "[Service]\nExecStart=/bin/a\n")
			writeFile(entryDir, "b.service", "[Service]\nExecStart=/bin/b\n")
			entry, err := workloads.LoadCatalogEntry(catalogDir, "iface-check")
			Expect(err).NotTo(HaveOccurred())
			multi := workloads.NewGenericMultiWorkload(
				config.WorkloadConfig{CPUCores: 2, Memory: "2Gi"},
				entry, "ns", "", "", nil,
			)
			var _ workloads.MultiVMWorkload = multi
		})
	})
})
