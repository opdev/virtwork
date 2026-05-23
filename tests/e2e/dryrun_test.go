//go:build e2e

// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/opdev/virtwork/internal/testutil"
)

var _ = Describe("virtwork run --dry-run", func() {
	It("should print VM specs without cluster connection", func() {
		stdout, _, exitCode, err := testutil.RunVirtwork("run", "--dry-run", "--workloads", "cpu")
		Expect(err).NotTo(HaveOccurred())
		Expect(exitCode).To(Equal(0))
		Expect(stdout).To(ContainSubstring("virtwork-cpu-0"))
	})

	It("should print specs for all workloads by default", func() {
		stdout, _, exitCode, err := testutil.RunVirtwork("run", "--dry-run")
		Expect(err).NotTo(HaveOccurred())
		Expect(exitCode).To(Equal(0))
		Expect(stdout).To(ContainSubstring("virtwork-cpu-0"))
		Expect(stdout).To(ContainSubstring("virtwork-memory-0"))
		Expect(stdout).To(ContainSubstring("virtwork-database-0"))
		Expect(stdout).To(ContainSubstring("virtwork-disk-0"))
		Expect(stdout).To(ContainSubstring("virtwork-network-server-0"))
		Expect(stdout).To(ContainSubstring("virtwork-network-client-0"))
	})

	It("should honor --vm-count", func() {
		stdout, _, exitCode, err := testutil.RunVirtwork(
			"run", "--dry-run", "--workloads", "cpu", "--vm-count", "3")
		Expect(err).NotTo(HaveOccurred())
		Expect(exitCode).To(Equal(0))
		Expect(stdout).To(ContainSubstring("virtwork-cpu-0"))
		Expect(stdout).To(ContainSubstring("virtwork-cpu-1"))
		Expect(stdout).To(ContainSubstring("virtwork-cpu-2"))
	})

	It("should honor --vm-count for network workload pairs", func() {
		stdout, _, exitCode, err := testutil.RunVirtwork(
			"run", "--dry-run", "--workloads", "network", "--vm-count", "2")
		Expect(err).NotTo(HaveOccurred())
		Expect(exitCode).To(Equal(0))
		// 2 pairs = 4 VMs total
		Expect(stdout).To(ContainSubstring("virtwork-network-server-0"))
		Expect(stdout).To(ContainSubstring("virtwork-network-client-0"))
		Expect(stdout).To(ContainSubstring("virtwork-network-server-1"))
		Expect(stdout).To(ContainSubstring("virtwork-network-client-1"))
	})

	It("should exit 0 even with invalid kubeconfig (dry-run skips cluster)", func() {
		_, _, exitCode, err := testutil.RunVirtwork(
			"run", "--dry-run", "--workloads", "cpu",
			"--kubeconfig", "/nonexistent/path")
		Expect(err).NotTo(HaveOccurred())
		Expect(exitCode).To(Equal(0))
	})

	It("should include SSH user in output when --ssh-user is set", func() {
		stdout, _, exitCode, err := testutil.RunVirtwork(
			"run", "--dry-run", "--workloads", "cpu",
			"--ssh-user", "testuser")
		Expect(err).NotTo(HaveOccurred())
		Expect(exitCode).To(Equal(0))
		Expect(stdout).To(ContainSubstring("testuser"))
	})

	It("should fail for unknown workload name", func() {
		_, stderr, exitCode, err := testutil.RunVirtwork(
			"run", "--dry-run", "--workloads", "nonexistent")
		Expect(err).NotTo(HaveOccurred())
		Expect(exitCode).NotTo(Equal(0))
		Expect(stderr).To(ContainSubstring("workload not found"))
	})
})
