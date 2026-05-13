package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ibrahimkizilarslan/entropy/pkg/config"
)

func TestScenarioCommandStructure(t *testing.T) {
	if scenarioCmd.Name() != "scenario" {
		t.Errorf("Command name = %q, want %q", scenarioCmd.Name(), "scenario")
	}

	if scenarioCmd.Short == "" {
		t.Error("Command Short should not be empty")
	}
}

func TestScenarioRunCommandArgs(t *testing.T) {
	cmd := scenarioRunCmd

	tests := []struct {
		name      string
		args      []string
		expectErr bool
	}{
		{
			name:      "No arguments",
			args:      []string{},
			expectErr: true,
		},
		{
			name:      "One argument",
			args:      []string{"scenario.yaml"},
			expectErr: false,
		},
		{
			name:      "Multiple arguments",
			args:      []string{"scenario1.yaml", "scenario2.yaml"},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd.SetArgs(tt.args)
			// Just verify args parsing works
			err := cmd.Args(cmd, tt.args)

			if (err != nil) != tt.expectErr {
				t.Errorf("Got error = %v, want error = %v", err != nil, tt.expectErr)
			}
		})
	}
}

func TestCreateValidScenario(t *testing.T) {
	tmpDir := t.TempDir()
	scenarioPath := filepath.Join(tmpDir, "test-scenario.yaml")

	// Create a valid scenario
	scenarioContent := `
name: "Test Scenario"
description: "A test scenario"
hypothesis: "System should recover after service failure"
steps:
  - probe:
      type: http
      url: "http://localhost:8080/health"
      expect_status: 200
  
  - inject:
      target: "test-service"
      action: stop
  
  - wait: 2s
  
  - probe:
      type: http
      url: "http://localhost:8080/health"
      expect_status: 503
  
  - inject:
      target: "test-service"
      action: restart
  
  - wait: 3s
  
  - probe:
      type: http
      url: "http://localhost:8080/health"
      expect_status: 200
`

	if err := os.WriteFile(scenarioPath, []byte(scenarioContent), 0644); err != nil {
		t.Fatalf("Failed to create scenario file: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(scenarioPath); err != nil {
		t.Fatalf("Scenario file not found: %v", err)
	}

	// Load scenario
	scenario, err := config.LoadScenario(scenarioPath)
	if err != nil {
		t.Fatalf("Failed to load scenario: %v", err)
	}

	if scenario.Name != "Test Scenario" {
		t.Errorf("Scenario name = %q, want %q", scenario.Name, "Test Scenario")
	}

	if len(scenario.Steps) == 0 {
		t.Error("Scenario should have steps")
	}
}


func TestScenarioWithMultipleActions(t *testing.T) {
	tmpDir := t.TempDir()
	scenarioPath := filepath.Join(tmpDir, "multi-action.yaml")

	scenarioContent := `
name: "Multi-Action Scenario"
description: "Test multiple chaos actions"
hypothesis: "System recovers from multiple failures"
steps:
  - inject:
      target: "service-a"
      action: pause
  
  - inject:
      target: "service-b"
      action:
        name: delay
        latency_ms: 500
        duration: 10
  
  - inject:
      target: "service-c"
      action:
        name: limit_cpu
        cpus: 0.5
        duration: 15
  
  - wait: 5s
  
  - probe:
      type: http
      url: "http://localhost:8080/status"
      expect_status: 200
`

	if err := os.WriteFile(scenarioPath, []byte(scenarioContent), 0644); err != nil {
		t.Fatalf("Failed to create scenario: %v", err)
	}

	scenario, err := config.LoadScenario(scenarioPath)
	if err != nil {
		t.Fatalf("Failed to load scenario: %v", err)
	}

	injectionSteps := 0
	for _, step := range scenario.Steps {
		if step.Type == "inject" {
			injectionSteps++
		}
	}

	if injectionSteps != 3 {
		t.Errorf("Expected 3 injection steps, got %d", injectionSteps)
	}
}
