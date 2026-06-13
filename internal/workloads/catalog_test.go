// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/opdev/virtwork/internal/workloads"
)

var _ = Describe("CatalogEntry", func() {
	var catalogDir string

	BeforeEach(func() {
		var err error
		catalogDir, err = os.MkdirTemp("", "virtwork-catalog-*")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		Expect(os.RemoveAll(catalogDir)).To(Succeed())
	})

	writeFile := func(dir, name, content string) {
		err := os.MkdirAll(dir, 0o750)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		err = os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
	}

	Describe("LoadCatalogEntry", func() {
		Context("single-role entry with no manifest", func() {
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
			})

			It("should load successfully", func() {
				entry, err := workloads.LoadCatalogEntry(catalogDir, "my-stress")
				Expect(err).NotTo(HaveOccurred())
				Expect(entry.Name).To(Equal("my-stress"))
				Expect(entry.ServiceFiles).To(HaveLen(1))
				Expect(entry.ServiceFiles).To(HaveKey("workload.service"))
			})

			It("should have empty manifest fields", func() {
				entry, err := workloads.LoadCatalogEntry(catalogDir, "my-stress")
				Expect(err).NotTo(HaveOccurred())
				Expect(entry.Manifest.Packages).To(BeEmpty())
				Expect(entry.Manifest.Params).To(BeEmpty())
				Expect(entry.Manifest.Roles).To(BeEmpty())
			})

			It("should not be multi-role", func() {
				entry, err := workloads.LoadCatalogEntry(catalogDir, "my-stress")
				Expect(err).NotTo(HaveOccurred())
				Expect(entry.IsMultiRole()).To(BeFalse())
			})

			It("should return empty param schema", func() {
				entry, err := workloads.LoadCatalogEntry(catalogDir, "my-stress")
				Expect(err).NotTo(HaveOccurred())
				Expect(entry.Schema()).To(BeEmpty())
			})
		})

		Context("single-role entry with manifest", func() {
			BeforeEach(func() {
				entryDir := filepath.Join(catalogDir, "my-cpu-lite")
				writeFile(entryDir, "workload.yaml", `description: "Lightweight CPU stress"
packages:
  - stress-ng
params:
  - key: cpu-load
    type: int
    default: "50"
    desc: "CPU load percentage"
  - key: method
    type: string
    default: "all"
    desc: "stress-ng method"
`)
				writeFile(entryDir, "workload.service", `[Unit]
Description=CPU lite

[Service]
ExecStart=/usr/bin/stress-ng --cpu 0 --cpu-load {{cpu-load}} --cpu-method {{method}}
Restart=always

[Install]
WantedBy=multi-user.target
`)
			})

			It("should load manifest fields", func() {
				entry, err := workloads.LoadCatalogEntry(catalogDir, "my-cpu-lite")
				Expect(err).NotTo(HaveOccurred())
				Expect(entry.Manifest.Description).To(Equal("Lightweight CPU stress"))
				Expect(entry.Manifest.Packages).To(ConsistOf("stress-ng"))
			})

			It("should parse param schema from manifest", func() {
				entry, err := workloads.LoadCatalogEntry(catalogDir, "my-cpu-lite")
				Expect(err).NotTo(HaveOccurred())
				schema := entry.Schema()
				Expect(schema).To(HaveLen(2))
				Expect(schema[0].Key).To(Equal("cpu-load"))
				Expect(schema[0].Type).To(Equal(workloads.ParamInt))
				Expect(schema[0].Default).To(Equal("50"))
				Expect(schema[1].Key).To(Equal("method"))
				Expect(schema[1].Type).To(Equal(workloads.ParamString))
			})

			It("should not be multi-role", func() {
				entry, err := workloads.LoadCatalogEntry(catalogDir, "my-cpu-lite")
				Expect(err).NotTo(HaveOccurred())
				Expect(entry.IsMultiRole()).To(BeFalse())
			})
		})

		Context("multi-role entry", func() {
			BeforeEach(func() {
				entryDir := filepath.Join(catalogDir, "my-benchmark")
				writeFile(entryDir, "workload.yaml", `description: "Server/client benchmark"
packages:
  - iperf3
params:
  - key: duration
    type: int
    default: "60"
    desc: "Test duration"
roles:
  - name: server
    vm-count: 1
  - name: client
    vm-count: 2
`)
				writeFile(entryDir, "server.service", `[Unit]
Description=Benchmark server

[Service]
ExecStart=/usr/bin/iperf3 -s
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
			})

			It("should load successfully", func() {
				entry, err := workloads.LoadCatalogEntry(catalogDir, "my-benchmark")
				Expect(err).NotTo(HaveOccurred())
				Expect(entry.Name).To(Equal("my-benchmark"))
				Expect(entry.IsMultiRole()).To(BeTrue())
			})

			It("should have role definitions from manifest", func() {
				entry, err := workloads.LoadCatalogEntry(catalogDir, "my-benchmark")
				Expect(err).NotTo(HaveOccurred())
				Expect(entry.Manifest.Roles).To(HaveLen(2))
				Expect(entry.Manifest.Roles[0].Name).To(Equal("server"))
				Expect(entry.Manifest.Roles[0].VMCount).To(Equal(1))
				Expect(entry.Manifest.Roles[1].Name).To(Equal("client"))
				Expect(entry.Manifest.Roles[1].VMCount).To(Equal(2))
			})

			It("should map service files to roles", func() {
				entry, err := workloads.LoadCatalogEntry(catalogDir, "my-benchmark")
				Expect(err).NotTo(HaveOccurred())
				Expect(entry.ServiceFiles).To(HaveKey("server"))
				Expect(entry.ServiceFiles).To(HaveKey("client"))
				Expect(entry.ServiceFiles["server"]).To(ContainSubstring("iperf3 -s"))
				Expect(entry.ServiceFiles["client"]).To(ContainSubstring("iperf3 -c"))
			})

			It("should return param schema", func() {
				entry, err := workloads.LoadCatalogEntry(catalogDir, "my-benchmark")
				Expect(err).NotTo(HaveOccurred())
				schema := entry.Schema()
				Expect(schema).To(HaveLen(1))
				Expect(schema[0].Key).To(Equal("duration"))
			})
		})

		Context("multi-role with flexible role names", func() {
			BeforeEach(func() {
				entryDir := filepath.Join(catalogDir, "multi-tier")
				writeFile(entryDir, "workload.yaml", `description: "Three-tier workload"
roles:
  - name: frontend
    vm-count: 2
  - name: backend
    vm-count: 1
`)
				writeFile(entryDir, "frontend.service", "[Service]\nExecStart=/bin/frontend\n")
				writeFile(entryDir, "backend.service", "[Service]\nExecStart=/bin/backend\n")
			})

			It("should accept arbitrary role names", func() {
				entry, err := workloads.LoadCatalogEntry(catalogDir, "multi-tier")
				Expect(err).NotTo(HaveOccurred())
				Expect(entry.Manifest.Roles[0].Name).To(Equal("frontend"))
				Expect(entry.Manifest.Roles[1].Name).To(Equal("backend"))
				Expect(entry.ServiceFiles).To(HaveKey("frontend"))
				Expect(entry.ServiceFiles).To(HaveKey("backend"))
			})
		})

		Context("multiple service files in single-role entry", func() {
			BeforeEach(func() {
				entryDir := filepath.Join(catalogDir, "multi-svc")
				writeFile(entryDir, "app.service", "[Service]\nExecStart=/bin/app\n")
				writeFile(entryDir, "monitor.service", "[Service]\nExecStart=/bin/monitor\n")
			})

			It("should discover all service files", func() {
				entry, err := workloads.LoadCatalogEntry(catalogDir, "multi-svc")
				Expect(err).NotTo(HaveOccurred())
				Expect(entry.ServiceFiles).To(HaveLen(2))
				Expect(entry.ServiceFiles).To(HaveKey("app.service"))
				Expect(entry.ServiceFiles).To(HaveKey("monitor.service"))
			})
		})

		Context("param type parsing", func() {
			BeforeEach(func() {
				entryDir := filepath.Join(catalogDir, "typed-params")
				writeFile(entryDir, "workload.yaml", `params:
  - key: flag
    type: bool
    default: "true"
    desc: "A boolean flag"
  - key: items
    type: list
    default: "a;b;c"
    desc: "A list param"
  - key: opts
    type: dict
    default: "k1=v1;k2=v2"
    desc: "A dict param"
`)
				writeFile(entryDir, "workload.service", "[Service]\nExecStart=/bin/test\n")
			})

			It("should parse all param types", func() {
				entry, err := workloads.LoadCatalogEntry(catalogDir, "typed-params")
				Expect(err).NotTo(HaveOccurred())
				schema := entry.Schema()
				Expect(schema).To(HaveLen(3))
				Expect(schema[0].Type).To(Equal(workloads.ParamBool))
				Expect(schema[1].Type).To(Equal(workloads.ParamList))
				Expect(schema[2].Type).To(Equal(workloads.ParamDict))
			})
		})

		Context("error cases", func() {
			It("should return ErrCatalogEntryNotFound for missing directory", func() {
				_, err := workloads.LoadCatalogEntry(catalogDir, "nonexistent")
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring(workloads.ErrCatalogEntryNotFound.Error())))
			})

			It("should return ErrCatalogNoServices when no .service files exist", func() {
				entryDir := filepath.Join(catalogDir, "empty-entry")
				writeFile(entryDir, "workload.yaml", "description: empty\n")

				_, err := workloads.LoadCatalogEntry(catalogDir, "empty-entry")
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring(workloads.ErrCatalogNoServices.Error())))
			})

			It("should return ErrCatalogManifestRequired for multi-role without manifest", func() {
				entryDir := filepath.Join(catalogDir, "no-manifest-multi")
				writeFile(entryDir, "server.service", "[Service]\nExecStart=/bin/s\n")
				writeFile(entryDir, "client.service", "[Service]\nExecStart=/bin/c\n")

				entry, err := workloads.LoadCatalogEntry(catalogDir, "no-manifest-multi")
				Expect(err).NotTo(HaveOccurred())
				Expect(entry.IsMultiRole()).To(BeFalse())
				Expect(entry.ServiceFiles).To(HaveLen(2))
			})

			It("should return ErrCatalogMissingRoleService when role has no matching service", func() {
				entryDir := filepath.Join(catalogDir, "missing-role-svc")
				writeFile(entryDir, "workload.yaml", `roles:
  - name: server
    vm-count: 1
  - name: client
    vm-count: 1
`)
				writeFile(entryDir, "server.service", "[Service]\nExecStart=/bin/s\n")

				_, err := workloads.LoadCatalogEntry(catalogDir, "missing-role-svc")
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring(workloads.ErrCatalogMissingRoleService.Error())))
			})
		})
	})

	Describe("Factory", func() {
		It("should return a factory that creates GenericWorkload for single-role", func() {
			entryDir := filepath.Join(catalogDir, "single")
			writeFile(entryDir, "workload.service", "[Service]\nExecStart=/bin/test\n")

			entry, err := workloads.LoadCatalogEntry(catalogDir, "single")
			Expect(err).NotTo(HaveOccurred())

			factory := entry.Factory()
			Expect(factory).NotTo(BeNil())
		})

		It("should return a factory that creates GenericMultiWorkload for multi-role", func() {
			entryDir := filepath.Join(catalogDir, "multi")
			writeFile(entryDir, "workload.yaml", `roles:
  - name: server
    vm-count: 1
  - name: client
    vm-count: 1
`)
			writeFile(entryDir, "server.service", "[Service]\nExecStart=/bin/s\n")
			writeFile(entryDir, "client.service", "[Service]\nExecStart=/bin/c\n")

			entry, err := workloads.LoadCatalogEntry(catalogDir, "multi")
			Expect(err).NotTo(HaveOccurred())

			factory := entry.Factory()
			Expect(factory).NotTo(BeNil())
		})
	})
})
