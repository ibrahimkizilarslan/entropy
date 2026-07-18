package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDoctorCmdRun_Success drives the actual doctorCmd.Run closure (not just
// the underlying engine.AnalyzeTopology helper) against a temp directory
// with a minimal docker-compose.yml. doctorCmd.Run never calls os.Exit on
// its own — analysis failures just print and return — so the success path
// is safe to exercise directly in-process.
func TestDoctorCmdRun_Success(t *testing.T) {
	tmpDir := t.TempDir()
	composeContent := `
services:
  service-a:
    image: nginx
    restart: always
    healthcheck:
      test: ["CMD", "true"]
    deploy:
      replicas: 2
      resources:
        limits:
          cpus: "0.5"
          memory: "256M"
`
	if err := os.WriteFile(filepath.Join(tmpDir, "docker-compose.yml"), []byte(composeContent), 0644); err != nil {
		t.Fatalf("failed to write docker-compose.yml: %v", err)
	}

	t.Chdir(tmpDir)

	// Ensure the docker/k8s runtime branch selector is in its default state.
	oldRuntimeType := runtimeType
	runtimeType = "docker"
	defer func() { runtimeType = oldRuntimeType }()

	// doctorCmd.Run only prints and returns on error/success — it never
	// calls os.Exit, so this is safe to invoke directly and will exercise
	// the real command wiring (cmd.Run) rather than just the engine helper.
	doctorCmd.Run(doctorCmd, []string{})
}

// TestDoctorCmdRun_NoComposeFile verifies the error path (no compose file
// found) also returns cleanly without calling os.Exit.
func TestDoctorCmdRun_NoComposeFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	oldRuntimeType := runtimeType
	runtimeType = "docker"
	defer func() { runtimeType = oldRuntimeType }()

	doctorCmd.Run(doctorCmd, []string{})
}
