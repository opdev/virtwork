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
				"--audit-db", "/nonexistent/deeply/nested/path/db.sqlite",
			})
			err := rootCmd.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("auditor"))
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
