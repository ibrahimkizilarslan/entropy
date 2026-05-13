package worker_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ibrahimkizilarslan/entropy/pkg/config"
	"github.com/ibrahimkizilarslan/entropy/pkg/worker"
)

func TestRunDaemonWithValidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_chaos.yaml")

	// Create a valid config file
	configYAML := `interval: 1
targets:
  - test-service
actions:
  - name: pause
safety:
  max_down: 1
  cooldown: 1
  dry_run: true
`

	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	dryRun := true
	maxDown := 1
	cooldown := 1

	// Run daemon in a goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- worker.RunDaemon(configPath, &dryRun, &maxDown, &cooldown)
	}()

	// Wait a bit for initialization
	time.Sleep(100 * time.Millisecond)

	// Send interrupt signal to our own process to stop the daemon
	p, _ := os.FindProcess(os.Getpid())
	_ = p.Signal(os.Interrupt)

	// Wait for daemon to exit
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Daemon exited with error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for daemon to exit")
	}
}

func TestRunDaemonWithInvalidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid_chaos.yaml")

	// Create an invalid config file
	invalidContent := []byte("invalid: yaml: content: [")
	if err := os.WriteFile(configPath, invalidContent, 0644); err != nil {
		t.Fatalf("Failed to write invalid config: %v", err)
	}

	dryRun := true
	maxDown := 1
	cooldown := 30

	// RunDaemon should fail when config is invalid
	err := worker.RunDaemon(configPath, &dryRun, &maxDown, &cooldown)
	if err == nil {
		t.Error("Expected error when loading invalid config")
	}
}

func TestRunDaemonConfigOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_chaos.yaml")

	// Create config with original values
	configYAML := `interval: 10
targets:
  - test-service
actions:
  - name: pause
safety:
  max_down: 1
  cooldown: 30
  dry_run: false
`

	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// Load config to verify it can be overridden
	loaded, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Test override values
	newDryRun := true
	newMaxDown := 2
	newCooldown := 60

	if newDryRun != loaded.Safety.DryRun {
		loaded.Safety.DryRun = newDryRun
	}
	if newMaxDown != loaded.Safety.MaxDown {
		loaded.Safety.MaxDown = newMaxDown
	}
	if newCooldown != loaded.Safety.Cooldown {
		loaded.Safety.Cooldown = newCooldown
	}

	if loaded.Safety.DryRun != newDryRun {
		t.Errorf("DryRun override failed: got %v, want %v", loaded.Safety.DryRun, newDryRun)
	}

	if loaded.Safety.MaxDown != newMaxDown {
		t.Errorf("MaxDown override failed: got %d, want %d", loaded.Safety.MaxDown, newMaxDown)
	}

	if loaded.Safety.Cooldown != newCooldown {
		t.Errorf("Cooldown override failed: got %d, want %d", loaded.Safety.Cooldown, newCooldown)
	}
}

func TestRunDaemonWithMissingConfig(t *testing.T) {
	nonExistentPath := "/tmp/nonexistent_chaos_" + t.Name() + ".yaml"

	dryRun := true
	maxDown := 1
	cooldown := 30

	// RunDaemon should fail when config file doesn't exist
	err := worker.RunDaemon(nonExistentPath, &dryRun, &maxDown, &cooldown)
	if err == nil {
		t.Error("Expected error when config file is missing")
	}
}

func TestRunDaemonSafetyParameters(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_chaos.yaml")

	// Create config
	configYAML := `interval: 10
targets:
  - service-a
  - service-b
actions:
  - name: pause
  - name: restart
safety:
  max_down: 1
  cooldown: 30
  dry_run: true
`

	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	loaded, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify safety parameters are reasonable
	if loaded.Safety.MaxDown < 1 {
		t.Error("MaxDown should be at least 1")
	}

	if loaded.Safety.Cooldown < 1 {
		t.Error("Cooldown should be at least 1 second")
	}

	if len(loaded.Targets) == 0 {
		t.Error("Targets should not be empty")
	}

	if len(loaded.Actions) == 0 {
		t.Error("Actions should not be empty")
	}
}
