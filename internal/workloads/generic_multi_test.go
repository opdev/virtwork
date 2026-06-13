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

	Context("multi-role entry with storage declared", func() {
		BeforeEach(func() {
			entryDir := filepath.Join(catalogDir, "storage-multi")
			writeFile(entryDir, "workload.yaml", `roles:
  - name: server
    vm-count: 1
  - name: client
    vm-count: 2
storage:
  - name: data
    size: 10Gi
    serial: vw-data
    mount: /mnt/data
  - name: logs
    size: 5Gi
    serial: vw-logs
    mount: /var/log/app
`)
			writeFile(entryDir, "server.service", "[Service]\nExecStart=/bin/server\n")
			writeFile(entryDir, "client.service", "[Service]\nExecStart=/bin/client\n")
			entry, err := workloads.LoadCatalogEntry(catalogDir, "storage-multi")
			Expect(err).NotTo(HaveOccurred())
			w = workloads.NewGenericMultiWorkload(
				config.WorkloadConfig{CPUCores: 2, Memory: "2Gi"},
				entry, "test-ns", "", "", nil,
			)
		})

		It("should return DataVolumeTemplates for all storage entries", func() {
			dvts, err := w.DataVolumeTemplates()
			Expect(err).NotTo(HaveOccurred())
			Expect(dvts).To(HaveLen(2))
			Expect(dvts[0].Name).To(Equal("data"))
			Expect(dvts[1].Name).To(Equal("logs"))
		})

		It("should return ExtraDisks with correct serials", func() {
			disks := w.ExtraDisks()
			Expect(disks).To(HaveLen(2))
			Expect(disks[0].Name).To(Equal("data"))
			Expect(disks[0].Serial).To(Equal("vw-data"))
			Expect(string(disks[0].DiskDevice.Disk.Bus)).To(Equal("virtio"))
			Expect(disks[1].Name).To(Equal("logs"))
			Expect(disks[1].Serial).To(Equal("vw-logs"))
		})

		It("should return ExtraVolumes referencing DataVolumes", func() {
			volumes := w.ExtraVolumes()
			Expect(volumes).To(HaveLen(2))
			Expect(volumes[0].Name).To(Equal("data"))
			Expect(volumes[0].DataVolume).NotTo(BeNil())
			Expect(volumes[0].DataVolume.Name).To(Equal("data"))
			Expect(volumes[1].Name).To(Equal("logs"))
		})

		It("should inject disk setup scripts into every role's userdata", func() {
			for _, role := range []string{"server", "client"} {
				result, err := w.UserdataForRole(role, "test-ns")
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("virtwork-disk-setup-data.sh"))
				Expect(result).To(ContainSubstring("virtwork-disk-setup-logs.sh"))
			}
		})

		It("should run disk setup scripts before daemon-reload in role userdata", func() {
			result, err := w.UserdataForRole("server", "test-ns")
			Expect(err).NotTo(HaveOccurred())
			parsed := parseYAML(result)
			runcmd := parsed["runcmd"].([]interface{})

			var lastSetupIdx, reloadIdx int
			for i, cmd := range runcmd {
				arr := cmd.([]interface{})
				if len(arr) == 1 && arr[0] == "/usr/local/bin/virtwork-disk-setup-logs.sh" {
					lastSetupIdx = i
				}
				if len(arr) == 2 && arr[0] == "systemctl" && arr[1] == "daemon-reload" {
					reloadIdx = i
				}
			}
			Expect(lastSetupIdx).To(BeNumerically("<", reloadIdx),
				"disk setup scripts should run before daemon-reload")
		})

		It("should include disk setup script content with correct serial and mount", func() {
			result, err := w.UserdataForRole("server", "test-ns")
			Expect(err).NotTo(HaveOccurred())
			parsed := parseYAML(result)
			files := parsed["write_files"].([]interface{})
			var found bool
			for _, f := range files {
				fm := f.(map[string]interface{})
				if fm["path"] == "/usr/local/bin/virtwork-disk-setup-data.sh" {
					found = true
					Expect(fm["permissions"]).To(Equal("0755"))
					Expect(fm["content"]).To(ContainSubstring("vw-data"))
					Expect(fm["content"]).To(ContainSubstring("/mnt/data"))
				}
			}
			Expect(found).To(BeTrue(), "expected disk setup script in write_files")
		})

		It("should not affect RoleDistribution or VMCount", func() {
			dist := w.RoleDistribution()
			Expect(dist).To(HaveLen(2))
			Expect(w.VMCount()).To(Equal(3))
		})
	})

	Context("multi-role entry with service declared", func() {
		BeforeEach(func() {
			entryDir := filepath.Join(catalogDir, "svc-multi")
			writeFile(entryDir, "workload.yaml", `roles:
  - name: server
    vm-count: 1
  - name: client
    vm-count: 2
service:
  ports:
    - name: iperf
      port: 5201
      protocol: TCP
  selector-role: server
`)
			writeFile(entryDir, "server.service", "[Service]\nExecStart=/bin/server\n")
			writeFile(entryDir, "client.service", "[Service]\nExecStart=/bin/client\n")
			entry, err := workloads.LoadCatalogEntry(catalogDir, "svc-multi")
			Expect(err).NotTo(HaveOccurred())
			w = workloads.NewGenericMultiWorkload(
				config.WorkloadConfig{CPUCores: 2, Memory: "2Gi"},
				entry, "test-ns", "", "", nil,
			)
		})

		It("should require a service", func() {
			Expect(w.RequiresService()).To(BeTrue())
		})

		It("should return a ServiceSpec with correct name and namespace", func() {
			svc := w.ServiceSpec()
			Expect(svc).NotTo(BeNil())
			Expect(svc.Name).To(Equal("virtwork-svc-multi"))
			Expect(svc.Namespace).To(Equal("test-ns"))
		})

		It("should use selector-role for the service selector", func() {
			svc := w.ServiceSpec()
			Expect(svc.Spec.Selector).To(HaveKeyWithValue("virtwork/role", "server"))
			Expect(svc.Spec.Selector).NotTo(HaveKey("app.kubernetes.io/name"))
		})

		It("should include all declared ports", func() {
			svc := w.ServiceSpec()
			Expect(svc.Spec.Ports).To(HaveLen(1))
			Expect(svc.Spec.Ports[0].Name).To(Equal("iperf"))
			Expect(svc.Spec.Ports[0].Port).To(Equal(int32(5201)))
			Expect(string(svc.Spec.Ports[0].Protocol)).To(Equal("TCP"))
		})

		It("should set standard labels on the service", func() {
			svc := w.ServiceSpec()
			Expect(svc.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", "virtwork"))
			Expect(svc.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "virtwork"))
			Expect(svc.Labels).To(HaveKeyWithValue("app.kubernetes.io/component", "svc-multi"))
		})
	})

	Context("multi-role entry with service but no selector-role", func() {
		BeforeEach(func() {
			entryDir := filepath.Join(catalogDir, "svc-no-role")
			writeFile(entryDir, "workload.yaml", `roles:
  - name: server
    vm-count: 1
  - name: client
    vm-count: 1
service:
  ports:
    - name: http
      port: 8080
      protocol: TCP
`)
			writeFile(entryDir, "server.service", "[Service]\nExecStart=/bin/server\n")
			writeFile(entryDir, "client.service", "[Service]\nExecStart=/bin/client\n")
			entry, err := workloads.LoadCatalogEntry(catalogDir, "svc-no-role")
			Expect(err).NotTo(HaveOccurred())
			w = workloads.NewGenericMultiWorkload(
				config.WorkloadConfig{CPUCores: 2, Memory: "2Gi"},
				entry, "test-ns", "", "", nil,
			)
		})

		It("should use app label as selector when no selector-role", func() {
			svc := w.ServiceSpec()
			Expect(svc.Spec.Selector).To(HaveKeyWithValue(
				"app.kubernetes.io/name", "virtwork-svc-no-role"))
			Expect(svc.Spec.Selector).NotTo(HaveKey("virtwork/role"))
		})
	})

	Context("multi-role entry without storage or service", func() {
		BeforeEach(func() {
			entryDir := filepath.Join(catalogDir, "plain-multi")
			writeFile(entryDir, "workload.yaml", `roles:
  - name: a
    vm-count: 1
  - name: b
    vm-count: 1
`)
			writeFile(entryDir, "a.service", "[Service]\nExecStart=/bin/a\n")
			writeFile(entryDir, "b.service", "[Service]\nExecStart=/bin/b\n")
			entry, err := workloads.LoadCatalogEntry(catalogDir, "plain-multi")
			Expect(err).NotTo(HaveOccurred())
			w = workloads.NewGenericMultiWorkload(
				config.WorkloadConfig{CPUCores: 2, Memory: "2Gi"},
				entry, "test-ns", "", "", nil,
			)
		})

		It("should not require a service", func() {
			Expect(w.RequiresService()).To(BeFalse())
		})

		It("should return nil ServiceSpec", func() {
			Expect(w.ServiceSpec()).To(BeNil())
		})

		It("should return nil for all storage methods", func() {
			Expect(w.ExtraVolumes()).To(BeNil())
			Expect(w.ExtraDisks()).To(BeNil())
			dvts, err := w.DataVolumeTemplates()
			Expect(err).NotTo(HaveOccurred())
			Expect(dvts).To(BeNil())
		})
	})

	Context("multi-role entry with both storage and service", func() {
		BeforeEach(func() {
			entryDir := filepath.Join(catalogDir, "full-multi")
			writeFile(entryDir, "workload.yaml", `roles:
  - name: primary
    vm-count: 1
  - name: replica
    vm-count: 2
packages:
  - postgresql
storage:
  - name: pgdata
    size: 20Gi
    serial: vw-pgdata
    mount: /var/lib/pgsql
service:
  ports:
    - name: postgres
      port: 5432
      protocol: TCP
  selector-role: primary
`)
			writeFile(entryDir, "primary.service", "[Service]\nExecStart=/bin/pg-primary\n")
			writeFile(entryDir, "replica.service", "[Service]\nExecStart=/bin/pg-replica\n")
			entry, err := workloads.LoadCatalogEntry(catalogDir, "full-multi")
			Expect(err).NotTo(HaveOccurred())
			w = workloads.NewGenericMultiWorkload(
				config.WorkloadConfig{CPUCores: 4, Memory: "4Gi"},
				entry, "test-ns", "", "", nil,
			)
		})

		It("should support both storage and service simultaneously", func() {
			dvts, err := w.DataVolumeTemplates()
			Expect(err).NotTo(HaveOccurred())
			Expect(dvts).To(HaveLen(1))

			Expect(w.RequiresService()).To(BeTrue())
			svc := w.ServiceSpec()
			Expect(svc.Spec.Ports).To(HaveLen(1))
			Expect(svc.Spec.Ports[0].Port).To(Equal(int32(5432)))
			Expect(svc.Spec.Selector).To(HaveKeyWithValue("virtwork/role", "primary"))
		})

		It("should include disk setup in all roles' userdata", func() {
			for _, role := range []string{"primary", "replica"} {
				result, err := w.UserdataForRole(role, "test-ns")
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("virtwork-disk-setup-pgdata.sh"))
				Expect(result).To(ContainSubstring("postgresql"))
			}
		})
	})
})
