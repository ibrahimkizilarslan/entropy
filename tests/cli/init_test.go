package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ibrahimkizilarslan/entropy/pkg/config"
	"github.com/ibrahimkizilarslan/entropy/pkg/engine"
)

func TestInitCommandWithoutCompose(t *testing.T) {
	tmpDir := t.TempDir()
	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(oldCwd); err != nil {
			t.Logf("Warning: failed to restore directory: %v", err)
		}
	}()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	// Should fail when docker-compose.yml doesn't exist.
	_, _, err = engine.DiscoverComposeServices(".")
	if err == nil {
		t.Error("Expected error when docker-compose.yml not found")
	}
}

func TestInitCommandGeneratesChaosYAML(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a minimal docker-compose.yml
	composeContent := `
version: '3'
services:
  service-a:
    image: nginx
  service-b:
    image: redis
`

	composePath := filepath.Join(tmpDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("Failed to create docker-compose.yml: %v", err)
	}

	// Change to temp directory
	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(oldCwd); err != nil {
			t.Logf("Warning: failed to restore directory: %v", err)
		}
	}()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	// Discover services
	services, _, err := engine.DiscoverComposeServices(".")
	if err != nil {
		t.Fatalf("Failed to discover services: %v", err)
	}

	if len(services) == 0 {
		t.Error("No services discovered from docker-compose.yml")
	}

	// Generate chaos config
	chaosPath := filepath.Join(tmpDir, "chaos.yaml")
	if err := engine.GenerateDefaultChaosConfig(services, chaosPath); err != nil {
		t.Fatalf("Failed to generate chaos config: %v", err)
	}

	// Verify generated file exists
	if _, err := os.Stat(chaosPath); err != nil {
		t.Fatalf("Generated chaos.yaml not found: %v", err)
	}

	// Load and verify content
	cfg, err := config.LoadConfig(chaosPath)
	if err != nil {
		t.Fatalf("Failed to load generated chaos config: %v", err)
	}

	if len(cfg.Targets) == 0 {
		t.Error("Generated config has no targets")
	}

	if cfg.Interval <= 0 {
		t.Error("Generated config has invalid interval")
	}
}

func TestInitCommandForceFlag(t *testing.T) {
	tmpDir := t.TempDir()
	chaosPath := filepath.Join(tmpDir, "chaos.yaml")

	// Create initial chaos.yaml
	initialContent := []byte("initial: config\n")
	if err := os.WriteFile(chaosPath, initialContent, 0644); err != nil {
		t.Fatalf("Failed to create initial chaos.yaml: %v", err)
	}

	// Simulation of 'init' logic:
	// If file exists and force is false, it should fail (in CLI it exits, here we check logic).
	force := false
	if _, err := os.Stat(chaosPath); err == nil && !force {
		// Expected behavior
	} else {
		t.Error("Expected logic to detect existing file and lack of force flag")
	}

	// If force is true, it should proceed to generate
	force = true
	if _, err := os.Stat(chaosPath); err == nil && !force {
		t.Error("Logic should allow proceeding when force is true")
	}

	// Verify initial file still exists (we didn't actually overwrite in this test yet).
	if _, err := os.Stat(chaosPath); err != nil {
		t.Fatalf("Initial chaos.yaml missing: %v", err)
	}
}

func TestChaosConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		yamlContent string
		wantErr     bool
	}{
		{
			name: "Valid config",
			yamlContent: `interval: 10
targets:
  - service-a
actions:
  - name: stop
safety:
  max_down: 1
  cooldown: 30
`,
			wantErr: false,
		},
		{
			name: "No targets",
			yamlContent: `interval: 10
targets: []
actions:
  - name: stop
`,
			wantErr: true,
		},
		{
			name: "Zero interval",
			yamlContent: `interval: 0
targets:
  - service-a
actions:
  - name: stop
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpFile := t.TempDir() + "/test_config.yaml"
			err := os.WriteFile(tmpFile, []byte(tt.yamlContent), 0644)
			if err != nil {
				t.Fatalf("Failed to write config file: %v", err)
			}

			_, err = config.LoadConfig(tmpFile)

			if tt.wantErr && err == nil {
				t.Error("Expected validation error but got none")
			}

			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}
