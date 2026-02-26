// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package wait_test

import (
	"context"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/opdev/virtwork/internal/cluster"
	"github.com/opdev/virtwork/internal/wait"
)

var _ = Describe("WaitForVMReady", func() {
	var (
		ctx    context.Context
		scheme = cluster.NewScheme()
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("should return nil when immediately ready", func() {
		vmi := &kubevirtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ready-vm",
				Namespace: "default",
			},
			Status: kubevirtv1.VirtualMachineInstanceStatus{
				Phase: kubevirtv1.Running,
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vmi).Build()

		err := wait.WaitForVMReady(ctx, c, "ready-vm", "default", 5*time.Second, 10*time.Millisecond)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should return nil after N polls", func() {
		vmi := &kubevirtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "eventual-vm",
				Namespace: "default",
			},
			Status: kubevirtv1.VirtualMachineInstanceStatus{
				Phase: kubevirtv1.Scheduling,
			},
		}

		var callCount int32
		c := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(vmi).
			WithInterceptorFuncs(interceptor.Funcs{
				Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					err := cl.Get(ctx, key, obj, opts...)
					if err != nil {
						return err
					}
					count := atomic.AddInt32(&callCount, 1)
					if vmiObj, ok := obj.(*kubevirtv1.VirtualMachineInstance); ok {
						if count >= 3 {
							vmiObj.Status.Phase = kubevirtv1.Running
						}
					}
					return nil
				},
			}).
			Build()

		err := wait.WaitForVMReady(ctx, c, "eventual-vm", "default", 5*time.Second, 10*time.Millisecond)
		Expect(err).NotTo(HaveOccurred())
		Expect(atomic.LoadInt32(&callCount)).To(BeNumerically(">=", int32(3)))
	})

	It("should return error on timeout", func() {
		vmi := &kubevirtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "stuck-vm",
				Namespace: "default",
			},
			Status: kubevirtv1.VirtualMachineInstanceStatus{
				Phase: kubevirtv1.Scheduling,
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vmi).Build()

		err := wait.WaitForVMReady(ctx, c, "stuck-vm", "default", 50*time.Millisecond, 10*time.Millisecond)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("timed out"))
	})

	It("should retry when VMI not found and eventually timeout", func() {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()

		err := wait.WaitForVMReady(ctx, c, "nonexistent", "default", 50*time.Millisecond, 10*time.Millisecond)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("timed out"))
	})

	It("should succeed when VMI appears after initial not-found", func() {
		var callCount int32
		c := fake.NewClientBuilder().
			WithScheme(scheme).
			WithInterceptorFuncs(interceptor.Funcs{
				Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					count := atomic.AddInt32(&callCount, 1)
					if count <= 2 {
						return cl.Get(ctx, key, obj, opts...) // not found from empty store
					}
					// Simulate VMI appearing on 3rd call
					if vmiObj, ok := obj.(*kubevirtv1.VirtualMachineInstance); ok {
						vmiObj.Name = key.Name
						vmiObj.Namespace = key.Namespace
						vmiObj.Status.Phase = kubevirtv1.Running
					}
					return nil
				},
			}).
			Build()

		err := wait.WaitForVMReady(ctx, c, "delayed-vm", "default", 5*time.Second, 10*time.Millisecond)
		Expect(err).NotTo(HaveOccurred())
		Expect(atomic.LoadInt32(&callCount)).To(BeNumerically(">=", int32(3)))
	})

	It("should respect context cancellation", func() {
		vmi := &kubevirtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cancel-vm",
				Namespace: "default",
			},
			Status: kubevirtv1.VirtualMachineInstanceStatus{
				Phase: kubevirtv1.Scheduling,
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vmi).Build()

		cancelCtx, cancel := context.WithCancel(ctx)
		cancel() // Cancel immediately

		err := wait.WaitForVMReady(cancelCtx, c, "cancel-vm", "default", 5*time.Second, 10*time.Millisecond)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("WaitForAllVMsReady", func() {
	var (
		ctx    context.Context
		scheme = cluster.NewScheme()
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("should poll all VMs concurrently", func() {
		vmi1 := &kubevirtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vm-1",
				Namespace: "default",
			},
			Status: kubevirtv1.VirtualMachineInstanceStatus{
				Phase: kubevirtv1.Running,
			},
		}
		vmi2 := &kubevirtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vm-2",
				Namespace: "default",
			},
			Status: kubevirtv1.VirtualMachineInstanceStatus{
				Phase: kubevirtv1.Running,
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vmi1, vmi2).Build()

		results := wait.WaitForAllVMsReady(
			ctx,
			c,
			[]string{"vm-1", "vm-2"},
			"default",
			5*time.Second,
			10*time.Millisecond,
		)
		Expect(results).To(HaveLen(2))
		Expect(results["vm-1"]).NotTo(HaveOccurred())
		Expect(results["vm-2"]).NotTo(HaveOccurred())
	})

	It("should report partial failures", func() {
		vmi1 := &kubevirtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "good-vm",
				Namespace: "default",
			},
			Status: kubevirtv1.VirtualMachineInstanceStatus{
				Phase: kubevirtv1.Running,
			},
		}
		// bad-vm does not exist, so it will fail
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vmi1).Build()

		results := wait.WaitForAllVMsReady(
			ctx,
			c,
			[]string{"good-vm", "bad-vm"},
			"default",
			50*time.Millisecond,
			10*time.Millisecond,
		)
		Expect(results).To(HaveLen(2))
		Expect(results["good-vm"]).NotTo(HaveOccurred())
		Expect(results["bad-vm"]).To(HaveOccurred())
	})

	It("should handle empty names list", func() {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()

		results := wait.WaitForAllVMsReady(ctx, c, []string{}, "default", 5*time.Second, 10*time.Millisecond)
		Expect(results).To(BeEmpty())
	})
})
