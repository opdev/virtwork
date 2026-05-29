// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package resources

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opdev/virtwork/internal/constants"
)

var ErrCloudInitTooLarge = errors.New("cloud-init userdata exceeds 1 MiB Kubernetes limit")

// EnsureNamespace creates a namespace with the given labels if it does not
// already exist. AlreadyExists errors are treated as success (idempotent).
func EnsureNamespace(ctx context.Context, c client.Client, name string, labels map[string]string) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
	}
	err := c.Create(ctx, ns)
	if apierrors.IsAlreadyExists(err) {
		existing := &corev1.Namespace{}
		if err := c.Get(ctx, client.ObjectKeyFromObject(ns), existing); err != nil {
			return fmt.Errorf("fetching existing namespace %s: %w", name, err)
		}
		base := existing.DeepCopy()
		if existing.Labels == nil {
			existing.Labels = make(map[string]string, len(labels))
		}
		for k, v := range labels {
			existing.Labels[k] = v
		}
		return c.Patch(ctx, existing, client.MergeFrom(base))
	}
	return err
}

// CreateService creates a Kubernetes Service. AlreadyExists errors are treated
// as success (idempotent).
func CreateService(ctx context.Context, c client.Client, svc *corev1.Service) error {
	err := c.Create(ctx, svc)
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

// CreateCloudInitSecret creates a Secret holding cloud-init userdata.
// The secret is labeled for cleanup. AlreadyExists errors are treated as
// success (idempotent).
func CreateCloudInitSecret(
	ctx context.Context,
	c client.Client,
	name, namespace, userdata string,
	labels map[string]string,
) error {
	if len(userdata) > constants.MaxSecretDataSize {
		return fmt.Errorf(
			"%w: %q is %d bytes",
			ErrCloudInitTooLarge, name, len(userdata),
		)
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		StringData: map[string]string{
			constants.SecretKeyUserdata: userdata,
		},
	}
	err := c.Create(ctx, secret)
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

// DeleteManagedSecrets lists and deletes secrets matching the given labels in
// the namespace. Returns the count of successfully deleted secrets.
func DeleteManagedSecrets(
	ctx context.Context,
	c client.Client,
	namespace string,
	labels map[string]string,
) (int, error) {
	secretList := &corev1.SecretList{}
	opts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels(labels),
	}
	if err := c.List(ctx, secretList, opts...); err != nil {
		return 0, fmt.Errorf("listing secrets in %s: %w", namespace, err)
	}

	deleted := 0
	var errs []error
	for i := range secretList.Items {
		if err := c.Delete(ctx, &secretList.Items[i]); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			errs = append(errs, fmt.Errorf("deleting secret %s: %w", secretList.Items[i].Name, err))
			continue
		}
		deleted++
	}
	return deleted, errors.Join(errs...)
}

// DeleteManagedServices lists and deletes services matching the given labels in
// the namespace. Returns the count of successfully deleted services.
func DeleteManagedServices(
	ctx context.Context,
	c client.Client,
	namespace string,
	labels map[string]string,
) (int, error) {
	svcList := &corev1.ServiceList{}
	opts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels(labels),
	}
	if err := c.List(ctx, svcList, opts...); err != nil {
		return 0, fmt.Errorf("listing services in %s: %w", namespace, err)
	}

	deleted := 0
	var errs []error
	for i := range svcList.Items {
		if err := c.Delete(ctx, &svcList.Items[i]); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			errs = append(errs, fmt.Errorf("deleting service %s: %w", svcList.Items[i].Name, err))
			continue
		}
		deleted++
	}
	return deleted, errors.Join(errs...)
}
