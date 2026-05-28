//go:build integration

// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package cleanup_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opdev/virtwork/internal/cleanup"
	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/resources"
	"github.com/opdev/virtwork/internal/testutil"
	"github.com/opdev/virtwork/internal/vm"
)

var _ = Describe("CleanupAll [integration]", func() {
	var (
		ctx       context.Context
		c         client.Client
		namespace string
	)

	BeforeEach(func() {
		ctx = context.Background()
		c = testutil.MustConnect("")
		namespace = testutil.UniqueNamespace("cleanup")
		Expect(resources.EnsureNamespace(ctx, c, namespace, testutil.ManagedLabels())).To(Succeed())
	})

	AfterEach(func() {
		// Final safety net — always try to remove the namespace
		testutil.CleanupNamespace(ctx, c, namespace)
	})

	It("should delete VMs by managed-by label", func() {
		opts := testutil.DefaultVMOpts("cleanup-vm-0", namespace)
		vmObj, err := vm.BuildVMSpec(opts)
		Expect(err).NotTo(HaveOccurred())
		Expect(vm.CreateVM(ctx, c, vmObj)).To(Succeed())

		result, err := cleanup.CleanupAll(ctx, c, &config.Config{Namespace: namespace}, false, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.VMsDeleted).To(Equal(1))
	})

	It("should delete services by managed-by label", func() {
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cleanup-svc",
				Namespace: namespace,
				Labels:    testutil.ManagedLabels(),
			},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "test"},
				Ports: []corev1.ServicePort{
					{Name: "http", Port: 80, Protocol: corev1.ProtocolTCP},
				},
			},
		}
		Expect(resources.CreateService(ctx, c, svc)).To(Succeed())

		result, err := cleanup.CleanupAll(ctx, c, &config.Config{Namespace: namespace}, false, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.ServicesDeleted).To(Equal(1))
	})

	It("should delete secrets by managed-by label", func() {
		Expect(
			resources.CreateCloudInitSecret(
				ctx,
				c,
				"cleanup-secret",
				namespace,
				"#cloud-config\n",
				testutil.ManagedLabels(),
			),
		).To(Succeed())

		result, err := cleanup.CleanupAll(ctx, c, &config.Config{Namespace: namespace}, false, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.SecretsDeleted).To(Equal(1))
	})

	It("should not delete resources without the managed-by label", func() {
		// Create an unmanaged secret
		unmanaged := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "unmanaged",
				Namespace: namespace,
			},
			StringData: map[string]string{"key": "value"},
		}
		Expect(c.Create(ctx, unmanaged)).To(Succeed())

		result, err := cleanup.CleanupAll(ctx, c, &config.Config{Namespace: namespace}, false, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.SecretsDeleted).To(Equal(0))

		// Unmanaged secret should still exist
		retrieved := &corev1.Secret{}
		Expect(c.Get(ctx, client.ObjectKey{Name: "unmanaged", Namespace: namespace}, retrieved)).To(Succeed())
	})

	It("should delete the namespace when flagged", func() {
		result, err := cleanup.CleanupAll(ctx, c, &config.Config{Namespace: namespace}, true, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.NamespaceDeleted).To(BeTrue())
	})

	It("should not delete the namespace when not flagged", func() {
		result, err := cleanup.CleanupAll(ctx, c, &config.Config{Namespace: namespace}, false, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.NamespaceDeleted).To(BeFalse())

		// Namespace should still exist
		ns := &corev1.Namespace{}
		Expect(c.Get(ctx, client.ObjectKey{Name: namespace}, ns)).To(Succeed())
	})

	It("should report accurate counts for mixed resources", func() {
		// Create a VM, a service, and a secret
		opts := testutil.DefaultVMOpts("cleanup-mix-vm", namespace)
		vmObj, err := vm.BuildVMSpec(opts)
		Expect(err).NotTo(HaveOccurred())
		Expect(vm.CreateVM(ctx, c, vmObj)).To(Succeed())

		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cleanup-mix-svc",
				Namespace: namespace,
				Labels:    testutil.ManagedLabels(),
			},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "test"},
				Ports: []corev1.ServicePort{
					{Name: "http", Port: 80, Protocol: corev1.ProtocolTCP},
				},
			},
		}
		Expect(resources.CreateService(ctx, c, svc)).To(Succeed())

		Expect(
			resources.CreateCloudInitSecret(
				ctx,
				c,
				"cleanup-mix-secret",
				namespace,
				"#cloud-config\n",
				testutil.ManagedLabels(),
			),
		).To(Succeed())

		result, err := cleanup.CleanupAll(ctx, c, &config.Config{Namespace: namespace}, false, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.VMsDeleted).To(Equal(1))
		Expect(result.ServicesDeleted).To(Equal(1))
		Expect(result.SecretsDeleted).To(Equal(1))
		Expect(result.Errors).To(BeEmpty())
	})

	It("should handle empty namespace gracefully", func() {
		result, err := cleanup.CleanupAll(ctx, c, &config.Config{Namespace: namespace}, false, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.VMsDeleted).To(Equal(0))
		Expect(result.ServicesDeleted).To(Equal(0))
		Expect(result.SecretsDeleted).To(Equal(0))
		Expect(result.Errors).To(BeEmpty())
	})

	It("should be idempotent when run twice", func() {
		opts := testutil.DefaultVMOpts("cleanup-idem-vm", namespace)
		vmObj, err := vm.BuildVMSpec(opts)
		Expect(err).NotTo(HaveOccurred())
		Expect(vm.CreateVM(ctx, c, vmObj)).To(Succeed())

		result1, err := cleanup.CleanupAll(ctx, c, &config.Config{Namespace: namespace}, false, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(result1.VMsDeleted).To(Equal(1))

		// Wait (120s) for the VM to be fully removed (KubeVirt finalizers cause
		// the object to linger in Terminating state).
		Eventually(func() int {
			vms, _ := vm.ListVMs(ctx, c, namespace, testutil.ManagedLabels())
			return len(vms)
		}, 120*time.Second, 2*time.Second).Should(Equal(0))

		result2, err := cleanup.CleanupAll(ctx, c, &config.Config{Namespace: namespace}, false, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(result2.VMsDeleted).To(Equal(0))
	})
})
