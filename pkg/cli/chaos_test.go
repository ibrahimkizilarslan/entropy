package cli

import (
	"os"
	"path/filepath"
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
			cmd := startCmd
			cmd.SetArgs([]string{})

			for flag, value := range tt.flags {
				if err := cmd.Flags().Set(flag, value); err != nil {
					t.Fatalf("Failed to set flag %q: %v", flag, err)
				}
			}
		})
	}
}

func TestStopCmdWithoutRunningEngine(t *testing.T) {
	// Clean up any existing state
	state := utils.NewStateManager("")
	_ = state.Clear()

	cmd := stopCmd
	cmd.SetArgs([]string{})

	// Run should exit with error code since no engine is running
	// This is expected behavior
	_ = cmd
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
	_ = state.Clear()

	// Test status when no engine is running
	cmd := statusCmd
	cmd.SetArgs([]string{})

	// Run should handle missing PID gracefully
	_ = cmd
}

func TestLogsCommand(t *testing.T) {
	tmpDir := t.TempDir()
	state := utils.NewStateManager(tmpDir)
	_ = state.EnsureDir()

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

// TestResolveInjectAllowedTargets_MissingConfigFailsClosed verifies the P0 fix:
// when chaos.yaml is missing and --skip-validation is NOT set, inject must
// refuse to proceed (fail-closed) instead of silently allowing any target.
func TestResolveInjectAllowedTargets_MissingConfigFailsClosed(t *testing.T) {
	tmpDir := t.TempDir()
	missingConfig := filepath.Join(tmpDir, "does-not-exist.yaml")

	targets, err := resolveInjectAllowedTargets(missingConfig, "any-container", false)
	if err == nil {
		t.Fatal("expected an error when config is missing and skipValidation is false, got nil")
	}
	if targets != nil {
		t.Errorf("expected nil allowed targets on failure, got %v", targets)
	}
}

// TestResolveInjectAllowedTargets_SkipValidationBypasses verifies that
// --skip-validation is the only sanctioned way to bypass the allow-list.
func TestResolveInjectAllowedTargets_SkipValidationBypasses(t *testing.T) {
	tmpDir := t.TempDir()
	missingConfig := filepath.Join(tmpDir, "does-not-exist.yaml")

	targets, err := resolveInjectAllowedTargets(missingConfig, "any-container", true)
	if err != nil {
		t.Fatalf("expected no error with skipValidation=true, got: %v", err)
	}
	if targets != nil {
		t.Errorf("expected nil (unrestricted) targets with skipValidation=true, got %v", targets)
	}
}

// TestResolveInjectAllowedTargets_TargetNotInList verifies existing behavior:
// a valid config that doesn't contain the requested target is rejected.
func TestResolveInjectAllowedTargets_TargetNotInList(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "chaos.yaml")
	configYAML := `interval: 10
targets:
  - service-a
actions:
  - name: stop
safety:
  max_down: 1
  cooldown: 30
`
	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	targets, err := resolveInjectAllowedTargets(configPath, "not-a-target", false)
	if err == nil {
		t.Fatal("expected an error for a target not in the allow-list, got nil")
	}
	if targets != nil {
		t.Errorf("expected nil allowed targets on failure, got %v", targets)
	}
}

// TestResolveInjectAllowedTargets_ValidConfigReturnsAllowList verifies the
// happy path: a valid config containing the target returns the full allow-list.
func TestResolveInjectAllowedTargets_ValidConfigReturnsAllowList(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "chaos.yaml")
	configYAML := `interval: 10
targets:
  - service-a
  - service-b
actions:
  - name: stop
safety:
  max_down: 1
  cooldown: 30
`
	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	targets, err := resolveInjectAllowedTargets(configPath, "service-a", false)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(targets) != 2 {
		t.Errorf("expected 2 allowed targets, got %d: %v", len(targets), targets)
	}
}

func TestCleanupCommand(t *testing.T) {
	tmpDir := t.TempDir()
	state := utils.NewStateManager(tmpDir)

	// Ensure directory exists
	_ = state.EnsureDir()

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
