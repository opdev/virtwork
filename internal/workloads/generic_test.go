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

	Context("entry with storage declared", func() {
		BeforeEach(func() {
			entryDir := filepath.Join(catalogDir, "with-storage")
			writeFile(entryDir, "workload.yaml", `description: "Storage workload"
storage:
  - name: data
    size: 10Gi
    serial: vw-data
    mount: /mnt/data
`)
			writeFile(entryDir, "workload.service", "[Service]\nExecStart=/bin/app\n")
			entry, err := workloads.LoadCatalogEntry(catalogDir, "with-storage")
			Expect(err).NotTo(HaveOccurred())
			w = workloads.NewGenericWorkload(
				config.WorkloadConfig{CPUCores: 2, Memory: "2Gi"},
				entry, "test-ns", "", "", nil,
			)
		})

		It("should return DataVolumeTemplates for declared storage", func() {
			dvts, err := w.DataVolumeTemplates()
			Expect(err).NotTo(HaveOccurred())
			Expect(dvts).To(HaveLen(1))
			Expect(dvts[0].Name).To(Equal("data"))
		})

		It("should return ExtraDisks with correct serial and bus", func() {
			disks := w.ExtraDisks()
			Expect(disks).To(HaveLen(1))
			Expect(disks[0].Name).To(Equal("data"))
			Expect(disks[0].Serial).To(Equal("vw-data"))
			Expect(disks[0].DiskDevice.Disk).NotTo(BeNil())
			Expect(string(disks[0].DiskDevice.Disk.Bus)).To(Equal("virtio"))
		})

		It("should return ExtraVolumes referencing the DataVolume", func() {
			volumes := w.ExtraVolumes()
			Expect(volumes).To(HaveLen(1))
			Expect(volumes[0].Name).To(Equal("data"))
			Expect(volumes[0].DataVolume).NotTo(BeNil())
			Expect(volumes[0].DataVolume.Name).To(Equal("data"))
		})

		It("should inject disk setup script into cloud-init write_files", func() {
			result, err := w.CloudInitUserdata()
			Expect(err).NotTo(HaveOccurred())
			parsed := parseYAML(result)
			files := parsed["write_files"].([]interface{})
			var setupScript map[string]interface{}
			for _, f := range files {
				fm := f.(map[string]interface{})
				if fm["path"] == "/usr/local/bin/virtwork-disk-setup-data.sh" {
					setupScript = fm
				}
			}
			Expect(setupScript).NotTo(BeNil(), "expected disk setup script in write_files")
			Expect(setupScript["permissions"]).To(Equal("0755"))
			Expect(setupScript["content"]).To(ContainSubstring("vw-data"))
			Expect(setupScript["content"]).To(ContainSubstring("/mnt/data"))
		})

		It("should run disk setup script before daemon-reload in runcmd", func() {
			result, err := w.CloudInitUserdata()
			Expect(err).NotTo(HaveOccurred())
			parsed := parseYAML(result)
			runcmd := parsed["runcmd"].([]interface{})

			var setupIdx, reloadIdx int
			for i, cmd := range runcmd {
				arr := cmd.([]interface{})
				if len(arr) == 1 && arr[0] == "/usr/local/bin/virtwork-disk-setup-data.sh" {
					setupIdx = i
				}
				if len(arr) == 2 && arr[0] == "systemctl" && arr[1] == "daemon-reload" {
					reloadIdx = i
				}
			}
			Expect(setupIdx).To(BeNumerically("<", reloadIdx),
				"disk setup script should run before daemon-reload")
		})
	})

	Context("entry with multiple storage definitions", func() {
		BeforeEach(func() {
			entryDir := filepath.Join(catalogDir, "multi-storage")
			writeFile(entryDir, "workload.yaml", `storage:
  - name: data
    size: 10Gi
    serial: vw-data
    mount: /mnt/data
  - name: logs
    size: 5Gi
    serial: vw-logs
    mount: /var/log/app
`)
			writeFile(entryDir, "workload.service", "[Service]\nExecStart=/bin/app\n")
			entry, err := workloads.LoadCatalogEntry(catalogDir, "multi-storage")
			Expect(err).NotTo(HaveOccurred())
			w = workloads.NewGenericWorkload(
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

		It("should return ExtraDisks for all storage entries", func() {
			disks := w.ExtraDisks()
			Expect(disks).To(HaveLen(2))
			Expect(disks[0].Serial).To(Equal("vw-data"))
			Expect(disks[1].Serial).To(Equal("vw-logs"))
		})

		It("should return ExtraVolumes for all storage entries", func() {
			volumes := w.ExtraVolumes()
			Expect(volumes).To(HaveLen(2))
			Expect(volumes[0].Name).To(Equal("data"))
			Expect(volumes[1].Name).To(Equal("logs"))
		})

		It("should inject disk setup scripts for each storage entry", func() {
			result, err := w.CloudInitUserdata()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("virtwork-disk-setup-data.sh"))
			Expect(result).To(ContainSubstring("virtwork-disk-setup-logs.sh"))
		})
	})

	Context("entry with service declared", func() {
		BeforeEach(func() {
			entryDir := filepath.Join(catalogDir, "with-svc-def")
			writeFile(entryDir, "workload.yaml", `description: "Service workload"
service:
  ports:
    - name: http
      port: 8080
      protocol: TCP
    - name: metrics
      port: 9090
      protocol: UDP
`)
			writeFile(entryDir, "workload.service", "[Service]\nExecStart=/bin/app\n")
			entry, err := workloads.LoadCatalogEntry(catalogDir, "with-svc-def")
			Expect(err).NotTo(HaveOccurred())
			w = workloads.NewGenericWorkload(
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
			Expect(svc.Name).To(Equal("virtwork-with-svc-def"))
			Expect(svc.Namespace).To(Equal("test-ns"))
		})

		It("should set standard labels on the service", func() {
			svc := w.ServiceSpec()
			Expect(svc.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", "virtwork"))
			Expect(svc.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "virtwork"))
			Expect(svc.Labels).To(HaveKeyWithValue("app.kubernetes.io/component", "with-svc-def"))
		})

		It("should use app label as selector for single-role", func() {
			svc := w.ServiceSpec()
			Expect(svc.Spec.Selector).To(HaveKeyWithValue(
				"app.kubernetes.io/name", "virtwork-with-svc-def"))
		})

		It("should include all declared ports", func() {
			svc := w.ServiceSpec()
			Expect(svc.Spec.Ports).To(HaveLen(2))
			Expect(svc.Spec.Ports[0].Name).To(Equal("http"))
			Expect(svc.Spec.Ports[0].Port).To(Equal(int32(8080)))
			Expect(string(svc.Spec.Ports[0].Protocol)).To(Equal("TCP"))
			Expect(svc.Spec.Ports[1].Name).To(Equal("metrics"))
			Expect(svc.Spec.Ports[1].Port).To(Equal(int32(9090)))
			Expect(string(svc.Spec.Ports[1].Protocol)).To(Equal("UDP"))
		})
	})

	Context("entry without service or storage", func() {
		BeforeEach(func() {
			entryDir := filepath.Join(catalogDir, "plain")
			writeFile(entryDir, "workload.service", "[Service]\nExecStart=/bin/app\n")
			entry, err := workloads.LoadCatalogEntry(catalogDir, "plain")
			Expect(err).NotTo(HaveOccurred())
			w = workloads.NewGenericWorkload(
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

	Context("entry with both storage and service", func() {
		BeforeEach(func() {
			entryDir := filepath.Join(catalogDir, "full")
			writeFile(entryDir, "workload.yaml", `description: "Full featured"
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
`)
			writeFile(entryDir, "workload.service", "[Service]\nExecStart=/bin/pg\n")
			entry, err := workloads.LoadCatalogEntry(catalogDir, "full")
			Expect(err).NotTo(HaveOccurred())
			w = workloads.NewGenericWorkload(
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
		})

		It("should include both disk setup and service in cloud-init", func() {
			result, err := w.CloudInitUserdata()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("virtwork-disk-setup-pgdata.sh"))
			Expect(result).To(ContainSubstring("postgresql"))
		})
	})
})
