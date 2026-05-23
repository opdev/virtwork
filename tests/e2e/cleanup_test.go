//go:build e2e

// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/opdev/virtwork/internal/testutil"
	"github.com/opdev/virtwork/internal/vm"
)

var _ = Describe("virtwork cleanup", Label("slow"), func() {
	var namespace string

	BeforeEach(func() {
		namespace = testutil.UniqueNamespace("e2e-clean")

		// Deploy resources first
		_, _, exitCode, err := testutil.RunVirtwork(
			"run", "--workloads", "cpu", "--vm-count", "1",
			"--namespace", namespace, "--no-wait")
		Expect(err).NotTo(HaveOccurred())
		Expect(exitCode).To(Equal(0))
	})

	AfterEach(func() {
		// Safety net cleanup
		_, _, _, _ = testutil.RunVirtwork("cleanup",
			"--namespace", namespace, "--delete-namespace")
	})

	It("should delete all managed resources", func() {
		stdout, _, exitCode, err := testutil.RunVirtwork(
			"cleanup", "--namespace", namespace)
		Expect(err).NotTo(HaveOccurred())
		Expect(exitCode).To(Equal(0))
		Expect(stdout).To(ContainSubstring(`"vms_deleted":1`))
	})

	It("should not delete the namespace by default", func() {
		stdout, _, exitCode, err := testutil.RunVirtwork(
			"cleanup", "--namespace", namespace)
		Expect(err).NotTo(HaveOccurred())
		Expect(exitCode).To(Equal(0))
		Expect(stdout).NotTo(ContainSubstring("namespace deleted"))
	})

	It("should delete namespace when --delete-namespace is set", func() {
		stdout, _, exitCode, err := testutil.RunVirtwork(
			"cleanup", "--namespace", namespace,
			"--delete-namespace")
		Expect(err).NotTo(HaveOccurred())
		Expect(exitCode).To(Equal(0))
		Expect(stdout).To(ContainSubstring("namespace"))
	})

	It("should be idempotent (cleanup twice succeeds)", func() {
		_, _, exitCode1, err := testutil.RunVirtwork(
			"cleanup", "--namespace", namespace)
		Expect(err).NotTo(HaveOccurred())
		Expect(exitCode1).To(Equal(0))

		// Wait for VMs to be fully removed (KubeVirt finalizers).
		c := testutil.MustConnect("")
		ctx := context.Background()
		Eventually(func() int {
			vms, _ := vm.ListVMs(ctx, c, namespace, testutil.ManagedLabels())
			return len(vms)
		}, 120*time.Second, 2*time.Second).Should(Equal(0))

		stdout, _, exitCode2, err := testutil.RunVirtwork(
			"cleanup", "--namespace", namespace)
		Expect(err).NotTo(HaveOccurred())
		Expect(exitCode2).To(Equal(0))
		Expect(stdout).To(ContainSubstring(`"vms_deleted":0`))
	})
})
