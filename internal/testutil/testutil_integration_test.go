// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

//go:build integration

package testutil_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opdev/virtwork/internal/testutil"
)

var _ = Describe("MustConnect", func() {
	Context("when connecting to a cluster", func() {
		It("should successfully connect with valid kubeconfig", func() {
			c := testutil.MustConnect("")
			Expect(c).NotTo(BeNil())
		})

		It("should use KUBECONFIG environment variable when path is empty", func() {
			c := testutil.MustConnect("")
			Expect(c).NotTo(BeNil())

			list := &corev1.NamespaceList{}
			err := c.List(context.Background(), list)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("when given invalid kubeconfig path", func() {
		It("should panic on connection failure", func() {
			Expect(func() {
				testutil.MustConnect("/invalid/path/to/kubeconfig")
			}).To(Panic())
		})
	})
})

var _ = Describe("EnsureTestNamespace", func() {
	var (
		ctx       context.Context
		c         client.Client
		namespace string
	)

	BeforeEach(func() {
		ctx = context.Background()
		c = testutil.MustConnect("")
		namespace = testutil.UniqueNamespace("ensure-test")
	})

	AfterEach(func() {
		if namespace != "" {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			_ = c.Delete(ctx, ns)
		}
	})

	Context("when creating a test namespace", func() {
		It("should successfully create a namespace", func() {
			err := testutil.EnsureTestNamespace(ctx, c, namespace)
			Expect(err).NotTo(HaveOccurred())

			ns := &corev1.Namespace{}
			err = c.Get(ctx, client.ObjectKey{Name: namespace}, ns)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should add managed-by labels to the namespace", func() {
			err := testutil.EnsureTestNamespace(ctx, c, namespace)
			Expect(err).NotTo(HaveOccurred())

			ns := &corev1.Namespace{}
			err = c.Get(ctx, client.ObjectKey{Name: namespace}, ns)
			Expect(err).NotTo(HaveOccurred())

			labels := testutil.ManagedLabels()
			for key, value := range labels {
				Expect(ns.Labels).To(HaveKeyWithValue(key, value))
			}
		})

		It("should not error if namespace already exists", func() {
			err := testutil.EnsureTestNamespace(ctx, c, namespace)
			Expect(err).NotTo(HaveOccurred())

			err = testutil.EnsureTestNamespace(ctx, c, namespace)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

var _ = Describe("CleanupNamespace", func() {
	var (
		ctx       context.Context
		c         client.Client
		namespace string
	)

	BeforeEach(func() {
		ctx = context.Background()
		c = testutil.MustConnect("")
		namespace = testutil.UniqueNamespace("cleanup-test")
		err := testutil.EnsureTestNamespace(ctx, c, namespace)
		Expect(err).NotTo(HaveOccurred())
	})

	Context("when cleaning up a namespace", func() {
		It("should not panic on cleanup", func() {
			Expect(func() {
				testutil.CleanupNamespace(ctx, c, namespace)
			}).NotTo(Panic())
		})

		It("should handle cleanup of non-existent namespace gracefully", func() {
			nonexistent := "virtwork-test-nonexistent-12345678"
			Expect(func() {
				testutil.CleanupNamespace(ctx, c, nonexistent)
			}).NotTo(Panic())
		})
	})
})

var _ = Describe("WaitForVMRunning", func() {
	var (
		ctx       context.Context
		c         client.Client
		namespace string
	)

	BeforeEach(func() {
		ctx = context.Background()
		c = testutil.MustConnect("")
		namespace = testutil.UniqueNamespace("wait-test")
		err := testutil.EnsureTestNamespace(ctx, c, namespace)
		Expect(err).NotTo(HaveOccurred())

		// Warm the KubeVirt API discovery cache so timing assertions
		// aren't skewed by first-call REST mapping overhead.
		_ = testutil.WaitForVMRunning(ctx, c, "warmup", namespace, 1*time.Second)
	})

	AfterEach(func() {
		if namespace != "" {
			testutil.CleanupNamespace(ctx, c, namespace)
		}
	})

	Context("when waiting for a non-existent VM", func() {
		It("should timeout and return an error", func() {
			err := testutil.WaitForVMRunning(ctx, c, "nonexistent-vm", namespace, 5*time.Second)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("timeout"))
		})
	})

	Context("when waiting with a very short timeout", func() {
		It("should return error quickly", func() {
			start := time.Now()
			err := testutil.WaitForVMRunning(ctx, c, "test-vm", namespace, 1*time.Second)
			elapsed := time.Since(start)

			Expect(err).To(HaveOccurred())
			Expect(elapsed).To(BeNumerically(">=", 1*time.Second))
			Expect(elapsed).To(BeNumerically("<", 5*time.Second))
		})
	})
})
