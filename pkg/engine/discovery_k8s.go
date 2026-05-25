package engine

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
)

// DiscoverK8sTargets connects to Kubernetes and returns a list of target application names
// based on Deployments and StatefulSets in the current namespace.
func DiscoverK8sTargets(namespace string) ([]string, string, error) {
	clientset, ns, err := newK8sClientSet(namespace)
	if err != nil {
		return nil, "", err
	}

	var targets []string

	// Create a context with timeout for API calls
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get Deployments
	deps, err := clientset.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, "", fmt.Errorf("failed to list Deployments in namespace '%s': %w", ns, err)
	}
	for _, d := range deps.Items {
		// Try to use app label if exists, otherwise deployment name
		if app, ok := d.Labels["app"]; ok {
			targets = append(targets, app)
		} else {
			targets = append(targets, d.Name)
		}
	}

	// Get StatefulSets
	sts, err := clientset.AppsV1().StatefulSets(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, "", fmt.Errorf("failed to list StatefulSets in namespace '%s': %w", ns, err)
	}
	for _, s := range sts.Items {
		if app, ok := s.Labels["app"]; ok {
			targets = append(targets, app)
		} else {
			targets = append(targets, s.Name)
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
		return nil, "", fmt.Errorf("no Deployments or StatefulSets found in namespace '%s'", ns)
	}

	sourceInfo := fmt.Sprintf("kubernetes namespace '%s'", ns)
	return uniqueTargets, sourceInfo, nil
}
