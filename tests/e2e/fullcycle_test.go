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

var _ = Describe("Full deployment cycle", Label("slow"), func() {
	var namespace string

	BeforeEach(func() {
		namespace = testutil.UniqueNamespace("e2e-cycle")
	})

	AfterEach(func() {
		_, stderr, exitCode, err := testutil.RunVirtwork("cleanup",
			"--namespace", namespace, "--delete-namespace", "--yes")
		if err != nil || exitCode != 0 {
			GinkgoWriter.Printf("AfterEach cleanup failed (ns=%s exit=%d): err=%v stderr=%s\n",
				namespace, exitCode, err, stderr)
		}
	})

	It("should deploy CPU workload, wait for readiness, and clean up", func() {
		// Step 1: Deploy
		stdout, _, exitCode, err := testutil.RunVirtwork(
			"run", "--workloads", "cpu", "--vm-count", "1",
			"--namespace", namespace, "--timeout", "300")
		Expect(err).NotTo(HaveOccurred())
		Expect(exitCode).To(Equal(0))
		Expect(stdout).To(ContainSubstring("ready"))

		// Step 2: Verify resources exist on cluster
		c := testutil.MustConnect("")
		ctx := context.Background()
		vms, err := vm.ListVMs(ctx, c, namespace, testutil.ManagedLabels())
		Expect(err).NotTo(HaveOccurred())
		Expect(vms).To(HaveLen(1))
		Expect(vms[0].Name).To(Equal("virtwork-cpu-0"))

		// Step 3: Cleanup
		stdout, _, exitCode, err = testutil.RunVirtwork(
			"cleanup", "--namespace", namespace, "--yes")
		Expect(err).NotTo(HaveOccurred())
		Expect(exitCode).To(Equal(0))
		Expect(stdout).To(ContainSubstring(`"vms_deleted":1`))

		// Step 4: Verify resources are gone (KubeVirt finalizers may delay removal)
		Eventually(func() int {
			vms, err = vm.ListVMs(ctx, c, namespace, testutil.ManagedLabels())
			Expect(err).NotTo(HaveOccurred())
			return len(vms)
		}, 60*time.Second, 2*time.Second).Should(Equal(0))
	})

	It("should deploy network workload and clean up all resources", func() {
		// Deploy
		_, _, exitCode, err := testutil.RunVirtwork(
			"run", "--workloads", "network", "--vm-count", "1",
			"--namespace", namespace, "--no-wait")
		Expect(err).NotTo(HaveOccurred())
		Expect(exitCode).To(Equal(0))

		// Verify service exists
		c := testutil.MustConnect("")
		ctx := context.Background()

		svc := &corev1.Service{}
		err = c.Get(ctx, client.ObjectKey{
			Name: "virtwork-iperf3-server", Namespace: namespace,
		}, svc)
		Expect(err).NotTo(HaveOccurred())

		// Verify 2 VMs exist (server + client)
		vms, err := vm.ListVMs(ctx, c, namespace, testutil.ManagedLabels())
		Expect(err).NotTo(HaveOccurred())
		Expect(vms).To(HaveLen(2))

		// Cleanup
		stdout, _, exitCode, err := testutil.RunVirtwork(
			"cleanup", "--namespace", namespace, "--yes")
		Expect(err).NotTo(HaveOccurred())
		Expect(exitCode).To(Equal(0))
		Expect(stdout).To(ContainSubstring(`"vms_deleted":2`))
	})
})
