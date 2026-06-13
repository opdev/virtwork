// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("runE", func() {
	Context("error paths", func() {
		It("returns error for nonexistent config file", func() {
			rootCmd := newRootCmd()
			rootCmd.SetArgs([]string{"run", "--config", "/nonexistent/config.yaml"})
			err := rootCmd.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("config"))
		})

		It("returns error for invalid workload name", func() {
			rootCmd := newRootCmd()
			rootCmd.SetArgs([]string{
				"run", "--dry-run", "--workloads", "bogus-workload", "--no-audit",
			})
			err := rootCmd.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("bogus-workload"))
		})

		It("returns error when cluster connection fails in non-dry-run mode", func() {
			tmpDir := GinkgoT().TempDir()
			badKubeconfig := filepath.Join(tmpDir, "kubeconfig")
			Expect(os.WriteFile(badKubeconfig, []byte(`not valid yaml{{{`), 0o600)).To(Succeed())

			rootCmd := newRootCmd()
			rootCmd.SetArgs([]string{
				"run",
				"--workloads", "cpu",
				"--no-audit",
				"--kubeconfig", badKubeconfig,
			})
			err := rootCmd.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("connecting to cluster"))
		})

		It("returns error when initAuditor fails", func() {
			rootCmd := newRootCmd()
			rootCmd.SetArgs([]string{
				"run",
				"--dry-run",
				"--workloads", "cpu",
				"--audit-db", "/dev/null/impossible/db.sqlite",
			})
			err := rootCmd.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("auditor"))
		})
	})

	Context("catalog workloads", func() {
		var catalogDir string

		writeFile := func(dir, name, content string) {
			err := os.MkdirAll(dir, 0o750)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			err = os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
		}

		BeforeEach(func() {
			var err error
			catalogDir, err = os.MkdirTemp("", "virtwork-cli-catalog-*")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			Expect(os.RemoveAll(catalogDir)).To(Succeed())
		})

		It("runs only catalog entry when --from-catalog set without --workloads", func() {
			entryDir := filepath.Join(catalogDir, "my-svc")
			writeFile(entryDir, "workload.service", "[Service]\nExecStart=/bin/true\n")

			rootCmd := newRootCmd()
			var buf bytes.Buffer
			rootCmd.SetOut(&buf)
			rootCmd.SetArgs([]string{
				"run", "--dry-run", "--no-audit",
				"--catalog-dir", catalogDir,
				"--from-catalog", "my-svc",
			})
			Expect(rootCmd.Execute()).To(Succeed())

			output := buf.String()
			Expect(output).To(ContainSubstring("my-svc"))
			Expect(output).NotTo(ContainSubstring("virtwork-cpu"))
		})

		It("runs both catalog and built-in when both flags set", func() {
			entryDir := filepath.Join(catalogDir, "my-svc")
			writeFile(entryDir, "workload.service", "[Service]\nExecStart=/bin/true\n")

			rootCmd := newRootCmd()
			var buf bytes.Buffer
			rootCmd.SetOut(&buf)
			rootCmd.SetArgs([]string{
				"run", "--dry-run", "--no-audit",
				"--workloads", "cpu",
				"--catalog-dir", catalogDir,
				"--from-catalog", "my-svc",
			})
			Expect(rootCmd.Execute()).To(Succeed())

			output := buf.String()
			Expect(output).To(ContainSubstring("virtwork-cpu"))
			Expect(output).To(ContainSubstring("my-svc"))
		})

		It("returns error for invalid catalog entry", func() {
			rootCmd := newRootCmd()
			rootCmd.SetArgs([]string{
				"run", "--dry-run", "--no-audit",
				"--catalog-dir", catalogDir,
				"--from-catalog", "nonexistent",
			})
			err := rootCmd.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("nonexistent"))
		})
	})

	Context("dry-run mode", func() {
		It("succeeds without a cluster connection", func() {
			rootCmd := newRootCmd()
			var buf bytes.Buffer
			rootCmd.SetOut(&buf)
			rootCmd.SetArgs([]string{
				"run", "--dry-run", "--workloads", "cpu", "--no-audit",
			})
			Expect(rootCmd.Execute()).To(Succeed())

			output := buf.String()
			Expect(output).To(ContainSubstring("virtwork-cpu"))
		})

		It("succeeds with multiple workloads", func() {
			rootCmd := newRootCmd()
			var buf bytes.Buffer
			rootCmd.SetOut(&buf)
			rootCmd.SetArgs([]string{
				"run", "--dry-run", "--workloads", "cpu,memory", "--no-audit",
			})
			Expect(rootCmd.Execute()).To(Succeed())

			output := buf.String()
			Expect(output).To(ContainSubstring("virtwork-cpu"))
			Expect(output).To(ContainSubstring("virtwork-memory"))
		})

		It("prints VM YAML specs to output", func() {
			rootCmd := newRootCmd()
			var buf bytes.Buffer
			rootCmd.SetOut(&buf)
			rootCmd.SetArgs([]string{
				"run", "--dry-run", "--workloads", "cpu", "--no-audit",
			})
			Expect(rootCmd.Execute()).To(Succeed())

			output := buf.String()
			Expect(output).To(ContainSubstring("kind: VirtualMachine"))
		})

		It("respects --vm-count flag", func() {
			rootCmd := newRootCmd()
			var buf bytes.Buffer
			rootCmd.SetOut(&buf)
			rootCmd.SetArgs([]string{
				"run", "--dry-run", "--workloads", "cpu", "--vm-count", "3", "--no-audit",
			})
			Expect(rootCmd.Execute()).To(Succeed())

			output := buf.String()
			Expect(output).To(ContainSubstring("virtwork-cpu-0"))
			Expect(output).To(ContainSubstring("virtwork-cpu-1"))
			Expect(output).To(ContainSubstring("virtwork-cpu-2"))
		})
	})
})
