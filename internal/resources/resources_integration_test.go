//go:build integration

// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package resources_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opdev/virtwork/internal/constants"
	"github.com/opdev/virtwork/internal/resources"
	"github.com/opdev/virtwork/internal/testutil"
)

var _ = Describe("EnsureNamespace [integration]", func() {
	var (
		ctx       context.Context
		c         client.Client
		namespace string
	)

	BeforeEach(func() {
		ctx = context.Background()
		c = testutil.MustConnect("")
		namespace = testutil.UniqueNamespace("res-ns")
	})

	AfterEach(func() {
		testutil.CleanupNamespace(ctx, c, namespace)
	})

	It("should create a real namespace on the cluster", func() {
		err := resources.EnsureNamespace(ctx, c, namespace, testutil.ManagedLabels())
		Expect(err).NotTo(HaveOccurred())

		ns := &corev1.Namespace{}
		err = c.Get(ctx, client.ObjectKey{Name: namespace}, ns)
		Expect(err).NotTo(HaveOccurred())
		Expect(ns.Name).To(Equal(namespace))
	})

	It("should be idempotent on repeated calls", func() {
		err := resources.EnsureNamespace(ctx, c, namespace, testutil.ManagedLabels())
		Expect(err).NotTo(HaveOccurred())

		err = resources.EnsureNamespace(ctx, c, namespace, testutil.ManagedLabels())
		Expect(err).NotTo(HaveOccurred())
	})

	It("should apply managed-by labels to the namespace", func() {
		labels := testutil.ManagedLabels()
		err := resources.EnsureNamespace(ctx, c, namespace, labels)
		Expect(err).NotTo(HaveOccurred())

		ns := &corev1.Namespace{}
		err = c.Get(ctx, client.ObjectKey{Name: namespace}, ns)
		Expect(err).NotTo(HaveOccurred())
		Expect(ns.Labels).To(HaveKeyWithValue(constants.LabelManagedBy, constants.ManagedByValue))
	})
})

var _ = Describe("CreateService [integration]", func() {
	var (
		ctx       context.Context
		c         client.Client
		namespace string
	)

	BeforeEach(func() {
		ctx = context.Background()
		c = testutil.MustConnect("")
		namespace = testutil.UniqueNamespace("res-svc")
		Expect(resources.EnsureNamespace(ctx, c, namespace, testutil.ManagedLabels())).To(Succeed())
	})

	AfterEach(func() {
		testutil.CleanupNamespace(ctx, c, namespace)
	})

	It("should create a real service", func() {
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service",
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
		err := resources.CreateService(ctx, c, svc)
		Expect(err).NotTo(HaveOccurred())

		retrieved := &corev1.Service{}
		err = c.Get(ctx, client.ObjectKey{Name: "test-service", Namespace: namespace}, retrieved)
		Expect(err).NotTo(HaveOccurred())
		Expect(retrieved.Spec.Ports).To(HaveLen(1))
	})

	It("should be idempotent", func() {
		makeSvc := func() *corev1.Service {
			return &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc-idem",
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
		}
		Expect(resources.CreateService(ctx, c, makeSvc())).To(Succeed())
		Expect(resources.CreateService(ctx, c, makeSvc())).To(Succeed())
	})
})

var _ = Describe("CreateCloudInitSecret [integration]", func() {
	var (
		ctx       context.Context
		c         client.Client
		namespace string
	)

	BeforeEach(func() {
		ctx = context.Background()
		c = testutil.MustConnect("")
		namespace = testutil.UniqueNamespace("res-sec")
		Expect(resources.EnsureNamespace(ctx, c, namespace, testutil.ManagedLabels())).To(Succeed())
	})

	AfterEach(func() {
		testutil.CleanupNamespace(ctx, c, namespace)
	})

	It("should create a secret with userdata field", func() {
		err := resources.CreateCloudInitSecret(
			ctx,
			c,
			"test-cloudinit",
			namespace,
			"#cloud-config\n",
			testutil.ManagedLabels(),
		)
		Expect(err).NotTo(HaveOccurred())

		secret := &corev1.Secret{}
		err = c.Get(ctx, client.ObjectKey{Name: "test-cloudinit", Namespace: namespace}, secret)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(secret.Data["userdata"])).To(Equal("#cloud-config\n"))
	})

	It("should be idempotent", func() {
		Expect(
			resources.CreateCloudInitSecret(
				ctx,
				c,
				"test-idem",
				namespace,
				"#cloud-config\n",
				testutil.ManagedLabels(),
			),
		).To(Succeed())
		Expect(
			resources.CreateCloudInitSecret(
				ctx,
				c,
				"test-idem",
				namespace,
				"#cloud-config\n",
				testutil.ManagedLabels(),
			),
		).To(Succeed())
	})

	It("should apply managed-by labels", func() {
		Expect(
			resources.CreateCloudInitSecret(
				ctx,
				c,
				"test-labels",
				namespace,
				"#cloud-config\n",
				testutil.ManagedLabels(),
			),
		).To(Succeed())

		secret := &corev1.Secret{}
		Expect(c.Get(ctx, client.ObjectKey{Name: "test-labels", Namespace: namespace}, secret)).To(Succeed())
		Expect(secret.Labels).To(HaveKeyWithValue(constants.LabelManagedBy, constants.ManagedByValue))
	})
})

var _ = Describe("DeleteManagedSecrets [integration]", func() {
	var (
		ctx       context.Context
		c         client.Client
		namespace string
	)

	BeforeEach(func() {
		ctx = context.Background()
		c = testutil.MustConnect("")
		namespace = testutil.UniqueNamespace("res-del")
		Expect(resources.EnsureNamespace(ctx, c, namespace, testutil.ManagedLabels())).To(Succeed())
	})

	AfterEach(func() {
		testutil.CleanupNamespace(ctx, c, namespace)
	})

	It("should delete only labeled secrets", func() {
		// Create a managed secret
		Expect(
			resources.CreateCloudInitSecret(
				ctx,
				c,
				"managed-secret",
				namespace,
				"#cloud-config\n",
				testutil.ManagedLabels(),
			),
		).To(Succeed())

		// Create an unmanaged secret
		unmanaged := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "unmanaged-secret",
				Namespace: namespace,
			},
			StringData: map[string]string{"key": "value"},
		}
		Expect(c.Create(ctx, unmanaged)).To(Succeed())

		count, err := resources.DeleteManagedSecrets(ctx, c, namespace, testutil.ManagedLabels())
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(1))

		// Unmanaged secret should still exist
		retrieved := &corev1.Secret{}
		Expect(c.Get(ctx, client.ObjectKey{Name: "unmanaged-secret", Namespace: namespace}, retrieved)).To(Succeed())
	})
})

var _ = Describe("DeleteManagedServices [integration]", func() {
	var (
		ctx       context.Context
		c         client.Client
		namespace string
	)

	BeforeEach(func() {
		ctx = context.Background()
		c = testutil.MustConnect("")
		namespace = testutil.UniqueNamespace("res-delsvc")
		Expect(resources.EnsureNamespace(ctx, c, namespace, testutil.ManagedLabels())).To(Succeed())
	})

	AfterEach(func() {
		testutil.CleanupNamespace(ctx, c, namespace)
	})

	It("should delete only labeled services and return count", func() {
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "managed-svc",
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

		count, err := resources.DeleteManagedServices(ctx, c, namespace, testutil.ManagedLabels())
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(1))
	})
})
