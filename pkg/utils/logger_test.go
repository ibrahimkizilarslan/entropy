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

	// Initialize the logger in text format for testing
	logger, err := NewChaosLogger(logPath, "text")
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
		"msg=\"ENGINE STARTED\" targets=svc1,svc2 interval=10 max_down=2 cooldown=30 dry_run=false",
		"msg=ACTION action=stop target=svc1 dry_run=false result=stopped",
		"msg=ERROR action=stop target=svc2 dry_run=false error=\"container not found\"",
		"msg=ACTION action=delay target=svc1 dry_run=true result=(dry-run)",
		"msg=COOLDOWN remaining=12.5",
		"msg=MAX_DOWN down=svc1,svc2",
		"msg=\"ENGINE ERROR\" message=\"some engine error\"",
		"msg=\"ENGINE STOPPED\" cycles=5 injections=3",
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
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	logger, err := NewChaosLogger("", "text")
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
