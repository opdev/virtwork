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

var _ = Describe("Connect", func() {
	It("should return error when both in-cluster and kubeconfig fail", func() {
		// Ensure we're not running in-cluster by unsetting the env var
		origHost := os.Getenv("KUBERNETES_SERVICE_HOST")
		os.Unsetenv("KUBERNETES_SERVICE_HOST")
		defer func() {
			if origHost != "" {
				os.Setenv("KUBERNETES_SERVICE_HOST", origHost)
			}
		}()

		_, err := cluster.Connect("/nonexistent/kubeconfig/path")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("kubeconfig"))
	})

	It("should use kubeconfig path when provided", func() {
		// Create a minimal but invalid kubeconfig to confirm the path is used
		tmpFile, err := os.CreateTemp("", "kubeconfig-*.yaml")
		Expect(err).NotTo(HaveOccurred())
		defer os.Remove(tmpFile.Name())

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
  name: test
current-context: test
users:
- name: test
  user:
    token: fake-token
`)
		Expect(err).NotTo(HaveOccurred())
		tmpFile.Close()

		// Ensure we're not running in-cluster
		origHost := os.Getenv("KUBERNETES_SERVICE_HOST")
		os.Unsetenv("KUBERNETES_SERVICE_HOST")
		defer func() {
			if origHost != "" {
				os.Setenv("KUBERNETES_SERVICE_HOST", origHost)
			}
		}()

		// Connect should succeed (client creation doesn't dial the server)
		c, err := cluster.Connect(tmpFile.Name())
		Expect(err).NotTo(HaveOccurred())
		Expect(c).NotTo(BeNil())
	})
})
