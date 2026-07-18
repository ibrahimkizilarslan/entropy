package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// TestValidateCmdRun_Success drives the actual validateCmd.Run closure
// against a valid config file. Only the success path is exercised directly
// in-process — the error path calls os.Exit(1), which would terminate the
// test binary.
func TestValidateCmdRun_Success(t *testing.T) {
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
		t.Fatalf("failed to write config: %v", err)
	}

	validateCmd.Run(validateCmd, []string{configPath})
}
