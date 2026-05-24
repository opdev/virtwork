//go:build e2e

// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/opdev/virtwork/internal/testutil"
)

var _ = Describe("virtwork run", Label("slow"), func() {
	var namespace string

	BeforeEach(func() {
		namespace = testutil.UniqueNamespace("e2e-run")
	})

	AfterEach(func() {
		_, stderr, exitCode, err := testutil.RunVirtwork("cleanup",
			"--namespace", namespace, "--delete-namespace", "--yes")
		if err != nil || exitCode != 0 {
			GinkgoWriter.Printf("AfterEach cleanup failed (ns=%s exit=%d): err=%v stderr=%s\n",
				namespace, exitCode, err, stderr)
		}
	})

	It("should deploy a single CPU workload with --no-wait", func() {
		stdout, _, exitCode, err := testutil.RunVirtwork(
			"run", "--workloads", "cpu", "--vm-count", "1",
			"--namespace", namespace, "--no-wait")
		Expect(err).NotTo(HaveOccurred())
		Expect(exitCode).To(Equal(0))
		Expect(stdout).To(ContainSubstring(namespace))
		Expect(stdout).To(ContainSubstring("virtwork-cpu-0"))
	})

	It("should deploy multiple workloads", func() {
		stdout, _, exitCode, err := testutil.RunVirtwork(
			"run", "--workloads", "cpu,memory",
			"--namespace", namespace, "--no-wait")
		Expect(err).NotTo(HaveOccurred())
		Expect(exitCode).To(Equal(0))
		Expect(stdout).To(ContainSubstring("virtwork-cpu-0"))
		Expect(stdout).To(ContainSubstring("virtwork-memory-0"))
	})

	It("should deploy network workload with service", func() {
		stdout, _, exitCode, err := testutil.RunVirtwork(
			"run", "--workloads", "network", "--vm-count", "1",
			"--namespace", namespace, "--no-wait")
		Expect(err).NotTo(HaveOccurred())
		Expect(exitCode).To(Equal(0))
		Expect(stdout).To(ContainSubstring("virtwork-network-server-0"))
		Expect(stdout).To(ContainSubstring("virtwork-network-client-0"))
	})

	It("should wait for VM readiness when not using --no-wait", func() {
		stdout, _, exitCode, err := testutil.RunVirtwork(
			"run", "--workloads", "cpu", "--vm-count", "1",
			"--namespace", namespace, "--timeout", "300")
		Expect(err).NotTo(HaveOccurred())
		Expect(exitCode).To(Equal(0))
		Expect(stdout).To(ContainSubstring("ready"))
	})

	It("should fail with nonexistent kubeconfig", func() {
		_, stderr, exitCode, err := testutil.RunVirtwork(
			"run", "--workloads", "cpu",
			"--kubeconfig", "/nonexistent/path",
			"--namespace", namespace)
		Expect(err).NotTo(HaveOccurred())
		Expect(exitCode).NotTo(Equal(0))
		Expect(stderr).NotTo(BeEmpty())
	})
})
