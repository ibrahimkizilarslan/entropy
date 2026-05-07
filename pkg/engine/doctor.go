package engine

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
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
