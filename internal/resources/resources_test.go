// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package resources_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/opdev/virtwork/internal/cluster"
	"github.com/opdev/virtwork/internal/resources"
)

var _ = Describe("EnsureNamespace", func() {
	var (
		ctx    context.Context
		scheme = cluster.NewScheme()
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("should create namespace", func() {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()

		err := resources.EnsureNamespace(ctx, c, "test-ns", map[string]string{
			"app.kubernetes.io/managed-by": "virtwork",
		})
		Expect(err).NotTo(HaveOccurred())

		// Verify the namespace was created
		ns := &corev1.Namespace{}
		err = c.Get(ctx, client.ObjectKey{Name: "test-ns"}, ns)
		Expect(err).NotTo(HaveOccurred())
		Expect(ns.Name).To(Equal("test-ns"))
	})

	It("should skip on AlreadyExists", func() {
		existing := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "existing-ns",
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()

		err := resources.EnsureNamespace(ctx, c, "existing-ns", map[string]string{
			"app.kubernetes.io/managed-by": "virtwork",
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("should apply labels", func() {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()

		labels := map[string]string{
			"app.kubernetes.io/managed-by": "virtwork",
			"custom-label":                 "custom-value",
		}
		err := resources.EnsureNamespace(ctx, c, "labeled-ns", labels)
		Expect(err).NotTo(HaveOccurred())

		ns := &corev1.Namespace{}
		err = c.Get(ctx, client.ObjectKey{Name: "labeled-ns"}, ns)
		Expect(err).NotTo(HaveOccurred())
		Expect(ns.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "virtwork"))
		Expect(ns.Labels).To(HaveKeyWithValue("custom-label", "custom-value"))
	})

	It("should return error on non-AlreadyExists failure", func() {
		c := fake.NewClientBuilder().
			WithScheme(scheme).
			WithInterceptorFuncs(interceptor.Funcs{
				Create: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					return apierrors.NewForbidden(
						schema.GroupResource{Group: "", Resource: "namespaces"},
						"test-ns",
						nil,
					)
				},
			}).
			Build()

		err := resources.EnsureNamespace(ctx, c, "test-ns", nil)
		Expect(err).To(HaveOccurred())
		Expect(apierrors.IsForbidden(err)).To(BeTrue())
	})
})

var _ = Describe("CreateService", func() {
	var (
		ctx    context.Context
		scheme = cluster.NewScheme()
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	newTestService := func(name, namespace string) *corev1.Service {
		return &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "virtwork",
				},
			},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{
					"virtwork/role": "server",
				},
				Ports: []corev1.ServicePort{
					{
						Name:       "iperf3",
						Port:       5201,
						TargetPort: intstr.FromInt(5201),
						Protocol:   corev1.ProtocolTCP,
					},
				},
			},
		}
	}

	It("should create service", func() {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		svc := newTestService("test-svc", "default")

		err := resources.CreateService(ctx, c, svc)
		Expect(err).NotTo(HaveOccurred())

		got := &corev1.Service{}
		err = c.Get(ctx, client.ObjectKey{Name: "test-svc", Namespace: "default"}, got)
		Expect(err).NotTo(HaveOccurred())
		Expect(got.Name).To(Equal("test-svc"))
	})

	It("should skip on AlreadyExists", func() {
		existing := newTestService("existing-svc", "default")
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()

		dup := newTestService("existing-svc", "default")
		err := resources.CreateService(ctx, c, dup)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should return error on non-AlreadyExists failure", func() {
		c := fake.NewClientBuilder().
			WithScheme(scheme).
			WithInterceptorFuncs(interceptor.Funcs{
				Create: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					return apierrors.NewUnauthorized("unauthorized")
				},
			}).
			Build()

		svc := newTestService("test-svc", "default")
		err := resources.CreateService(ctx, c, svc)
		Expect(err).To(HaveOccurred())
		Expect(apierrors.IsUnauthorized(err)).To(BeTrue())
	})
})

