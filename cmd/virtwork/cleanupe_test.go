// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/opdev/virtwork/internal/cluster"
	"github.com/opdev/virtwork/internal/constants"
)

func fakeConnectEmpty(_ string) (client.Client, string, error) {
	scheme := cluster.NewScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	return c, "fake-context", nil
}

func fakeConnectWithResources(_ string) (client.Client, string, error) {
	scheme := cluster.NewScheme()
	vm := &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "virtwork-cpu-0",
			Namespace: constants.DefaultNamespace,
			Labels: map[string]string{
				constants.LabelManagedBy: constants.ManagedByValue,
				constants.LabelRunID:     "test-run-001",
			},
		},
	}
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(vm).
		Build()
	return c, "fake-context", nil
}

var _ = Describe("cleanupE", func() {
	Context("error paths", func() {
		It("returns error for nonexistent config file", func() {
			rootCmd := newRootCmd()
			rootCmd.SetArgs([]string{"cleanup", "--config", "/nonexistent/config.yaml"})
			err := rootCmd.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("config"))
		})

		It("returns error when initAuditor fails", func() {
			rootCmd := newRootCmd()
			rootCmd.SetArgs([]string{
				"cleanup",
				"--audit-db", "/nonexistent/deeply/nested/path/db.sqlite",
			})
			err := rootCmd.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("auditor"))
		})

		It("returns error for malformed kubeconfig", func() {
			tmpDir := GinkgoT().TempDir()
			badKubeconfig := filepath.Join(tmpDir, "kubeconfig")
			Expect(os.WriteFile(badKubeconfig, []byte(`not valid yaml{{{`), 0o600)).To(Succeed())

			rootCmd := newRootCmd()
			rootCmd.SetArgs([]string{
				"cleanup",
				"--kubeconfig", badKubeconfig,
				"--no-audit",
			})
			err := rootCmd.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("connecting to cluster"))
		})
	})

	Context("with empty fake client", func() {
		var origConnect func(string) (client.Client, string, error)

		BeforeEach(func() {
			origConnect = clusterConnect
			clusterConnect = fakeConnectEmpty
		})

		AfterEach(func() {
			clusterConnect = origConnect
		})

		It("reports nothing to clean up when no managed resources exist", func() {
			rootCmd := newRootCmd()
			var buf bytes.Buffer
			rootCmd.SetOut(&buf)
			rootCmd.SetArgs([]string{"cleanup", "--no-audit", "--yes"})
			Expect(rootCmd.Execute()).To(Succeed())

			output := buf.String()
			Expect(output).To(ContainSubstring("nothing to clean up"))
		})

		It("reports nothing in dry-run mode", func() {
			rootCmd := newRootCmd()
			var buf bytes.Buffer
			rootCmd.SetOut(&buf)
			rootCmd.SetArgs([]string{"cleanup", "--dry-run", "--no-audit"})
			Expect(rootCmd.Execute()).To(Succeed())

			output := buf.String()
			Expect(output).To(ContainSubstring("nothing to clean up"))
		})

		It("reports nothing with --run-id filter", func() {
			rootCmd := newRootCmd()
			var buf bytes.Buffer
			rootCmd.SetOut(&buf)
			rootCmd.SetArgs([]string{
				"cleanup", "--run-id", "nonexistent-run", "--no-audit",
			})
			Expect(rootCmd.Execute()).To(Succeed())

			output := buf.String()
			Expect(output).To(ContainSubstring("nothing to clean up"))
		})

		It("covers all three cleanup mode branches", func() {
			for _, tc := range []struct {
				args []string
			}{
				{[]string{"cleanup", "--no-audit", "--yes"}},
				{[]string{"cleanup", "--dry-run", "--no-audit"}},
				{[]string{"cleanup", "--run-id", "x", "--no-audit"}},
			} {
				rootCmd := newRootCmd()
				var buf bytes.Buffer
				rootCmd.SetOut(&buf)
				rootCmd.SetArgs(tc.args)
				Expect(rootCmd.Execute()).To(Succeed())
			}
		})
	})

	Context("with pre-populated fake client", func() {
		var origConnect func(string) (client.Client, string, error)

		BeforeEach(func() {
			origConnect = clusterConnect
			clusterConnect = fakeConnectWithResources
		})

		AfterEach(func() {
			clusterConnect = origConnect
		})

		It("shows preview and skips deletion in dry-run mode", func() {
			rootCmd := newRootCmd()
			var buf bytes.Buffer
			rootCmd.SetOut(&buf)
			rootCmd.SetArgs([]string{"cleanup", "--dry-run", "--no-audit"})
			Expect(rootCmd.Execute()).To(Succeed())

			output := buf.String()
			Expect(output).To(ContainSubstring("dry-run mode"))
		})

		It("deletes resources when --yes is set", func() {
			rootCmd := newRootCmd()
			var buf bytes.Buffer
			rootCmd.SetOut(&buf)
			rootCmd.SetArgs([]string{"cleanup", "--yes", "--no-audit"})
			Expect(rootCmd.Execute()).To(Succeed())

			output := buf.String()
			Expect(output).To(ContainSubstring("cleanup complete"))
		})

		It("deletes resources when --run-id matches (skips prompt)", func() {
			rootCmd := newRootCmd()
			var buf bytes.Buffer
			rootCmd.SetOut(&buf)
			rootCmd.SetArgs([]string{
				"cleanup", "--run-id", "test-run-001", "--no-audit",
			})
			Expect(rootCmd.Execute()).To(Succeed())

			output := buf.String()
			Expect(output).To(ContainSubstring("cleanup complete"))
		})

		It("aborts when user declines confirmation prompt", func() {
			rootCmd := newRootCmd()
			var buf bytes.Buffer
			rootCmd.SetOut(&buf)
			rootCmd.SetIn(strings.NewReader("no\n"))
			rootCmd.SetArgs([]string{"cleanup", "--no-audit"})
			Expect(rootCmd.Execute()).To(Succeed())

			output := buf.String()
			Expect(output).To(ContainSubstring("cleanup aborted"))
		})

		It("proceeds when user confirms at prompt", func() {
			rootCmd := newRootCmd()
			var buf bytes.Buffer
			rootCmd.SetOut(&buf)
			rootCmd.SetIn(strings.NewReader("yes\n"))
			rootCmd.SetArgs([]string{"cleanup", "--no-audit"})
			Expect(rootCmd.Execute()).To(Succeed())

			output := buf.String()
			Expect(output).To(ContainSubstring("cleanup complete"))
		})

		It("deletes resources with --delete-namespace", func() {
			rootCmd := newRootCmd()
			var buf bytes.Buffer
			rootCmd.SetOut(&buf)
			rootCmd.SetArgs([]string{
				"cleanup", "--yes", "--delete-namespace", "--no-audit",
			})
			Expect(rootCmd.Execute()).To(Succeed())

			output := buf.String()
			Expect(output).To(ContainSubstring("cleanup complete"))
		})
	})
})
