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
)

// UniqueNamespace returns a namespace name like "virtwork-test-<prefix>-<random>"
// to avoid collisions between parallel test runs.
func UniqueNamespace(prefix string) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("virtwork-test-%s-%s", prefix, hex.EncodeToString(b))
}

// MustConnect connects to the cluster using the given kubeconfig path.
// If kubeconfigPath is empty, it checks the KUBECONFIG environment variable
// before falling back to default kubeconfig resolution.
// Panics on failure — suitable for test setup where connection failure
// should abort the suite.
func MustConnect(kubeconfigPath string) client.Client {
	if kubeconfigPath == "" {
		kubeconfigPath = os.Getenv("KUBECONFIG")
	}
	c, err := cluster.Connect(kubeconfigPath)
	if err != nil {
		panic(fmt.Sprintf("testutil.MustConnect: %v", err))
	}
	return c
}

// ManagedLabels returns the standard virtwork managed-by labels used for
// resource tracking and cleanup.
func ManagedLabels() map[string]string {
	return map[string]string{
		constants.LabelManagedBy: constants.ManagedByValue,
	}
}

// CleanupNamespace deletes all virtwork-managed resources in the namespace,
// then deletes the namespace itself. Errors are logged but do not cause panic.
// Suitable for use with Ginkgo's DeferCleanup.
func CleanupNamespace(ctx context.Context, c client.Client, namespace string) {
	_, _ = cleanup.CleanupAll(ctx, c, &config.Config{Namespace: namespace}, true, "")
}

// DefaultVMOpts returns a minimal VMSpecOpts suitable for integration tests.
// Uses 1 CPU, 512Mi memory, the default Fedora containerDisk, and a simple
// cloud-init that does nothing.
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
// for use in integration tests.
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

// WaitForVMRunning polls until the VMI reaches Running phase or the timeout
// expires. Uses short intervals appropriate for test environments.
func WaitForVMRunning(ctx context.Context, c client.Client, name, namespace string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	interval := 5 * time.Second

	for {
		phase, err := vm.GetVMIPhase(ctx, c, name, namespace)
		if err == nil && phase == "Running" {
			return nil
		}
		if time.Now().After(deadline) {
			if err != nil {
				return fmt.Errorf("timeout waiting for VM %s to be running: %w", name, err)
			}
			return fmt.Errorf("timeout waiting for VM %s to be running (phase: %s)", name, phase)
		}
		time.Sleep(interval)
	}
}
