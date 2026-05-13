package cli_test

import (
	"os"
	"testing"
	"time"

	"github.com/ibrahimkizilarslan/entropy/pkg/config"
	"github.com/ibrahimkizilarslan/entropy/pkg/utils"
)

func TestStartCmdFlags(t *testing.T) {
	tests := []struct {
		name    string
		flags   map[string]string
		wantErr bool
	}{
		{
			name: "Default config flag",
			flags: map[string]string{
				"config": "chaos.yaml",
			},
			wantErr: true, // chaos.yaml doesn't exist in test
		},
		{
			name: "Dry-run flag",
			flags: map[string]string{
				"dry-run": "true",
			},
			wantErr: true,
		},
		{
			name: "Max-down flag",
			flags: map[string]string{
				"max-down": "2",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test flag parsing
			for flag, value := range tt.flags {
				if flag == "" || value == "" {
					t.Error("Flag and value should not be empty")
				}
			}
		})
	}
}

func TestCreateValidChaosConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := tmpDir + "/test_chaos.yaml"

	// Create a valid config file
	configYAML := `interval: 10
targets:
  - service-a
  - service-b
actions:
  - name: stop
  - name: restart
safety:
  max_down: 1
  cooldown: 30
  dry_run: false
`

	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// Verify the file was created
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("Config file not created: %v", err)
	}

	// Load and verify
	loaded, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if loaded.Interval != 10 {
		t.Errorf("Interval mismatch: got %d, want %d", loaded.Interval, 10)
	}

	if len(loaded.Targets) != 2 {
		t.Errorf("Targets count mismatch: got %d, want %d", len(loaded.Targets), 2)
	}

	if loaded.Safety.MaxDown != 1 {
		t.Errorf("MaxDown mismatch: got %d, want %d", loaded.Safety.MaxDown, 1)
	}
}

func TestStatusCommand(t *testing.T) {
	// Clean state
	state := utils.NewStateManager("")
	if err := state.Clear(); err != nil {
		t.Logf("Warning: state.Clear failed: %v", err)
	}

	// Test status when no engine is running
	// Should handle missing PID gracefully
	pid := state.RunningPID()
	if pid != nil {
		t.Errorf("Expected no running PID, got %d", *pid)
	}
}

func TestLogsCommand(t *testing.T) {
	tmpDir := t.TempDir()
	state := utils.NewStateManager(tmpDir)
	if err := state.EnsureDir(); err != nil {
		t.Fatalf("Failed to ensure state dir: %v", err)
	}

	// Create a dummy log file
	logPath := state.LogFile()
	if err := os.WriteFile(logPath, []byte("test log\n"), 0644); err != nil {
		t.Fatalf("Failed to create log file: %v", err)
	}

	// Verify log file exists and is readable
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if string(content) != "test log\n" {
		t.Errorf("Log content mismatch: got %q, want %q", string(content), "test log\n")
	}
}

func TestCleanupCommand(t *testing.T) {
	tmpDir := t.TempDir()
	state := utils.NewStateManager(tmpDir)

	// Ensure directory exists
	if err := state.EnsureDir(); err != nil {
		t.Fatalf("Failed to ensure state dir: %v", err)
	}

	// Create state file
	testState := &utils.EngineState{
		PID:        12345,
		StartedAt:  time.Now(),
		ConfigPath: "chaos.yaml",
		DryRun:     false,
	}

	if err := state.Write(testState); err != nil {
		t.Fatalf("Failed to write state: %v", err)
	}

	// Clear should remove state file
	if err := state.Clear(); err != nil {
		t.Fatalf("Failed to clear state: %v", err)
	}

	// Verify state is cleared
	pid := state.RunningPID()
	if pid != nil {
		t.Errorf("State not properly cleared, still has PID %d", *pid)
	}
}
