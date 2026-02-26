// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package wait

import (
	"context"
	"fmt"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// WaitForVMReady polls the VMI phase until it reaches Running or the timeout
// expires. It uses time.Sleep for polling intervals and respects context
// cancellation.
func WaitForVMReady(
	ctx context.Context,
	c client.Client,
	name, namespace string,
	timeout, interval time.Duration,
) error {
	deadline := time.Now().Add(timeout)

	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("context cancelled waiting for VM %s/%s: %w", namespace, name, err)
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for VM %s/%s to become ready", namespace, name)
		}

		vmi := &kubevirtv1.VirtualMachineInstance{}
		key := client.ObjectKey{Name: name, Namespace: namespace}
		if err := c.Get(ctx, key, vmi); err != nil {
			if !apierrors.IsNotFound(err) {
				return fmt.Errorf("getting VMI %s/%s: %w", namespace, name, err)
			}
			fmt.Printf("VM %s: VMI not yet created, retrying...\n", name)
			select {
			case <-ctx.Done():
				return fmt.Errorf("context cancelled waiting for VM %s/%s: %w", namespace, name, ctx.Err())
			case <-time.After(interval):
			}
			continue
		}

		if vmi.Status.Phase == kubevirtv1.Running {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled waiting for VM %s/%s: %w", namespace, name, ctx.Err())
		case <-time.After(interval):
		}
	}
}

// WaitForAllVMsReady polls all named VMs concurrently using goroutines.
// Returns a map of VM name to error (nil if ready). Each VM is polled
// independently — a failure for one does not cancel others.
func WaitForAllVMsReady(
	ctx context.Context,
	c client.Client,
	names []string,
	namespace string,
	timeout, interval time.Duration,
) map[string]error {
	results := make(map[string]error, len(names))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, name := range names {
		wg.Add(1)
		go func(vmName string) {
			defer wg.Done()
			err := WaitForVMReady(ctx, c, vmName, namespace, timeout, interval)
			mu.Lock()
			results[vmName] = err
			mu.Unlock()
		}(name)
	}

	wg.Wait()
	return results
}
