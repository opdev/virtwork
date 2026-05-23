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

// ResolveKubeconfigPath returns the kubeconfig file path to use given
// the explicit path from Viper (--kubeconfig flag / VIRTWORK_KUBECONFIG).
// Precedence: explicitPath > KUBECONFIG env var > "" (triggers in-cluster
// or default ~/.kube/config resolution in Connect).
func ResolveKubeconfigPath(explicitPath string) string {
	if explicitPath != "" {
		return explicitPath
	}
	if v := os.Getenv("KUBECONFIG"); v != "" {
		return v
	}
	return ""
}

// Connect creates a controller-runtime client.Client using the following
// kubeconfig resolution order:
//
//  1. kubeconfigPath (already resolved via ResolveKubeconfigPath: --kubeconfig > VIRTWORK_KUBECONFIG > KUBECONFIG)
//  2. In-cluster service-account token (only attempted when kubeconfigPath is empty)
//  3. Default loading rules (~/.kube/config)
func Connect(kubeconfigPath string) (client.Client, string, error) {
	scheme := NewScheme()

	var restConfig *rest.Config
	var contextName string
	var err error

	if kubeconfigPath != "" {
		restConfig, contextName, err = configFromKubeconfig(kubeconfigPath)
		if err != nil {
			return nil, "", err
		}
	} else {
		restConfig, err = rest.InClusterConfig()
		if err == nil {
			contextName = "in-cluster"
		} else {
			restConfig, contextName, err = configFromKubeconfig("")
			if err != nil {
				return nil, "", err
			}
		}
	}

	c, err := client.New(restConfig, client.Options{Scheme: scheme})
	if err != nil {
		return nil, "", fmt.Errorf("failed to create controller-runtime client: %w", err)
	}

	return c, contextName, nil
}

// configFromKubeconfig loads a rest.Config and context name from a kubeconfig
// file. When path is empty, default loading rules (~/.kube/config) apply.
func configFromKubeconfig(path string) (*rest.Config, string, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if path != "" {
		loadingRules.ExplicitPath = path
	}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules, &clientcmd.ConfigOverrides{})

	restConfig, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, "", fmt.Errorf("failed to build kubeconfig from %q: %w", path, err)
	}

	var contextName string
	rawConfig, rawErr := kubeConfig.RawConfig()
	if rawErr == nil {
		contextName = rawConfig.CurrentContext
	}
	return restConfig, contextName, nil
}
