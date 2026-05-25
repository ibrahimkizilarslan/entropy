package engine

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// resolveNamespace returns the target Kubernetes namespace from the given value,
// falling back to the ENTROPY_K8S_NAMESPACE env var and then "default".
func resolveNamespace(namespace string) string {
	if namespace != "" {
		return namespace
	}
	if ns := os.Getenv("ENTROPY_K8S_NAMESPACE"); ns != "" {
		return ns
	}
	return "default"
}

// buildK8sConfig creates a *rest.Config by trying in-cluster config first,
// then falling back to the KUBECONFIG env var or ~/.kube/config.
func buildK8sConfig() (*rest.Config, error) {
	// Try in-cluster config first (running inside a pod)
	config, err := rest.InClusterConfig()
	if err == nil {
		return config, nil
	}

	// Fallback to kubeconfig file
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		if home := homedir.HomeDir(); home != "" {
			kubeconfig = filepath.Join(home, ".kube", "config")
		}
	}

	if kubeconfig == "" {
		return nil, fmt.Errorf("could not find kubeconfig. Set KUBECONFIG env or create ~/.kube/config")
	}

	config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubeconfig from %s: %w", kubeconfig, err)
	}

	return config, nil
}

// newK8sClientSet creates a Kubernetes clientset and resolves the target namespace.
// This is the single source of truth for K8s client initialization across the codebase.
func newK8sClientSet(namespace string) (*kubernetes.Clientset, string, error) {
	config, err := buildK8sConfig()
	if err != nil {
		return nil, "", err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	ns := resolveNamespace(namespace)
	return clientset, ns, nil
}
