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

// AnalyzeK8sTopology connects to the cluster and analyzes Deployments for resilience anti-patterns.
func AnalyzeK8sTopology(namespace string) ([]DoctorResult, error) {
	var config *rest.Config
	var err error

	config, err = rest.InClusterConfig()
	if err != nil {
		kubeconfig := os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			if home := homedir.HomeDir(); home != "" {
				kubeconfig = filepath.Join(home, ".kube", "config")
			}
		}

		if kubeconfig != "" {
			config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
			if err != nil {
				return nil, fmt.Errorf("failed to build kubeconfig from %s: %w", kubeconfig, err)
			}
		} else {
			return nil, fmt.Errorf("could not find kubeconfig")
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	if namespace == "" {
		namespace = os.Getenv("ENTROPY_K8S_NAMESPACE")
		if namespace == "" {
			namespace = "default"
		}
	}

	deps, err := clientset.AppsV1().Deployments(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments in namespace %s: %w", namespace, err)
	}

	var results []DoctorResult

	for _, d := range deps.Items {
		result := DoctorResult{ServiceName: d.Name, Issues: []DoctorIssue{}}

		// 1. SPOF
		if d.Spec.Replicas == nil || *d.Spec.Replicas < 2 {
			result.Issues = append(result.Issues, DoctorIssue{
				Severity: "CRITICAL",
				Category: "SPOF",
				Message:  "Deployment has less than 2 replicas configured. If the node or pod fails, there will be downtime.",
			})
		}

		for _, c := range d.Spec.Template.Spec.Containers {
			// 2. Resource Exhaustion
			hasCPU := c.Resources.Limits.Cpu() != nil && !c.Resources.Limits.Cpu().IsZero()
			hasMemory := c.Resources.Limits.Memory() != nil && !c.Resources.Limits.Memory().IsZero()

			if !hasCPU || !hasMemory {
				result.Issues = append(result.Issues, DoctorIssue{
					Severity: "WARNING",
					Category: "RESOURCES",
					Message:  fmt.Sprintf("Container '%s' is missing explicit CPU or Memory limits. A memory leak could exhaust node resources.", c.Name),
				})
			}

			// 3. Observability
			if c.LivenessProbe == nil || c.ReadinessProbe == nil {
				result.Issues = append(result.Issues, DoctorIssue{
					Severity: "WARNING",
					Category: "OBSERVABILITY",
					Message:  fmt.Sprintf("Container '%s' is missing Liveness or Readiness probes. Kubernetes cannot accurately route traffic or restart deadlocks.", c.Name),
				})
			}

			// 4. Security
			if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
				result.Issues = append(result.Issues, DoctorIssue{
					Severity: "CRITICAL",
					Category: "SECURITY",
					Message:  fmt.Sprintf("Container '%s' is running in privileged mode. This is a severe security risk.", c.Name),
				})
			}
		}

		results = append(results, result)
	}

	return results, nil
}