var _ = Describe("CreateCloudInitSecret", func() {
	var (
		ctx    context.Context
		scheme = cluster.NewScheme()
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("should create secret with userdata", func() {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()

		err := resources.CreateCloudInitSecret(ctx, c, "test-cloudinit", "default",
			"#cloud-config\npackages:\n  - vim\n",
			map[string]string{"app.kubernetes.io/managed-by": "virtwork"})
		Expect(err).NotTo(HaveOccurred())

		got := &corev1.Secret{}
		err = c.Get(ctx, client.ObjectKey{Name: "test-cloudinit", Namespace: "default"}, got)
		Expect(err).NotTo(HaveOccurred())
		// The fake client stores StringData as-is (real API server converts to Data)
		Expect(got.StringData["userdata"]).To(ContainSubstring("#cloud-config"))
	})

	It("should skip on AlreadyExists", func() {
		existing := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "existing-secret",
				Namespace: "default",
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()

		err := resources.CreateCloudInitSecret(ctx, c, "existing-secret", "default",
			"#cloud-config\n", nil)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should apply labels", func() {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		labels := map[string]string{
			"app.kubernetes.io/managed-by": "virtwork",
			"app.kubernetes.io/component":  "database",
		}

		err := resources.CreateCloudInitSecret(ctx, c, "labeled-secret", "default",
			"#cloud-config\n", labels)
		Expect(err).NotTo(HaveOccurred())

		got := &corev1.Secret{}
		err = c.Get(ctx, client.ObjectKey{Name: "labeled-secret", Namespace: "default"}, got)
		Expect(err).NotTo(HaveOccurred())
		Expect(got.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "virtwork"))
		Expect(got.Labels).To(HaveKeyWithValue("app.kubernetes.io/component", "database"))
	})

	It("should return error on non-AlreadyExists failure", func() {
		c := fake.NewClientBuilder().
			WithScheme(scheme).
			WithInterceptorFuncs(interceptor.Funcs{
				Create: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					return apierrors.NewForbidden(
						schema.GroupResource{Group: "", Resource: "secrets"},
						"test-secret",
						nil,
					)
				},
			}).
			Build()

		err := resources.CreateCloudInitSecret(ctx, c, "test-secret", "default",
			"#cloud-config\n", nil)
		Expect(err).To(HaveOccurred())
		Expect(apierrors.IsForbidden(err)).To(BeTrue())
	})
})

// nolint:dupl
var _ = Describe("DeleteManagedSecrets", func() {
	var (
		ctx    context.Context
		scheme = cluster.NewScheme()
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("should return delete count", func() {
		sec1 := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sec-1",
				Namespace: "default",
				Labels:    map[string]string{"app.kubernetes.io/managed-by": "virtwork"},
			},
		}
		sec2 := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sec-2",
				Namespace: "default",
				Labels:    map[string]string{"app.kubernetes.io/managed-by": "virtwork"},
			},
		}
		sec3 := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sec-other",
				Namespace: "default",
				Labels:    map[string]string{"app.kubernetes.io/managed-by": "other"},
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sec1, sec2, sec3).Build()

		count, err := resources.DeleteManagedSecrets(ctx, c, "default", map[string]string{
			"app.kubernetes.io/managed-by": "virtwork",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(2))

		secList := &corev1.SecretList{}
		err = c.List(ctx, secList, client.InNamespace("default"))
		Expect(err).NotTo(HaveOccurred())
		Expect(secList.Items).To(HaveLen(1))
		Expect(secList.Items[0].Name).To(Equal("sec-other"))
	})

	It("should return zero count when no secrets match", func() {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()

		count, err := resources.DeleteManagedSecrets(ctx, c, "default", map[string]string{
			"app.kubernetes.io/managed-by": "virtwork",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(0))
	})

	It("should return error on list failure", func() {
		c := fake.NewClientBuilder().
			WithScheme(scheme).
			WithInterceptorFuncs(interceptor.Funcs{
				List: func(ctx context.Context, cl client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
					return apierrors.NewForbidden(
						schema.GroupResource{Group: "", Resource: "secrets"},
						"",
						nil,
					)
				},
			}).
			Build()

		_, err := resources.DeleteManagedSecrets(ctx, c, "default", map[string]string{
			"app.kubernetes.io/managed-by": "virtwork",
		})
		Expect(err).To(HaveOccurred())
	})
})

// nolint:dupl
var _ = Describe("DeleteManagedServices", func() {
	var (
		ctx    context.Context
		scheme = cluster.NewScheme()
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("should return delete count", func() {
		svc1 := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "svc-1",
				Namespace: "default",
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "virtwork",
				},
			},
		}
		svc2 := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "svc-2",
				Namespace: "default",
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "virtwork",
				},
			},
		}
		svc3 := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "svc-other",
				Namespace: "default",
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "other",
				},
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc1, svc2, svc3).Build()

		count, err := resources.DeleteManagedServices(ctx, c, "default", map[string]string{
			"app.kubernetes.io/managed-by": "virtwork",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(2))

		// Verify managed services are gone
		svcList := &corev1.ServiceList{}
		err = c.List(ctx, svcList, client.InNamespace("default"))
		Expect(err).NotTo(HaveOccurred())
		Expect(svcList.Items).To(HaveLen(1))
		Expect(svcList.Items[0].Name).To(Equal("svc-other"))
	})

	It("should return zero count when no services match", func() {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()

		count, err := resources.DeleteManagedServices(ctx, c, "default", map[string]string{
			"app.kubernetes.io/managed-by": "virtwork",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(0))
	})

	It("should return error on list failure", func() {
		c := fake.NewClientBuilder().
			WithScheme(scheme).
			WithInterceptorFuncs(interceptor.Funcs{
				List: func(ctx context.Context, cl client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
					return apierrors.NewForbidden(
						schema.GroupResource{Group: "", Resource: "services"},
						"",
						nil,
					)
				},
			}).
			Build()

		_, err := resources.DeleteManagedServices(ctx, c, "default", map[string]string{
			"app.kubernetes.io/managed-by": "virtwork",
		})
		Expect(err).To(HaveOccurred())
	})
})
