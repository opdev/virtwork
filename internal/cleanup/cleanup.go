// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package cleanup

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"
	cdiv1beta1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/constants"
)

// CleanupResult summarises the outcome of a cleanup operation.
type CleanupResult struct {
	VMsDeleted       int
	ServicesDeleted  int
	SecretsDeleted   int
	DVsDeleted       int
	PVCsDeleted      int
	NamespaceDeleted bool
	Errors           []error
	RunIDs           []string // unique run IDs collected from cleaned-up resources

	DeletedVMNames      []string
	DeletedServiceNames []string
	DeletedSecretNames  []string
	DeletedDVNames      []string
	DeletedPVCNames     []string
}

// CleanupPreview summarises what a cleanup operation would delete, without modifying anything.
type CleanupPreview struct {
	VMCount      int
	ServiceCount int
	SecretCount  int
	DVCount      int
	PVCCount     int
	RunIDs       []string
	TotalCount   int
}

// PreviewCleanup lists virtwork-managed resources that would be deleted, without modifying anything.
// If runID is non-empty, only resources with that specific virtwork/run-id label are counted.
func PreviewCleanup(
	ctx context.Context,
	c client.Client,
	cfg *config.Config,
	runID string,
) (*CleanupPreview, error) {
	preview := &CleanupPreview{}
	managedLabels := map[string]string{
		constants.LabelManagedBy: constants.ManagedByValue,
	}
	if runID != "" {
		managedLabels[constants.LabelRunID] = runID
	}

	listOpts := []client.ListOption{
		client.InNamespace(cfg.Namespace),
		client.MatchingLabels(managedLabels),
	}

	runIDSet := make(map[string]struct{})

	vmList := &kubevirtv1.VirtualMachineList{}
	if err := c.List(ctx, vmList, listOpts...); err != nil {
		return nil, fmt.Errorf("listing VMs in %s: %w", cfg.Namespace, err)
	}
	preview.VMCount = len(vmList.Items)
	for i := range vmList.Items {
		collectRunID(vmList.Items[i].Labels, runIDSet)
	}

	svcList := &corev1.ServiceList{}
	if err := c.List(ctx, svcList, listOpts...); err != nil {
		return nil, fmt.Errorf("listing services in %s: %w", cfg.Namespace, err)
	}
	preview.ServiceCount = len(svcList.Items)
	for i := range svcList.Items {
		collectRunID(svcList.Items[i].Labels, runIDSet)
	}

	secretList := &corev1.SecretList{}
	if err := c.List(ctx, secretList, listOpts...); err != nil {
		return nil, fmt.Errorf("listing secrets in %s: %w", cfg.Namespace, err)
	}
	preview.SecretCount = len(secretList.Items)
	for i := range secretList.Items {
		collectRunID(secretList.Items[i].Labels, runIDSet)
	}

	dvList := &cdiv1beta1.DataVolumeList{}
	if err := c.List(ctx, dvList, listOpts...); err != nil {
		return nil, fmt.Errorf("listing DataVolumes in %s: %w", cfg.Namespace, err)
	}
	preview.DVCount = len(dvList.Items)
	for i := range dvList.Items {
		collectRunID(dvList.Items[i].Labels, runIDSet)
	}

	pvcList := &corev1.PersistentVolumeClaimList{}
	if err := c.List(ctx, pvcList, listOpts...); err != nil {
		return nil, fmt.Errorf("listing PVCs in %s: %w", cfg.Namespace, err)
	}
	preview.PVCCount = len(pvcList.Items)
	for i := range pvcList.Items {
		collectRunID(pvcList.Items[i].Labels, runIDSet)
	}

	for id := range runIDSet {
		preview.RunIDs = append(preview.RunIDs, id)
	}

	preview.TotalCount = preview.VMCount + preview.ServiceCount + preview.SecretCount + preview.DVCount + preview.PVCCount
	return preview, nil
}

