//go:build integration

// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package vm_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opdev/virtwork/internal/constants"
	"github.com/opdev/virtwork/internal/testutil"
	"github.com/opdev/virtwork/internal/vm"
)

var _ = Describe("CreateVM [integration]", func() {
	var (
		ctx       context.Context
		c         client.Client
		namespace string
	)

	BeforeEach(func() {
		ctx = context.Background()
		c = testutil.MustConnect("")
		namespace = testutil.UniqueNamespace("vm-create")
		Expect(testutil.EnsureTestNamespace(ctx, c, namespace)).To(Succeed())
	})

	AfterEach(func() {
		testutil.CleanupNamespace(ctx, c, namespace)
	})

	It("should create a VirtualMachine on the cluster", func() {
		opts := testutil.DefaultVMOpts("integ-vm-0", namespace)
		vmObj, err := vm.BuildVMSpec(opts)
		Expect(err).NotTo(HaveOccurred())

		err = vm.CreateVM(ctx, c, vmObj)
		Expect(err).NotTo(HaveOccurred())

		// Verify the VM exists
		vms, err := vm.ListVMs(ctx, c, namespace, testutil.ManagedLabels())
		Expect(err).NotTo(HaveOccurred())
		Expect(vms).To(HaveLen(1))
		Expect(vms[0].Name).To(Equal("integ-vm-0"))
	})

	It("should be idempotent on repeated calls", func() {
		opts := testutil.DefaultVMOpts("integ-vm-idem", namespace)
		vmObj, err := vm.BuildVMSpec(opts)
		Expect(err).NotTo(HaveOccurred())

		Expect(vm.CreateVM(ctx, c, vmObj)).To(Succeed())
		Expect(vm.CreateVM(ctx, c, vmObj)).To(Succeed())
	})

	It("should set the correct labels on the created VM", func() {
		opts := testutil.DefaultVMOpts("integ-vm-labels", namespace)
		vmObj, err := vm.BuildVMSpec(opts)
		Expect(err).NotTo(HaveOccurred())

		Expect(vm.CreateVM(ctx, c, vmObj)).To(Succeed())

		vms, err := vm.ListVMs(ctx, c, namespace, testutil.ManagedLabels())
		Expect(err).NotTo(HaveOccurred())
		Expect(vms).To(HaveLen(1))
		Expect(vms[0].Labels).To(HaveKeyWithValue(constants.LabelManagedBy, constants.ManagedByValue))
	})
})

var _ = Describe("DeleteVM [integration]", func() {
	var (
		ctx       context.Context
		c         client.Client
		namespace string
	)

	BeforeEach(func() {
		ctx = context.Background()
		c = testutil.MustConnect("")
		namespace = testutil.UniqueNamespace("vm-delete")
		Expect(testutil.EnsureTestNamespace(ctx, c, namespace)).To(Succeed())
	})

	AfterEach(func() {
		testutil.CleanupNamespace(ctx, c, namespace)
	})

	It("should delete an existing VM", func() {
		opts := testutil.DefaultVMOpts("integ-vm-del", namespace)
		vmObj, err := vm.BuildVMSpec(opts)
		Expect(err).NotTo(HaveOccurred())
		Expect(vm.CreateVM(ctx, c, vmObj)).To(Succeed())

		err := vm.DeleteVM(ctx, c, "integ-vm-del", namespace)
		Expect(err).NotTo(HaveOccurred())

		// KubeVirt finalizers keep the VM in Terminating state briefly.
		Eventually(func() int {
			vms, err := vm.ListVMs(ctx, c, namespace, testutil.ManagedLabels())
			Expect(err).NotTo(HaveOccurred())
			return len(vms)
		}, 60*time.Second, 2*time.Second).Should(Equal(0))
	})

	It("should be idempotent for nonexistent VMs", func() {
		err := vm.DeleteVM(ctx, c, "nonexistent-vm", namespace)
		Expect(err).NotTo(HaveOccurred())
	})
})

var _ = Describe("ListVMs [integration]", func() {
	var (
		ctx       context.Context
		c         client.Client
		namespace string
	)

	BeforeEach(func() {
		ctx = context.Background()
		c = testutil.MustConnect("")
		namespace = testutil.UniqueNamespace("vm-list")
		Expect(testutil.EnsureTestNamespace(ctx, c, namespace)).To(Succeed())
	})

	AfterEach(func() {
		testutil.CleanupNamespace(ctx, c, namespace)
	})

	It("should list VMs by label selector", func() {
		for i := 0; i < 3; i++ {
			opts := testutil.DefaultVMOpts("integ-vm-list-"+string(rune('0'+i)), namespace)
			vmObj, err := vm.BuildVMSpec(opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(vm.CreateVM(ctx, c, vmObj)).To(Succeed())
		}

		vms, err := vm.ListVMs(ctx, c, namespace, testutil.ManagedLabels())
		Expect(err).NotTo(HaveOccurred())
		Expect(vms).To(HaveLen(3))
	})

	It("should return empty list for namespace with no VMs", func() {
		vms, err := vm.ListVMs(ctx, c, namespace, testutil.ManagedLabels())
		Expect(err).NotTo(HaveOccurred())
		Expect(vms).To(BeEmpty())
	})
})

var _ = Describe("GetVMIPhase [integration]", Label("slow"), func() {
	var (
		ctx       context.Context
		c         client.Client
		namespace string
	)

	BeforeEach(func() {
		ctx = context.Background()
		c = testutil.MustConnect("")
		namespace = testutil.UniqueNamespace("vm-phase")
		Expect(testutil.EnsureTestNamespace(ctx, c, namespace)).To(Succeed())
	})

	AfterEach(func() {
		testutil.CleanupNamespace(ctx, c, namespace)
	})

	It("should return error for nonexistent VMI", func() {
		_, err := vm.GetVMIPhase(ctx, c, "nonexistent", namespace)
		Expect(err).To(HaveOccurred())
	})
})
