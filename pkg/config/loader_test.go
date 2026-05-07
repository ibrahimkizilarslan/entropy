package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetDefaults(t *testing.T) {
	cfg := ChaosConfig{}
	setDefaults(&cfg)

	if cfg.Safety.MaxDown != 1 {
		t.Errorf("expected MaxDown=1, got %d", cfg.Safety.MaxDown)
	}
	if cfg.Safety.Cooldown != 30 {
		t.Errorf("expected Cooldown=30, got %d", cfg.Safety.Cooldown)
	}
	if len(cfg.Actions) != 1 || cfg.Actions[0].Name != "stop" {
		t.Errorf("expected default action 'stop', got %v", cfg.Actions)
	}

	// Test default values for specific actions
	cfgWithActions := ChaosConfig{
		Actions: []ActionSpec{
			{Name: "delay"},
			{Name: "loss"},
			{Name: "limit_cpu"},
			{Name: "limit_memory"},
		},
	}
	setDefaults(&cfgWithActions)

	if cfgWithActions.Actions[0].LatencyMs != 300 {
		t.Errorf("expected LatencyMs=300 for delay")
	}
	if cfgWithActions.Actions[1].LossPercent != 20 {
		t.Errorf("expected LossPercent=20 for loss")
	}
	if cfgWithActions.Actions[2].CPUs != 0.25 {
		t.Errorf("expected CPUs=0.25 for limit_cpu")
	}
	if cfgWithActions.Actions[3].MemoryMB != 128 {
		t.Errorf("expected MemoryMB=128 for limit_memory")
	}
}

func TestLoadConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "chaos.yaml")

	yamlContent := `
interval: 5
targets:
  - test-svc
actions:
  - name: restart
safety:
  max_down: 2
  cooldown: 10
  dry_run: true
`
	err := os.WriteFile(configPath, []byte(yamlContent), 0644)
	if err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Interval != 5 {
		t.Errorf("expected Interval=5, got %d", cfg.Interval)
	}
	if cfg.Safety.MaxDown != 2 {
		t.Errorf("expected MaxDown=2, got %d", cfg.Safety.MaxDown)
	}
	if cfg.Safety.DryRun != true {
		t.Errorf("expected DryRun=true")
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	_, err := LoadConfig("nonexistent.yaml")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestLoadScenario(t *testing.T) {
	tempDir := t.TempDir()
	scenarioPath := filepath.Join(tempDir, "test-scenario.yaml")

	yamlContent := `
name: "Test Scenario"
description: "A test scenario"
hypothesis: "System survives"
steps:
  - wait: 1s
`
	err := os.WriteFile(scenarioPath, []byte(yamlContent), 0644)
	if err != nil {
		t.Fatalf("failed to write test scenario: %v", err)
	}

	cfg, err := LoadScenario(scenarioPath)
	if err != nil {
		t.Fatalf("LoadScenario failed: %v", err)
	}

	if cfg.Name != "Test Scenario" {
		t.Errorf("expected Name='Test Scenario', got '%s'", cfg.Name)
	}
	if len(cfg.Steps) != 1 {
		t.Errorf("expected 1 step, got %d", len(cfg.Steps))
	}
}
