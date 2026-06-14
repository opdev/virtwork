// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads_test

import (
	"os"
	"path/filepath"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/workloads"
)

func projectRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

var _ = Describe("Shipped catalog entries", func() {
	catalogDir := filepath.Join(projectRoot(), "catalog")

	var entries []string

	BeforeEach(func() {
		dirEntries, err := os.ReadDir(catalogDir)
		if os.IsNotExist(err) {
			Skip("catalog directory not found at " + catalogDir)
		}
		Expect(err).NotTo(HaveOccurred())

		entries = nil
		for _, d := range dirEntries {
			if d.IsDir() {
				entries = append(entries, d.Name())
			}
		}
		if len(entries) == 0 {
			Skip("no catalog entries found in " + catalogDir)
		}
	})

	It("should discover at least one entry", func() {
		Expect(entries).NotTo(BeEmpty())
	})

	It("should load, validate, and produce working workloads for every entry", func() {
		for _, name := range entries {
			By("validating entry: " + name)

			entry, err := workloads.LoadCatalogEntry(catalogDir, name)
			Expect(err).NotTo(HaveOccurred(), "LoadCatalogEntry failed for %s", name)

			schema := entry.Schema()
			Expect(schema).NotTo(BeNil(), "Schema() returned nil for %s", name)

			factory := entry.Factory()
			Expect(factory).NotTo(BeNil(), "Factory() returned nil for %s", name)

			cfg := config.WorkloadConfig{VMCount: 1, CPUCores: 2, Memory: "2Gi"}
			opts := &workloads.RegistryOpts{
				Namespace:   "test",
				SSHUser:     "virtwork",
				SSHPassword: "test",
			}
			wl := factory(cfg, opts)
			Expect(wl).NotTo(BeNil(), "factory produced nil workload for %s", name)
			Expect(wl.Name()).To(Equal(name), "Name() mismatch for %s", name)

			userdata, err := wl.CloudInitUserdata()
			Expect(err).NotTo(HaveOccurred(), "CloudInitUserdata failed for %s", name)
			Expect(userdata).To(HavePrefix("#cloud-config"), "userdata missing #cloud-config prefix for %s", name)

			if entry.IsMultiRole() {
				multi, ok := wl.(workloads.MultiVMWorkload)
				Expect(ok).To(BeTrue(), "multi-role entry %s did not produce MultiVMWorkload", name)

				roles := multi.RoleDistribution()
				Expect(roles).NotTo(BeEmpty(), "RoleDistribution empty for multi-role entry %s", name)

				for _, rs := range roles {
					roleUserdata, roleErr := multi.UserdataForRole(rs.Role, "test")
					Expect(roleErr).NotTo(HaveOccurred(), "UserdataForRole(%s) failed for %s", rs.Role, name)
					Expect(roleUserdata).To(
						HavePrefix("#cloud-config"),
						"role %s userdata missing #cloud-config for %s", rs.Role, name)
				}
			}

			if len(entry.Manifest.Storage) > 0 {
				dvts, dvtErr := wl.DataVolumeTemplates()
				Expect(dvtErr).NotTo(HaveOccurred(), "DataVolumeTemplates failed for %s", name)
				Expect(dvts).To(HaveLen(len(entry.Manifest.Storage)),
					"DataVolumeTemplates count mismatch for %s: expected %d", name, len(entry.Manifest.Storage))
			}

			if entry.Manifest.Service != nil {
				Expect(wl.RequiresService()).To(BeTrue(), "RequiresService should be true for %s", name)
			}
		}
	})
})
