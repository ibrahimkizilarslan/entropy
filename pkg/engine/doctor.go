package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	yaml "gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// ComposeService represents a subset of docker-compose service configuration
// used by the doctor for analyzing resilience.
type ComposeService struct {
	Image       string             `yaml:"image"`
	Deploy      *DeployConfig      `yaml:"deploy"`
	Restart     string             `yaml:"restart"`
	HealthCheck *HealthCheckConfig `yaml:"healthcheck"`
	Privileged  bool               `yaml:"privileged"`
	Networks    interface{}        `yaml:"networks"` // Can be list or map
}

type DeployConfig struct {
	Replicas  int             `yaml:"replicas"`
	Resources *ResourceConfig `yaml:"resources"`
}

type ResourceConfig struct {
	Limits *ResourceLimits `yaml:"limits"`
}

type ResourceLimits struct {
	CPUs   string `yaml:"cpus"`
	Memory string `yaml:"memory"`
}

type HealthCheckConfig struct {
	Test    interface{} `yaml:"test"` // can be string or list
	Disable bool        `yaml:"disable"`
}

type ComposeFile struct {
	Services map[string]ComposeService `yaml:"services"`
}

// DoctorIssue represents a single resilience issue found in a service
type DoctorIssue struct {
	Severity string // "CRITICAL", "WARNING"
	Category string // "SPOF", "RESOURCES", "RECOVERY", "OBSERVABILITY", "SECURITY"
	Message  string
}

// DoctorResult represents the analysis result for a specific service
type DoctorResult struct {
	ServiceName string
	Issues      []DoctorIssue
}

// AnalyzeTopology reads docker-compose.yml from the given directory and analyzes it for resilience anti-patterns.
func AnalyzeTopology(dir string) ([]DoctorResult, error) {
	var composePath string
	candidates := []string{"docker-compose.yml", "docker-compose.yaml", "compose.yaml"}
	for _, c := range candidates {
		p := filepath.Join(dir, c)
		if _, err := os.Stat(p); err == nil {
			composePath = p
			break
		}
	}

	if composePath == "" {
		return nil, fmt.Errorf("could not find any compose file in %s", dir)
	}

	data, err := os.ReadFile(composePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read compose file: %w", err)
	}

	var compose ComposeFile
	if err := yaml.Unmarshal(data, &compose); err != nil {
		return nil, fmt.Errorf("failed to parse compose file: %w", err)
	}

	var results []DoctorResult

	for name, svc := range compose.Services {
		result := DoctorResult{ServiceName: name, Issues: []DoctorIssue{}}

		// 1. SPOF (Single Point of Failure)
		if svc.Deploy == nil || svc.Deploy.Replicas < 2 {
			result.Issues = append(result.Issues, DoctorIssue{
				Severity: "CRITICAL",
				Category: "SPOF",
				Message:  "Service has less than 2 replicas configured. If the node or container fails, there will be downtime.",
			})
		}

		// 2. Resource Exhaustion
		hasCPU := svc.Deploy != nil && svc.Deploy.Resources != nil && svc.Deploy.Resources.Limits != nil && svc.Deploy.Resources.Limits.CPUs != ""
		hasMemory := svc.Deploy != nil && svc.Deploy.Resources != nil && svc.Deploy.Resources.Limits != nil && svc.Deploy.Resources.Limits.Memory != ""

		if !hasCPU || !hasMemory {
			result.Issues = append(result.Issues, DoctorIssue{
				Severity: "WARNING",
				Category: "RESOURCES",
				Message:  "Service is missing explicit CPU or Memory limits. A memory leak could exhaust host resources and crash other services.",
			})
		}

		// 3. Recovery Capability
		if svc.Restart == "" || svc.Restart == "no" {
			// Swarm/Deploy restart policy is also an option, but we check standard restart first
			result.Issues = append(result.Issues, DoctorIssue{
				Severity: "CRITICAL",
				Category: "RECOVERY",
				Message:  "No automatic restart policy defined. If the application crashes, it requires manual intervention.",
			})
		}

		// 4. Observability
		if svc.HealthCheck == nil || svc.HealthCheck.Disable {
			result.Issues = append(result.Issues, DoctorIssue{
				Severity: "WARNING",
				Category: "OBSERVABILITY",
				Message:  "No healthcheck configured. Orchestrators cannot accurately determine if the application is ready to receive traffic.",
			})
		}

		// 5. Security / Blast Radius
		if svc.Privileged {
			result.Issues = append(result.Issues, DoctorIssue{
				Severity: "CRITICAL",
				Category: "SECURITY",
				Message:  "Container is running in privileged mode. A compromise could allow attackers to gain root access to the host.",
			})
		}

		results = append(results, result)
	}

	return results, nil
}

// AnalyzeKubernetes connects to the cluster and analyzes Deployments in the given namespace
// for resilience anti-patterns (SPOF, Resource Limits, Probes, Privileged mode).
func AnalyzeKubernetes(namespace string) ([]DoctorResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	config, err := rest.InClusterConfig()
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
				return nil, fmt.Errorf("failed to build kubeconfig: %w", err)
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

	deps, err := clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments in namespace '%s': %w", namespace, err)
	}

	var results []DoctorResult

	for _, d := range deps.Items {
		result := DoctorResult{ServiceName: d.Name, Issues: []DoctorIssue{}}

		// 1. SPOF (Single Point of Failure)
		if d.Spec.Replicas == nil || *d.Spec.Replicas < 2 {
			result.Issues = append(result.Issues, DoctorIssue{
				Severity: "CRITICAL",
				Category: "SPOF",
				Message:  "Deployment has less than 2 replicas configured. If the node or pod fails, there will be downtime.",
			})
		}

		// Analyze containers
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

			// 3. Observability (Healthchecks)
			if c.LivenessProbe == nil || c.ReadinessProbe == nil {
				result.Issues = append(result.Issues, DoctorIssue{
					Severity: "WARNING",
					Category: "OBSERVABILITY",
					Message:  fmt.Sprintf("Container '%s' is missing Liveness or Readiness probes. K8s cannot accurately detect application hangs or ready state.", c.Name),
				})
			}

			// 4. Security (Privileged)
			if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
				result.Issues = append(result.Issues, DoctorIssue{
					Severity: "CRITICAL",
					Category: "SECURITY",
					Message:  fmt.Sprintf("Container '%s' is running in privileged mode. A compromise could allow node-level root access.", c.Name),
				})
			}
		}

		results = append(results, result)
	}

	return results, nil
}
