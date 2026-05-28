//go:build e2e

// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opdev/virtwork/internal/testutil"
	"github.com/opdev/virtwork/internal/vm"
)

var _ = Describe("TPS workload", Label("slow"), func() {
	var namespace string

	BeforeEach(func() {
		namespace = testutil.UniqueNamespace("e2e-tps")
	})

	AfterEach(func() {
		_, stderr, exitCode, err := testutil.RunVirtwork("cleanup",
			"--namespace", namespace, "--delete-namespace", "--yes")
		if err != nil || exitCode != 0 {
			GinkgoWriter.Printf("AfterEach cleanup failed (ns=%s exit=%d): err=%v stderr=%s\n",
				namespace, exitCode, err, stderr)
		}
	})

	It("should deploy server and client VMs with a service", func() {
		stdout, _, exitCode, err := testutil.RunVirtwork(
			"run", "--workloads", "tps", "--vm-count", "1",
			"--namespace", namespace, "--no-wait")
		Expect(err).NotTo(HaveOccurred())
		Expect(exitCode).To(Equal(0))
		Expect(stdout).To(ContainSubstring("virtwork-tps-server-0"))
		Expect(stdout).To(ContainSubstring("virtwork-tps-client-0"))

		c := testutil.MustConnect("")
		ctx := context.Background()

		svc := &corev1.Service{}
		err = c.Get(ctx, client.ObjectKey{
			Name: "virtwork-tps-server", Namespace: namespace,
		}, svc)
		Expect(err).NotTo(HaveOccurred())
		Expect(svc.Spec.Ports).To(HaveLen(3))

		vms, err := vm.ListVMs(ctx, c, namespace, testutil.ManagedLabels())
		Expect(err).NotTo(HaveOccurred())
		Expect(vms).To(HaveLen(2))
	})

	It("should clean up all TPS resources", func() {
		_, _, exitCode, err := testutil.RunVirtwork(
			"run", "--workloads", "tps", "--vm-count", "1",
			"--namespace", namespace, "--no-wait")
		Expect(err).NotTo(HaveOccurred())
		Expect(exitCode).To(Equal(0))

		stdout, _, exitCode, err := testutil.RunVirtwork(
			"cleanup", "--namespace", namespace, "--yes")
		Expect(err).NotTo(HaveOccurred())
		Expect(exitCode).To(Equal(0))
		Expect(stdout).To(ContainSubstring(`"vms_deleted":2`))

		c := testutil.MustConnect("")
		ctx := context.Background()
		Eventually(func() int {
			vms, err := vm.ListVMs(ctx, c, namespace, testutil.ManagedLabels())
			Expect(err).NotTo(HaveOccurred())
			return len(vms)
		}, 120*time.Second, 2*time.Second).Should(Equal(0))
	})
})
