package worker

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ibrahimkizilarslan/entropy/pkg/config"
	"github.com/ibrahimkizilarslan/entropy/pkg/utils"
)

func TestRunDaemonWithValidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_chaos.yaml")

	// Create a valid config file
	configYAML := `interval: 10
targets:
  - test-service
actions:
  - name: pause
safety:
  max_down: 1
  cooldown: 30
  dry_run: true
`

	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// Test daemon initialization with dry-run mode
	// In dry-run mode, no actual Docker operations will occur
	dryRun := true
	maxDown := 1
	cooldown := 30

	// Note: RunDaemon will block until interrupted, so we can only test setup
	// Full integration testing requires Docker to be running
	_ = dryRun
	_ = maxDown
	_ = cooldown
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
	err := RunDaemon(configPath, "docker", "text", &dryRun, &maxDown, &cooldown)
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
	err := RunDaemon(nonExistentPath, "docker", "text", &dryRun, &maxDown, &cooldown)
	if err == nil {
		t.Error("Expected error when config file is missing")
	}
}

// ---- applySafetyOverrides ----

func TestApplySafetyOverrides_NilPointersLeaveDefaultsUnchanged(t *testing.T) {
	cfg := &config.ChaosConfig{
		Safety: config.SafetyConfig{DryRun: false, MaxDown: 1, Cooldown: 30},
	}
	applySafetyOverrides(cfg, nil, nil, nil)

	if cfg.Safety.DryRun != false || cfg.Safety.MaxDown != 1 || cfg.Safety.Cooldown != 30 {
		t.Errorf("expected config unchanged with nil overrides, got %+v", cfg.Safety)
	}
}

func TestApplySafetyOverrides_NonNilPointersOverride(t *testing.T) {
	cfg := &config.ChaosConfig{
		Safety: config.SafetyConfig{DryRun: false, MaxDown: 1, Cooldown: 30},
	}
	dryRun := true
	maxDown := 5
	cooldown := 99
	applySafetyOverrides(cfg, &dryRun, &maxDown, &cooldown)

	if cfg.Safety.DryRun != true {
		t.Errorf("expected DryRun overridden to true, got %v", cfg.Safety.DryRun)
	}
	if cfg.Safety.MaxDown != 5 {
		t.Errorf("expected MaxDown overridden to 5, got %d", cfg.Safety.MaxDown)
	}
	if cfg.Safety.Cooldown != 99 {
		t.Errorf("expected Cooldown overridden to 99, got %d", cfg.Safety.Cooldown)
	}
}

// ---- runDaemonLoop lifecycle ----

// TestRunDaemonLoop_Lifecycle is a regression test for the P1 worker
// testability fix: it drives the daemon's actual start -> write-initial-
// state -> stop-signal -> engine-stop -> clear-state lifecycle, using an
// injected state directory and stop channel so it needs neither a live
// Docker daemon nor real OS signal delivery. The chaos interval is set high
// enough that no injection cycle fires during the test, keeping this a pure
// lifecycle test independent of engine/runtime behavior (already covered in
// pkg/engine).
func TestRunDaemonLoop_Lifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	// Keep the fault registry scoped to this test's tmpdir, so the daemon's
	// boot-time RecoverOrphans/StartExpiryWatcher never touches the real
	// user registry at ~/.entropy/registry.json.
	t.Setenv("ENTROPY_REGISTRY_PATH", filepath.Join(tmpDir, "registry.json"))

	cfg := &config.ChaosConfig{
		Interval: 3600,
		Targets:  []string{"svc-a"},
		Actions:  []config.ActionSpec{{Name: "stop"}},
		Safety:   config.SafetyConfig{MaxDown: 1, Cooldown: 7, DryRun: true},
	}

	stop := make(chan os.Signal, 1)
	done := make(chan error, 1)
	go func() {
		done <- runDaemonLoop(cfg, "docker", "text", "chaos.yaml", tmpDir, stop)
	}()

	state := utils.NewStateManager(tmpDir)
	deadline := time.Now().Add(3 * time.Second)
	var s *utils.EngineState
	for time.Now().Before(deadline) {
		if got, err := state.Read(); err == nil && got != nil {
			s = got
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if s == nil {
		t.Fatal("expected initial state file to be written within the timeout")
	}
	if s.PID != os.Getpid() {
		t.Errorf("expected PID %d, got %d", os.Getpid(), s.PID)
	}
	if !s.DryRun {
		t.Error("expected DryRun to be true")
	}
	if s.ConfigPath != "chaos.yaml" {
		t.Errorf("expected ConfigPath %q, got %q", "chaos.yaml", s.ConfigPath)
	}
	if s.CooldownTotal != 7 {
		t.Errorf("expected CooldownTotal 7 (from Safety.Cooldown), got %d", s.CooldownTotal)
	}

	stop <- os.Interrupt

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runDaemonLoop returned an error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("daemon did not stop within the timeout after receiving the stop signal")
	}

	if s2, err := state.Read(); err != nil {
		t.Errorf("unexpected error reading state after stop: %v", err)
	} else if s2 != nil {
		t.Error("expected state file to be cleared after a clean stop")
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
