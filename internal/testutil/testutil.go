// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

// Package testutil provides shared helpers for integration and E2E tests.
// This package has no build tags — it is a pure library imported only by
// tagged test files.
package testutil

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opdev/virtwork/internal/cleanup"
	"github.com/opdev/virtwork/internal/cluster"
	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/constants"
	"github.com/opdev/virtwork/internal/vm"
	"github.com/opdev/virtwork/internal/wait"
)

// UniqueNamespace returns a unique namespace name with the format
// "virtwork-test-<prefix>-<random>" to avoid collisions between parallel
// test runs or repeated test executions.
//
// The prefix parameter should be a short, descriptive identifier for the
// test suite or feature being tested (e.g., "vm", "cleanup", "workload").
//
// Example:
//
//	namespace := testutil.UniqueNamespace("vm-create")
//	// Returns: "virtwork-test-vm-create-a3f8b2c1"
func UniqueNamespace(prefix string) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("virtwork-test-%s-%s", prefix, hex.EncodeToString(b))
}

// MustConnect connects to the cluster using the given kubeconfig path.
// It panics on failure, making it suitable for test setup functions like
// Ginkgo's BeforeEach where connection failure should abort the suite.
//
// Kubeconfig resolution order:
//  1. kubeconfigPath parameter (if non-empty)
//  2. KUBECONFIG environment variable
//  3. Default kubeconfig (~/.kube/config)
//  4. In-cluster config (when running inside a pod)
//
// Example:
//
//	var c client.Client
//	BeforeEach(func() {
//	    c = testutil.MustConnect("") // Uses KUBECONFIG or default
//	})
//
// Panics with a descriptive message if connection fails.
func MustConnect(kubeconfigPath string) client.Client {
	if kubeconfigPath == "" {
		kubeconfigPath = os.Getenv("KUBECONFIG")
	}
	c, _, err := cluster.Connect(kubeconfigPath)
	if err != nil {
		panic(fmt.Sprintf("testutil.MustConnect: %v", err))
	}
	return c
}

// ManagedLabels returns the standard virtwork managed-by labels used for
// resource tracking and cleanup. These labels are applied to all resources
// created by tests to enable label-based cleanup via CleanupNamespace.
//
// Returns a map with app.kubernetes.io/managed-by: virtwork.
func ManagedLabels() map[string]string {
	return map[string]string{
		constants.LabelManagedBy: constants.ManagedByValue,
	}
}

// CleanupNamespace deletes all virtwork-managed resources in the namespace,
// then deletes the namespace itself. It is error-tolerant and suitable for
// use in test cleanup functions like Ginkgo's AfterEach or DeferCleanup.
//
// This function:
//   - Deletes VirtualMachines, Services, Secrets with managed-by labels
//   - Deletes the namespace
//   - Logs errors but does not panic or return them
//
// Example:
//
//	AfterEach(func() {
//	    testutil.CleanupNamespace(ctx, c, namespace)
//	})
//
// KubeVirt finalizers may delay namespace deletion by up to 60 seconds.
func CleanupNamespace(ctx context.Context, c client.Client, namespace string) {
	_, _ = cleanup.CleanupAll(ctx, c, &config.Config{Namespace: namespace}, true, "")
}

// DefaultVMOpts returns a minimal VMSpecOpts suitable for integration tests.
// The returned options use conservative resource settings that work on most
// test clusters.
//
// Default configuration:
//   - CPU: 1 core
//   - Memory: 512Mi
//   - Disk: Fedora containerDisk (constants.DefaultContainerDiskImage)
//   - Cloud-init: Empty cloud-config (no packages or setup)
//   - Labels: virtwork managed-by labels for cleanup tracking
//
// The resulting VM can be built with vm.BuildVMSpec() and created with
// vm.CreateVM(). Modify the returned options as needed for specific tests.
func DefaultVMOpts(name, namespace string) vm.VMSpecOpts {
	return vm.VMSpecOpts{
		Name:               name,
		Namespace:          namespace,
		ContainerDiskImage: constants.DefaultContainerDiskImage,
		CloudInitUserdata:  "#cloud-config\n",
		CPUCores:           1,
		Memory:             "512Mi",
		Labels: map[string]string{
			constants.LabelManagedBy: constants.ManagedByValue,
			constants.LabelAppName:   "virtwork",
			constants.LabelComponent: "test",
		},
	}
}

// EnsureTestNamespace creates a namespace with virtwork managed-by labels
// for use in integration tests. It is idempotent — if the namespace already
// exists, it returns nil (success) without error.
//
// The namespace is labeled with app.kubernetes.io/managed-by: virtwork for
// cleanup tracking. Use UniqueNamespace() to generate collision-proof names.
//
// Example:
//
//	namespace := testutil.UniqueNamespace("my-test")
//	err := testutil.EnsureTestNamespace(ctx, c, namespace)
//	Expect(err).NotTo(HaveOccurred())
//
// Returns an error only if creation fails for reasons other than AlreadyExists.
func EnsureTestNamespace(ctx context.Context, c client.Client, namespace string) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   namespace,
			Labels: ManagedLabels(),
		},
	}
	err := c.Create(ctx, ns)
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

// WaitForVMRunning polls until the VirtualMachineInstance reaches Running
// phase or the timeout expires. It uses 5-second polling intervals appropriate
// for test environments.
//
// The name parameter should match the VirtualMachine name, as KubeVirt creates
// a VirtualMachineInstance with the same name when the VM is started.
//
// Recommended timeout values:
//   - 2-3 minutes for containerDisk-based VMs on fast clusters
//   - 5 minutes for containerDisk-based VMs on resource-constrained clusters
//   - 10+ minutes for VMs with DataVolumes (PVC provisioning + import)
//
// Example:
//
//	err := testutil.WaitForVMRunning(ctx, c, "test-vm", namespace, 5*time.Minute)
//	Expect(err).NotTo(HaveOccurred())
//
// Returns an error if the timeout expires or if the VMI cannot be found.
func WaitForVMRunning(ctx context.Context, c client.Client, name, namespace string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	interval := 5 * time.Second

	for {
		phase, err := vm.GetVMIPhase(ctx, c, name, namespace)
		if err == nil && phase == "Running" {
			return nil
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			if err != nil {
				return fmt.Errorf("timeout waiting for VM %s to be running: %w", name, err)
			}
			return fmt.Errorf("waiting for VM %s to be running (phase: %s); %w", name, phase, wait.ErrVMTimeout)
		}
		if remaining < interval {
			interval = remaining
		}
		time.Sleep(interval)
	}
}
