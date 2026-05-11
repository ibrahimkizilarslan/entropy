package utils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ibrahimkizilarslan/entropy/pkg/config"
)

func TestChaosLogger(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "test.log")

	logger, err := NewChaosLogger(logPath)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Test LogStart
	cfg := &config.ChaosConfig{
		Targets:  []string{"svc1", "svc2"},
		Interval: 10,
		Safety: config.SafetyConfig{
			MaxDown:  2,
			Cooldown: 30,
			DryRun:   false,
		},
	}
	logger.LogStart(cfg)

	// Test LogInjection Success
	logger.LogInjection(InjectionEvent{
		Action:       "stop",
		Target:       "svc1",
		Success:      true,
		ResultStatus: "stopped",
	})

	// Test LogInjection Failure
	logger.LogInjection(InjectionEvent{
		Action:  "stop",
		Target:  "svc2",
		Success: false,
		Error:   "container not found",
	})

	// Test LogInjection DryRun
	logger.LogInjection(InjectionEvent{
		Action:       "delay",
		Target:       "svc1",
		Success:      true,
		ResultStatus: "(dry-run)",
		DryRun:       true,
	})

	// Test LogCooldownSkip
	logger.LogCooldownSkip(12.5)

	// Test LogMaxDownSkip
	logger.LogMaxDownSkip([]string{"svc1", "svc2"})

	// Test LogError
	logger.LogError("some engine error")

	// Test LogStop
	logger.LogStop(5, 3)

	logger.Close()

	// Verify content
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	logContent := string(content)

	expectedStrings := []string{
		"ENGINE STARTED | targets=svc1,svc2 interval=10s max_down=2 cooldown=30s dry_run=false",
		"STOP → svc1 | result=stopped",
		"STOP → svc2 | container not found",
		"[DRY-RUN] DELAY → svc1 | result=(dry-run)",
		"COOLDOWN | remaining=12.5s",
		"MAX_DOWN | down=svc1,svc2",
		"ENGINE ERROR | some engine error",
		"ENGINE STOPPED | cycles=5 injections=3",
	}

	for _, str := range expectedStrings {
		if !strings.Contains(logContent, str) {
			t.Errorf("Expected log to contain '%s', but it didn't.\nLog content:\n%s", str, logContent)
		}
	}
}

func TestNewChaosLogger_DefaultPath(t *testing.T) {
	// We should be careful about testing default path which is relative
	// Let's create a temp dir and set it as cwd
	originalCwd, _ := os.Getwd()
	tempDir := t.TempDir()
	_ = os.Chdir(tempDir)
	defer func() { _ = os.Chdir(originalCwd) }()

	logger, err := NewChaosLogger("")
	if err != nil {
		t.Fatalf("Failed to create logger with empty path: %v", err)
	}
	defer logger.Close()

	// Verify it created .entropy/engine.log
	info, err := os.Stat(".entropy/engine.log")
	if err != nil {
		t.Fatalf("Expected default log file to be created, got error: %v", err)
	}
	if info.IsDir() {
		t.Error("Expected file, got directory")
	}
}
