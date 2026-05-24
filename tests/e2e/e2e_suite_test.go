//go:build e2e

// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"context"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	"github.com/opdev/virtwork/internal/testutil"
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2E Suite")
}

var _ = BeforeSuite(func() {
	// Ensure the binary is built before any test runs.
	// BinaryPath() builds on first call and caches the result.
	path, err := testutil.BinaryPath()
	Expect(err).NotTo(HaveOccurred(), "Failed to build virtwork binary")
	GinkgoWriter.Printf("Using virtwork binary: %s\n", path)
})

var _ = AfterSuite(func() {
	c := testutil.MustConnect("")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var nsList corev1.NamespaceList
	if err := c.List(ctx, &nsList); err != nil {
		GinkgoWriter.Printf("AfterSuite: failed to list namespaces: %v\n", err)
		return
	}

	var stale []string
	for i := range nsList.Items {
		name := nsList.Items[i].Name
		if strings.HasPrefix(name, "virtwork-test-") && nsList.Items[i].DeletionTimestamp == nil {
			stale = append(stale, name)
		}
	}

	if len(stale) == 0 {
		GinkgoWriter.Println("AfterSuite: no stale virtwork-test-* namespaces found")
		return
	}

	GinkgoWriter.Printf("AfterSuite: sweeping %d stale namespace(s)\n", len(stale))
	for _, name := range stale {
		ns := &corev1.Namespace{}
		ns.Name = name
		if err := c.Delete(ctx, ns); err != nil {
			GinkgoWriter.Printf("AfterSuite: failed to delete namespace %s: %v\n", name, err)
		} else {
			GinkgoWriter.Printf("AfterSuite: deleted namespace %s\n", name)
		}
	}
})
