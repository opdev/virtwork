// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"
	cdiv1beta1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const defaultMaxRetries = 5

var baseRetryBackoff = time.Second

// VMSpecOpts contains all parameters needed to construct a VirtualMachine spec.
type VMSpecOpts struct {
	Name                string
	Namespace           string
	ContainerDiskImage  string
	CloudInitUserdata   string
	CloudInitSecretName string // When set, use UserDataSecretRef instead of inline
	CPUCores            int
	Memory              string
	Labels              map[string]string
	ExtraDisks          []kubevirtv1.Disk
	ExtraVolumes        []kubevirtv1.Volume
	DataVolumeTemplates []kubevirtv1.DataVolumeTemplateSpec
}

// BuildVMSpec constructs a KubeVirt VirtualMachine from the given options.
// It configures a containerDisk for the OS image, cloudInitNoCloud for userdata,
// masquerade networking, and virtio disk bus.
func BuildVMSpec(opts VMSpecOpts) (*kubevirtv1.VirtualMachine, error) {
	runStrategy := kubevirtv1.RunStrategyAlways

	disks := []kubevirtv1.Disk{
		{
			Name: "containerdisk",
			DiskDevice: kubevirtv1.DiskDevice{
				Disk: &kubevirtv1.DiskTarget{
					Bus: "virtio",
				},
			},
		},
		{
			Name: "cloudinitdisk",
			DiskDevice: kubevirtv1.DiskDevice{
				Disk: &kubevirtv1.DiskTarget{
					Bus: "virtio",
				},
			},
		},
	}
	disks = append(disks, opts.ExtraDisks...)

	var cloudInitVolume kubevirtv1.Volume
	if opts.CloudInitSecretName != "" {
		cloudInitVolume = kubevirtv1.Volume{
			Name: "cloudinitdisk",
			VolumeSource: kubevirtv1.VolumeSource{
				CloudInitNoCloud: &kubevirtv1.CloudInitNoCloudSource{
					UserDataSecretRef: &corev1.LocalObjectReference{
						Name: opts.CloudInitSecretName,
					},
				},
			},
		}
	} else {
		cloudInitVolume = kubevirtv1.Volume{
			Name: "cloudinitdisk",
			VolumeSource: kubevirtv1.VolumeSource{
				CloudInitNoCloud: &kubevirtv1.CloudInitNoCloudSource{
					UserData: opts.CloudInitUserdata,
				},
			},
		}
	}

	volumes := []kubevirtv1.Volume{
		{
			Name: "containerdisk",
			VolumeSource: kubevirtv1.VolumeSource{
				ContainerDisk: &kubevirtv1.ContainerDiskSource{
					Image: opts.ContainerDiskImage,
				},
			},
		},
		cloudInitVolume,
	}
	volumes = append(volumes, opts.ExtraVolumes...)

	memQty, err := resource.ParseQuantity(opts.Memory)
	if err != nil {
		return nil, fmt.Errorf("invalid memory %q (expected a quantity like 512Mi, 2Gi, or 4G): %w", opts.Memory, err)
	}

	return &kubevirtv1.VirtualMachine{
		TypeMeta: metav1.TypeMeta{
			APIVersion: kubevirtv1.SchemeGroupVersion.String(),
			Kind:       "VirtualMachine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      opts.Name,
			Namespace: opts.Namespace,
			Labels:    opts.Labels,
		},
		Spec: kubevirtv1.VirtualMachineSpec{
			RunStrategy: &runStrategy,
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: opts.Labels,
				},
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Domain: kubevirtv1.DomainSpec{
						CPU: &kubevirtv1.CPU{
							//nolint:gosec
							Cores: uint32(opts.CPUCores),
						},
						Resources: kubevirtv1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceMemory: memQty,
							},
						},
						Devices: kubevirtv1.Devices{
							Disks: disks,
							Interfaces: []kubevirtv1.Interface{
								{
									Name: "default",
									InterfaceBindingMethod: kubevirtv1.InterfaceBindingMethod{
										Masquerade: &kubevirtv1.InterfaceMasquerade{},
									},
								},
							},
						},
					},
					Networks: []kubevirtv1.Network{
						{
							Name: "default",
							NetworkSource: kubevirtv1.NetworkSource{
								Pod: &kubevirtv1.PodNetwork{},
							},
						},
					},
					Volumes: volumes,
				},
			},
			DataVolumeTemplates: opts.DataVolumeTemplates,
		},
	}, nil
}

