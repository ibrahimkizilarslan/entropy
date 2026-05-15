package engine

import (
	"strings"
	"testing"

	"github.com/ibrahimkizilarslan/entropy/pkg/config"
)

func TestScenarioRunner_Run_Success(t *testing.T) {
	mock := NewMockRuntime()
	
	// Create a simple scenario config
	cfg := &config.ScenarioConfig{
		Name:       "Test Scenario",
		Hypothesis: "System survives stop action",
		Steps: []config.ScenarioStep{
			{
				Type: "inject",
				Target: "service-a",
				Action: &config.ActionSpec{Name: "stop"},
			},
		},
	}

	logs := []string{}
	logCb := func(msg string) {
		logs = append(logs, msg)
	}

	runner := NewScenarioRunner(cfg, "docker", logCb)
	runner.runtime = mock // Inject mock runtime

	result := runner.Run()

	if !result.Success {
		t.Errorf("Expected scenario to succeed, got error: %s", result.Error)
	}
	if result.ExecutedSteps != 1 {
		t.Errorf("Expected 1 executed step, got %d", result.ExecutedSteps)
	}
	
	// Check if mock was called correctly
	if mock.CallCount("StopContainer") != 1 {
		t.Errorf("Expected 1 StopContainer call, got %d", mock.CallCount("StopContainer"))
	}

	// Verify internal state for rollback
	if len(runner.stopped) != 1 || runner.stopped[0] != "service-a" {
		t.Errorf("Expected runner to track stopped container 'service-a', got %v", runner.stopped)
	}

	// Check logs
	logStr := strings.Join(logs, "\n")
	if !strings.Contains(logStr, "Running Scenario: Test Scenario") {
		t.Error("Expected log output to contain scenario name")
	}
}

func TestScenarioRunner_RevertAll(t *testing.T) {
	mock := NewMockRuntime()
	cfg := &config.ScenarioConfig{Name: "Test Revert"}
	runner := NewScenarioRunner(cfg, "docker", nil)
	runner.runtime = mock

	// Manually set state as if actions had been executed
	runner.stopped = append(runner.stopped, "service-a", "service-b")
	runner.paused = append(runner.paused, "service-c")

	runner.RevertAll()

	// Should restart stopped containers
	if mock.CallCount("RestartContainer") != 2 {
		t.Errorf("Expected 2 RestartContainer calls for rollback, got %d", mock.CallCount("RestartContainer"))
	}

	// Should unpause paused containers
	if mock.CallCount("UnpauseContainer") != 1 {
		t.Errorf("Expected 1 UnpauseContainer call for rollback, got %d", mock.CallCount("UnpauseContainer"))
	}
}

func TestScenarioRunner_Run_ProbeFailure(t *testing.T) {
	mock := NewMockRuntime()
	
	cfg := &config.ScenarioConfig{
		Name: "Probe Failure Test",
		Steps: []config.ScenarioStep{
			{
				Type: "probe",
				Probe: &config.ProbeSpec{
					Type: "exec",
					Target: "service-a",
					Command: "ls -la",
				},
			},
		},
	}

	// Configure mock to fail the exec command
	mock.ExecExit = 1 

	runner := NewScenarioRunner(cfg, "docker", nil)
	runner.runtime = mock

	result := runner.Run()

	if result.Success {
		t.Error("Expected scenario to fail due to probe failure")
	}
	if result.ProbesTotal != 1 {
		t.Errorf("Expected 1 probe total, got %d", result.ProbesTotal)
	}
	if result.ProbesPassed != 0 {
		t.Errorf("Expected 0 probes passed, got %d", result.ProbesPassed)
	}
}
