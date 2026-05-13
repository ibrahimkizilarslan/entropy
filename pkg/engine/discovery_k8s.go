package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// DiscoverK8sTargets connects to Kubernetes and returns a list of target application names
// based on Deployments and StatefulSets in the current namespace.
func DiscoverK8sTargets(namespace string) ([]string, string, error) {
	var config *rest.Config
	var err error

	// Try in-cluster config first
	config, err = rest.InClusterConfig()
	if err != nil {
		// Fallback to kubeconfig
		kubeconfig := os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			if home := homedir.HomeDir(); home != "" {
				kubeconfig = filepath.Join(home, ".kube", "config")
			}
		}

		if kubeconfig != "" {
			config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
			if err != nil {
				return nil, "", fmt.Errorf("failed to build kubeconfig from %s: %w", kubeconfig, err)
			}
		} else {
			return nil, "", fmt.Errorf("could not find kubeconfig")
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	if namespace == "" {
		namespace = os.Getenv("ENTROPY_K8S_NAMESPACE")
		if namespace == "" {
			namespace = "default"
		}
	}

	var targets []string

	// Get Deployments
	deps, err := clientset.AppsV1().Deployments(namespace).List(context.Background(), metav1.ListOptions{})
	if err == nil {
		for _, d := range deps.Items {
			// Try to use app label if exists, otherwise deployment name
			if app, ok := d.Labels["app"]; ok {
				targets = append(targets, app)
			} else {
				targets = append(targets, d.Name)
			}
		}
	}

	// Get StatefulSets
	sts, err := clientset.AppsV1().StatefulSets(namespace).List(context.Background(), metav1.ListOptions{})
	if err == nil {
		for _, s := range sts.Items {
			if app, ok := s.Labels["app"]; ok {
				targets = append(targets, app)
			} else {
				targets = append(targets, s.Name)
			}
		}
	}

	// Deduplicate targets
	targetMap := make(map[string]bool)
	var uniqueTargets []string
	for _, t := range targets {
		if !targetMap[t] {
			targetMap[t] = true
			uniqueTargets = append(uniqueTargets, t)
		}
	}

	if len(uniqueTargets) == 0 {
		return nil, "", fmt.Errorf("no Deployments or StatefulSets found in namespace '%s'", namespace)
	}

	sourceInfo := fmt.Sprintf("kubernetes namespace '%s'", namespace)
	return uniqueTargets, sourceInfo, nil
}