// BuildDataVolumeTemplate constructs a DataVolumeTemplateSpec for a blank disk
// with the given name and size.
func BuildDataVolumeTemplate(name, size string) (kubevirtv1.DataVolumeTemplateSpec, error) {
	sizeQty, err := resource.ParseQuantity(size)
	if err != nil {
		return kubevirtv1.DataVolumeTemplateSpec{}, fmt.Errorf("invalid disk size %q (expected a quantity like 10Gi, 500Mi, or 1Ti): %w", size, err)
	}
	return kubevirtv1.DataVolumeTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: cdiv1beta1.DataVolumeSpec{
			Source: &cdiv1beta1.DataVolumeSource{
				Blank: &cdiv1beta1.DataVolumeBlankImage{},
			},
			Storage: &cdiv1beta1.StorageSpec{
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: sizeQty,
					},
				},
			},
		},
	}, nil
}

// CreateVM creates a VirtualMachine. AlreadyExists errors are treated as
// success (idempotent). Transient errors are retried with exponential backoff.
func CreateVM(ctx context.Context, c client.Client, vm *kubevirtv1.VirtualMachine) error {
	return retryOnTransient(ctx, func() error {
		err := c.Create(ctx, vm)
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		return err
	}, defaultMaxRetries)
}

// DeleteVM deletes a VirtualMachine by name and namespace. NotFound errors are
// treated as success (idempotent). Transient errors are retried.
func DeleteVM(ctx context.Context, c client.Client, name, namespace string) error {
	vm := &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	return retryOnTransient(ctx, func() error {
		err := c.Delete(ctx, vm)
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}, defaultMaxRetries)
}

// ListVMs returns VirtualMachines matching the given labels in the namespace.
func ListVMs(
	ctx context.Context,
	c client.Client,
	namespace string,
	labels map[string]string,
) ([]kubevirtv1.VirtualMachine, error) {
	vmList := &kubevirtv1.VirtualMachineList{}
	opts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels(labels),
	}
	if err := c.List(ctx, vmList, opts...); err != nil {
		return nil, fmt.Errorf("listing VMs in %s: %w", namespace, err)
	}
	return vmList.Items, nil
}

// GetVMIPhase returns the current phase of a VirtualMachineInstance.
func GetVMIPhase(
	ctx context.Context,
	c client.Client,
	name, namespace string,
) (kubevirtv1.VirtualMachineInstancePhase, error) {
	vmi := &kubevirtv1.VirtualMachineInstance{}
	key := client.ObjectKey{Name: name, Namespace: namespace}
	if err := c.Get(ctx, key, vmi); err != nil {
		return "", fmt.Errorf("getting VMI %s/%s: %w", namespace, name, err)
	}
	return vmi.Status.Phase, nil
}

// retryOnTransient retries fn on transient API errors with exponential backoff.
func retryOnTransient(ctx context.Context, fn func() error, maxRetries int) error {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("context cancelled: %w", err)
		}

		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		if !isTransientError(lastErr) {
			return lastErr
		}

		if attempt < maxRetries {
			backoff := baseRetryBackoff * time.Duration(1<<uint(attempt))
			select {
			case <-ctx.Done():
				return fmt.Errorf("context cancelled during retry backoff: %w", ctx.Err())
			case <-time.After(backoff):
			}
		}
	}
	return fmt.Errorf("max retries (%d) exceeded: %w", maxRetries, lastErr)
}

// isTransientError returns true for API errors that are worth retrying.
func isTransientError(err error) bool {
	return apierrors.IsTooManyRequests(err) ||
		apierrors.IsServerTimeout(err) ||
		apierrors.IsServiceUnavailable(err) ||
		apierrors.IsInternalError(err)
}
