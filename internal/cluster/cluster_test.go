// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package cluster_test

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubevirtv1 "kubevirt.io/api/core/v1"
	cdiv1beta1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	"github.com/opdev/virtwork/internal/cluster"
)

var _ = Describe("NewScheme", func() {
	var scheme *runtime.Scheme

	BeforeEach(func() {
		scheme = cluster.NewScheme()
	})

	It("should register core types", func() {
		obj := &corev1.Namespace{}
		gvks, _, err := scheme.ObjectKinds(obj)
		Expect(err).NotTo(HaveOccurred())
		Expect(gvks).NotTo(BeEmpty())
	})

	It("should register KubeVirt types", func() {
		obj := &kubevirtv1.VirtualMachine{}
		gvks, _, err := scheme.ObjectKinds(obj)
		Expect(err).NotTo(HaveOccurred())
		Expect(gvks).NotTo(BeEmpty())
	})

	It("should register CDI types", func() {
		obj := &cdiv1beta1.DataVolume{}
		gvks, _, err := scheme.ObjectKinds(obj)
		Expect(err).NotTo(HaveOccurred())
		Expect(gvks).NotTo(BeEmpty())
	})
})

var _ = Describe("ResolveKubeconfigPath", func() {
	var origKubeconfig string
	var hadKubeconfig bool

	BeforeEach(func() {
		origKubeconfig, hadKubeconfig = os.LookupEnv("KUBECONFIG")
		_ = os.Unsetenv("KUBECONFIG")
	})

	AfterEach(func() {
		if hadKubeconfig {
			_ = os.Setenv("KUBECONFIG", origKubeconfig)
		} else {
			_ = os.Unsetenv("KUBECONFIG")
		}
	})

	It("should return explicit path when provided", func() {
		_ = os.Setenv("KUBECONFIG", "/from-env")
		result := cluster.ResolveKubeconfigPath("/explicit/path")
		Expect(result).To(Equal("/explicit/path"))
	})

	It("should fall back to KUBECONFIG env when explicit path is empty", func() {
		_ = os.Setenv("KUBECONFIG", "/from-kubeconfig-env")
		result := cluster.ResolveKubeconfigPath("")
		Expect(result).To(Equal("/from-kubeconfig-env"))
	})

	It("should return empty when no explicit path and no KUBECONFIG", func() {
		result := cluster.ResolveKubeconfigPath("")
		Expect(result).To(BeEmpty())
	})
})

var _ = Describe("Connect", func() {
	// Helper to disable in-cluster detection for tests running outside a pod
	disableInCluster := func() func() {
		origHost, hadHost := os.LookupEnv("KUBERNETES_SERVICE_HOST")
		_ = os.Unsetenv("KUBERNETES_SERVICE_HOST")
		return func() {
			if hadHost {
				_ = os.Setenv("KUBERNETES_SERVICE_HOST", origHost)
			} else {
				_ = os.Unsetenv("KUBERNETES_SERVICE_HOST")
			}
		}
	}

	writeFakeKubeconfig := func(contextName string) string {
		tmpFile, err := os.CreateTemp("", "kubeconfig-*.yaml")
		Expect(err).NotTo(HaveOccurred())
		_, err = tmpFile.WriteString(`apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1:99999
  name: test
contexts:
- context:
    cluster: test
    user: test
  name: ` + contextName + `
current-context: ` + contextName + `
users:
- name: test
  user:
    token: fake-token
`)
		Expect(err).NotTo(HaveOccurred())
		_ = tmpFile.Close()
		return tmpFile.Name()
	}

	It("should return error when both in-cluster and kubeconfig fail", func() {
		restore := disableInCluster()
		defer restore()

		_, _, err := cluster.Connect("/nonexistent/kubeconfig/path")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("kubeconfig"))
	})

	It("should use explicit kubeconfig path when provided", func() {
		restore := disableInCluster()
		defer restore()

		path := writeFakeKubeconfig("explicit-ctx")
		defer func() { _ = os.Remove(path) }()

		c, contextName, err := cluster.Connect(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(c).NotTo(BeNil())
		Expect(contextName).To(Equal("explicit-ctx"))
	})

	It("should skip in-cluster when explicit path is provided", func() {
		path := writeFakeKubeconfig("explicit-over-incluster")
		defer func() { _ = os.Remove(path) }()

		c, contextName, err := cluster.Connect(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(c).NotTo(BeNil())
		Expect(contextName).To(Equal("explicit-over-incluster"))
	})

	It("should fall back to default rules when path is empty and not in-cluster", func() {
		restore := disableInCluster()
		defer restore()

		origKubeconfig, had := os.LookupEnv("KUBECONFIG")
		defer func() {
			if had {
				_ = os.Setenv("KUBECONFIG", origKubeconfig)
			} else {
				_ = os.Unsetenv("KUBECONFIG")
			}
		}()

		path := writeFakeKubeconfig("default-rules-ctx")
		defer func() { _ = os.Remove(path) }()
		_ = os.Setenv("KUBECONFIG", path)

		// ResolveKubeconfigPath("") would pick up KUBECONFIG, but
		// Connect("") should use default loading rules which also
		// check KUBECONFIG. Verify it works with an empty path.
		c, contextName, err := cluster.Connect("")
		Expect(err).NotTo(HaveOccurred())
		Expect(c).NotTo(BeNil())
		Expect(contextName).To(Equal("default-rules-ctx"))
	})
})
