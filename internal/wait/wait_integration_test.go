//go:build integration

// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package wait_test

import (
	"context"
	"log/slog"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opdev/virtwork/internal/resources"
	"github.com/opdev/virtwork/internal/testutil"
	"github.com/opdev/virtwork/internal/vm"
	"github.com/opdev/virtwork/internal/wait"
)

var _ = Describe("WaitForVMReady [integration]", Label("slow"), func() {
	var (
		ctx       context.Context
		c         client.Client
		namespace string
	)

	BeforeEach(func() {
		ctx = context.Background()
		c = testutil.MustConnect("")
		namespace = testutil.UniqueNamespace("wait-ready")
		Expect(resources.EnsureNamespace(ctx, c, namespace, testutil.ManagedLabels())).To(Succeed())
	})

	AfterEach(func() {
		testutil.CleanupNamespace(ctx, c, namespace)
	})

	It("should return nil once a VM reaches Running phase", func() {
		opts := testutil.DefaultVMOpts("wait-vm-0", namespace)
		vmObj, err := vm.BuildVMSpec(opts)
		Expect(err).NotTo(HaveOccurred())
		Expect(vm.CreateVM(ctx, c, vmObj)).To(Succeed())

		err := wait.WaitForVMReady(ctx, c, slog.Default(), "wait-vm-0", namespace, 5*time.Minute, 5*time.Second)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should return error on timeout for nonexistent VM", func() {
		err := wait.WaitForVMReady(ctx, c, slog.Default(), "nonexistent-vm", namespace, 10*time.Second, 2*time.Second)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("WaitForAllVMsReady [integration]", Label("slow"), func() {
	var (
		ctx       context.Context
		c         client.Client
		namespace string
	)

	BeforeEach(func() {
		ctx = context.Background()
		c = testutil.MustConnect("")
		namespace = testutil.UniqueNamespace("wait-all")
		Expect(resources.EnsureNamespace(ctx, c, namespace, testutil.ManagedLabels())).To(Succeed())
	})

	AfterEach(func() {
		testutil.CleanupNamespace(ctx, c, namespace)
	})

	It("should wait for multiple VMs concurrently", func() {
		for _, name := range []string{"wait-multi-0", "wait-multi-1"} {
			opts := testutil.DefaultVMOpts(name, namespace)
			vmObj, err := vm.BuildVMSpec(opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(vm.CreateVM(ctx, c, vmObj)).To(Succeed())
		}

		results := wait.WaitForAllVMsReady(ctx, c, slog.Default(),
			[]string{"wait-multi-0", "wait-multi-1"},
			namespace, 5*time.Minute, 5*time.Second)

		for name, err := range results {
			Expect(err).NotTo(HaveOccurred(), "VM %s failed to become ready", name)
		}
	})

	It("should report per-VM errors for nonexistent VMs", func() {
		results := wait.WaitForAllVMsReady(ctx, c, slog.Default(),
			[]string{"nonexistent-0", "nonexistent-1"},
			namespace, 10*time.Second, 2*time.Second)

		for _, err := range results {
			Expect(err).To(HaveOccurred())
		}
	})
})