// CleanupAll deletes all virtwork-managed resources in the given namespace.
// If runID is non-empty, only resources with that specific virtwork/run-id label are deleted.
// Individual deletion failures are recorded but do not abort the operation.
// If deleteNamespace is true, the namespace itself is deleted as the final step.
func CleanupAll(
	ctx context.Context,
	c client.Client,
	cfg *config.Config,
	deleteNamespace bool,
	runID string,
) (*CleanupResult, error) {
	result := &CleanupResult{}
	managedLabels := map[string]string{
		constants.LabelManagedBy: constants.ManagedByValue,
	}
	if runID != "" {
		managedLabels[constants.LabelRunID] = runID
	}

	runIDSet := make(map[string]struct{})

	// Setup delete options
	opts := []client.DeleteOption{}
	if cfg.DryRun {
		opts = append(opts, client.DryRunAll)
	}

	// Delete VMs by label
	vmList := &kubevirtv1.VirtualMachineList{}
	listOpts := []client.ListOption{
		client.InNamespace(cfg.Namespace),
		client.MatchingLabels(managedLabels),
	}
	if err := c.List(ctx, vmList, listOpts...); err != nil {
		return result, fmt.Errorf("listing VMs in %s: %w", cfg.Namespace, err)
	}

	for i := range vmList.Items {
		collectRunID(vmList.Items[i].Labels, runIDSet)
		if err := c.Delete(ctx, &vmList.Items[i], opts...); err != nil {
			if !apierrors.IsNotFound(err) {
				result.Errors = append(result.Errors, fmt.Errorf("deleting VM %s: %w", vmList.Items[i].Name, err))
			}
			continue
		}
		result.VMsDeleted++
		result.DeletedVMNames = append(result.DeletedVMNames, vmList.Items[i].Name)
	}

	// Delete services by label
	svcList := &corev1.ServiceList{}
	if err := c.List(ctx, svcList, listOpts...); err != nil {
		return result, fmt.Errorf("listing services in %s: %w", cfg.Namespace, err)
	}
	for i := range svcList.Items {
		collectRunID(svcList.Items[i].Labels, runIDSet)
		if err := c.Delete(ctx, &svcList.Items[i], opts...); err != nil {
			if !apierrors.IsNotFound(err) {
				result.Errors = append(result.Errors, fmt.Errorf("deleting service %s: %w", svcList.Items[i].Name, err))
			}
			continue
		}
		result.ServicesDeleted++
		result.DeletedServiceNames = append(result.DeletedServiceNames, svcList.Items[i].Name)
	}

	// Delete secrets by label
	secretList := &corev1.SecretList{}
	if err := c.List(ctx, secretList, listOpts...); err != nil {
		return result, fmt.Errorf("listing secrets in %s: %w", cfg.Namespace, err)
	}
	for i := range secretList.Items {
		collectRunID(secretList.Items[i].Labels, runIDSet)
		if err := c.Delete(ctx, &secretList.Items[i], opts...); err != nil {
			if !apierrors.IsNotFound(err) {
				result.Errors = append(
					result.Errors,
					fmt.Errorf("deleting secret %s: %w", secretList.Items[i].Name, err),
				)
			}
			continue
		}
		result.SecretsDeleted++
		result.DeletedSecretNames = append(result.DeletedSecretNames, secretList.Items[i].Name)
	}

	// Delete DataVolumes by label (before PVCs — DV controller may GC owned PVCs)
	dvList := &cdiv1beta1.DataVolumeList{}
	if err := c.List(ctx, dvList, listOpts...); err != nil {
		return result, fmt.Errorf("listing DataVolumes in %s: %w", cfg.Namespace, err)
	}
	for i := range dvList.Items {
		collectRunID(dvList.Items[i].Labels, runIDSet)
		if err := c.Delete(ctx, &dvList.Items[i], opts...); err != nil {
			if !apierrors.IsNotFound(err) {
				result.Errors = append(result.Errors, fmt.Errorf(
					"deleting DataVolume %s: %w", dvList.Items[i].Name, err),
				)
			}
			continue
		}
		result.DVsDeleted++
		result.DeletedDVNames = append(result.DeletedDVNames, dvList.Items[i].Name)
	}

	// Delete PVCs by label
	pvcList := &corev1.PersistentVolumeClaimList{}
	if err := c.List(ctx, pvcList, listOpts...); err != nil {
		return result, fmt.Errorf("listing PVCs in %s: %w", cfg.Namespace, err)
	}
	for i := range pvcList.Items {
		collectRunID(pvcList.Items[i].Labels, runIDSet)
		if err := c.Delete(ctx, &pvcList.Items[i], opts...); err != nil {
			if !apierrors.IsNotFound(err) {
				result.Errors = append(result.Errors, fmt.Errorf("deleting PVC %s: %w", pvcList.Items[i].Name, err))
			}
			continue
		}
		result.PVCsDeleted++
		result.DeletedPVCNames = append(result.DeletedPVCNames, pvcList.Items[i].Name)
	}

	// Collect unique run IDs
	for id := range runIDSet {
		result.RunIDs = append(result.RunIDs, id)
	}

	// Optionally delete namespace
	if deleteNamespace {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: cfg.Namespace,
			},
		}
		if err := c.Delete(ctx, ns); err != nil {
			if !apierrors.IsNotFound(err) {
				result.Errors = append(result.Errors, fmt.Errorf("deleting namespace %s: %w", cfg.Namespace, err))
			}
		} else {
			result.NamespaceDeleted = true
		}
	}

	return result, nil
}

// collectRunID extracts the virtwork/run-id label from a resource's labels and adds it to the set.
func collectRunID(labels map[string]string, set map[string]struct{}) {
	if id, ok := labels[constants.LabelRunID]; ok && id != "" {
		set[id] = struct{}{}
	}
}
