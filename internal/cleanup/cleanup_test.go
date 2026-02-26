// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package cleanup_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/opdev/virtwork/internal/cleanup"
	"github.com/opdev/virtwork/internal/cluster"
	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/constants"
	"github.com/opdev/virtwork/internal/vm"
)

var _ = Describe("CleanupAll", func() {
	var (
		ctx       context.Context
		scheme    = cluster.NewScheme()
		namespace = "test-ns"
		labels    = map[string]string{
			constants.LabelManagedBy: constants.ManagedByValue,
		}
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	newManagedVM := func(name string) *kubevirtv1.VirtualMachine {
		return vm.BuildVMSpec(vm.VMSpecOpts{
			Name:               name,
			Namespace:          namespace,
			ContainerDiskImage: "test-image",
			CloudInitUserdata:  "#cloud-config\n",
			CPUCores:           1,
			Memory:             "1Gi",
			Labels:             labels,
		})
	}

	newManagedSecret := func(name string) *corev1.Secret {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels:    labels,
			},
			StringData: map[string]string{
				"userdata": "#cloud-config\n",
			},
		}
	}

	newManagedService := func(name string) *corev1.Service {
		return &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels:    labels,
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{Port: 80, Protocol: corev1.ProtocolTCP},
				},
			},
		}
	}

	It("should delete VMs by managed-by label", func() {
		vm1 := newManagedVM("vm-1")
		vm2 := newManagedVM("vm-2")
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm1, vm2).Build()

		result, err := cleanup.CleanupAll(ctx, c, &config.Config{Namespace: namespace}, false, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.VMsDeleted).To(Equal(2))
		Expect(result.Errors).To(BeEmpty())

		// Verify VMs are gone
		vmList := &kubevirtv1.VirtualMachineList{}
		Expect(c.List(ctx, vmList, client.InNamespace(namespace))).To(Succeed())
		Expect(vmList.Items).To(BeEmpty())
	})

	// nolint:dupl
	It("should tolerate individual VM deletion errors", func() {
		vm1 := newManagedVM("vm-1")
		vm2 := newManagedVM("vm-2")
		callCount := 0
		c := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(vm1, vm2).
			WithInterceptorFuncs(interceptor.Funcs{
				Delete: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
					if _, ok := obj.(*kubevirtv1.VirtualMachine); ok {
						callCount++
						if callCount == 1 {
							return fmt.Errorf("simulated delete error")
						}
					}
					return cl.Delete(ctx, obj, opts...)
				},
			}).
			Build()

		result, err := cleanup.CleanupAll(ctx, c, &config.Config{Namespace: namespace}, false, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.VMsDeleted).To(Equal(1))
		Expect(result.Errors).To(HaveLen(1))
	})

	It("should delete services by managed-by label", func() {
		svc1 := newManagedService("svc-1")
		svc2 := newManagedService("svc-2")
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc1, svc2).Build()

		result, err := cleanup.CleanupAll(ctx, c, &config.Config{Namespace: namespace}, false, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.ServicesDeleted).To(Equal(2))
		Expect(result.Errors).To(BeEmpty())

		// Verify services are gone
		svcList := &corev1.ServiceList{}
		Expect(c.List(ctx, svcList, client.InNamespace(namespace))).To(Succeed())
		Expect(svcList.Items).To(BeEmpty())
	})

	// nolint:dupl
	It("should tolerate individual service deletion errors", func() {
		svc1 := newManagedService("svc-1")
		svc2 := newManagedService("svc-2")
		callCount := 0
		c := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(svc1, svc2).
			WithInterceptorFuncs(interceptor.Funcs{
				Delete: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
					if _, ok := obj.(*corev1.Service); ok {
						callCount++
						if callCount == 1 {
							return fmt.Errorf("simulated service delete error")
						}
					}
					return cl.Delete(ctx, obj, opts...)
				},
			}).
			Build()

		result, err := cleanup.CleanupAll(ctx, c, &config.Config{Namespace: namespace}, false, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.ServicesDeleted).To(Equal(1))
		Expect(result.Errors).To(HaveLen(1))
	})

	It("should delete secrets by managed-by label", func() {
		sec1 := newManagedSecret("sec-1")
		sec2 := newManagedSecret("sec-2")
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sec1, sec2).Build()

		result, err := cleanup.CleanupAll(ctx, c, &config.Config{Namespace: namespace}, false, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.SecretsDeleted).To(Equal(2))
		Expect(result.Errors).To(BeEmpty())

		secretList := &corev1.SecretList{}
		Expect(c.List(ctx, secretList, client.InNamespace(namespace))).To(Succeed())
		Expect(secretList.Items).To(BeEmpty())
	})

	// nolint:dupl
	It("should tolerate individual secret deletion errors", func() {
		sec1 := newManagedSecret("sec-1")
		sec2 := newManagedSecret("sec-2")
		callCount := 0
		c := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(sec1, sec2).
			WithInterceptorFuncs(interceptor.Funcs{
				Delete: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
					if _, ok := obj.(*corev1.Secret); ok {
						callCount++
						if callCount == 1 {
							return fmt.Errorf("simulated secret delete error")
						}
					}
					return cl.Delete(ctx, obj, opts...)
				},
			}).
			Build()

		result, err := cleanup.CleanupAll(ctx, c, &config.Config{Namespace: namespace}, false, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.SecretsDeleted).To(Equal(1))
		Expect(result.Errors).To(HaveLen(1))
	})

	It("should not delete namespace by default", func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   namespace,
				Labels: labels,
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()

		result, err := cleanup.CleanupAll(ctx, c, &config.Config{Namespace: namespace}, false, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.NamespaceDeleted).To(BeFalse())

		// Verify namespace still exists
		got := &corev1.Namespace{}
		Expect(c.Get(ctx, client.ObjectKey{Name: namespace}, got)).To(Succeed())
	})

	It("should delete namespace when flagged", func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   namespace,
				Labels: labels,
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()

		result, err := cleanup.CleanupAll(ctx, c, &config.Config{Namespace: namespace}, true, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.NamespaceDeleted).To(BeTrue())
	})

	It("should tolerate namespace deletion error", func() {
		c := fake.NewClientBuilder().
			WithScheme(scheme).
			WithInterceptorFuncs(interceptor.Funcs{
				Delete: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
					if _, ok := obj.(*corev1.Namespace); ok {
						return fmt.Errorf("simulated namespace delete error")
					}
					return cl.Delete(ctx, obj, opts...)
				},
			}).
			Build()

		result, err := cleanup.CleanupAll(ctx, c, &config.Config{Namespace: namespace}, true, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.NamespaceDeleted).To(BeFalse())
		Expect(result.Errors).To(HaveLen(1))
	})

	It("should report accurate counts for successful deletions", func() {
		vm1 := newManagedVM("vm-1")
		vm2 := newManagedVM("vm-2")
		vm3 := newManagedVM("vm-3")
		svc1 := newManagedService("svc-1")
		sec1 := newManagedSecret("sec-1")
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm1, vm2, vm3, svc1, sec1).Build()

		result, err := cleanup.CleanupAll(ctx, c, &config.Config{Namespace: namespace}, false, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.VMsDeleted).To(Equal(3))
		Expect(result.ServicesDeleted).To(Equal(1))
		Expect(result.SecretsDeleted).To(Equal(1))
		Expect(result.NamespaceDeleted).To(BeFalse())
		Expect(result.Errors).To(BeEmpty())
	})

	It("should handle empty namespace gracefully", func() {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()

		result, err := cleanup.CleanupAll(ctx, c, &config.Config{Namespace: namespace}, false, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.VMsDeleted).To(Equal(0))
		Expect(result.ServicesDeleted).To(Equal(0))
		Expect(result.SecretsDeleted).To(Equal(0))
		Expect(result.NamespaceDeleted).To(BeFalse())
		Expect(result.Errors).To(BeEmpty())
	})

	It("should use correct managed-by=virtwork label selector", func() {
		// Create a managed VM and an unmanaged VM
		managedVM := newManagedVM("managed-vm")
		unmanagedVM := vm.BuildVMSpec(vm.VMSpecOpts{
			Name:               "unmanaged-vm",
			Namespace:          namespace,
			ContainerDiskImage: "test-image",
			CloudInitUserdata:  "#cloud-config\n",
			CPUCores:           1,
			Memory:             "1Gi",
			Labels: map[string]string{
				constants.LabelManagedBy: "other-tool",
			},
		})
		// Create a managed service and an unmanaged service
		managedSvc := newManagedService("managed-svc")
		unmanagedSvc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "unmanaged-svc",
				Namespace: namespace,
				Labels: map[string]string{
					constants.LabelManagedBy: "other-tool",
				},
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{Port: 80, Protocol: corev1.ProtocolTCP},
				},
			},
		}
		c := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(managedVM, unmanagedVM, managedSvc, unmanagedSvc).
			Build()

		result, err := cleanup.CleanupAll(ctx, c, &config.Config{Namespace: namespace}, false, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.VMsDeleted).To(Equal(1))
		Expect(result.ServicesDeleted).To(Equal(1))

		// Verify unmanaged resources still exist
		vmList := &kubevirtv1.VirtualMachineList{}
		Expect(c.List(ctx, vmList, client.InNamespace(namespace))).To(Succeed())
		Expect(vmList.Items).To(HaveLen(1))
		Expect(vmList.Items[0].Name).To(Equal("unmanaged-vm"))

		svcList := &corev1.ServiceList{}
		Expect(c.List(ctx, svcList, client.InNamespace(namespace))).To(Succeed())
		Expect(svcList.Items).To(HaveLen(1))
		Expect(svcList.Items[0].Name).To(Equal("unmanaged-svc"))
	})
})
