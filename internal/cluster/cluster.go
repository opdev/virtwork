// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package cluster

import (
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	kubevirtv1 "kubevirt.io/api/core/v1"
	cdiv1beta1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewScheme builds a runtime.Scheme with core K8s types, KubeVirt types,
// and CDI (Containerized Data Importer) types registered.
func NewScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = kubevirtv1.AddToScheme(scheme)
	_ = cdiv1beta1.AddToScheme(scheme)
	return scheme
}

// Connect creates a controller-runtime client.Client. It first attempts
// in-cluster configuration; on failure it falls back to the kubeconfig at
// the given path (checking the KUBECONFIG env var when the path is empty).
// Both failures produce a wrapped error.
func Connect(kubeconfigPath string) (client.Client, string, error) {
	scheme := NewScheme()

	var contextName string
	if kubeconfigPath == "" {
		kubeconfigPath = os.Getenv("KUBECONFIG")
	}

	restConfig, err := rest.InClusterConfig()
	if err != nil {
		// Not in-cluster, load from kubeconfig
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		if kubeconfigPath != "" {
			loadingRules.ExplicitPath = kubeconfigPath
		}
		configOverrides := &clientcmd.ConfigOverrides{}
		kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
		restConfig, err = kubeConfig.ClientConfig()
		if err != nil {
			return nil, "", fmt.Errorf("failed to build kubeconfig from %q: %w", kubeconfigPath, err)
		}

		// Get the current context name
		rawConfig, err := kubeConfig.RawConfig()
		if err == nil {
			contextName = rawConfig.CurrentContext
		}
	} else {
		// In-cluster - no context name
		contextName = "in-cluster"
	}

	c, err := client.New(restConfig, client.Options{Scheme: scheme})
	if err != nil {
		return nil, "", fmt.Errorf("failed to create controller-runtime client: %w", err)
	}

	return c, contextName, nil
}
