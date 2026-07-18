package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// TestTopologyCmdRun_Success drives the actual topologyCmd.Run closure
// against a temp directory with a minimal docker-compose.yml. Like
// doctorCmd, topologyCmd.Run never calls os.Exit — it only prints and
// returns — so this is safe to exercise directly.
func TestTopologyCmdRun_Success(t *testing.T) {
	tmpDir := t.TempDir()
	composeContent := `
services:
  service-a:
    image: nginx
    networks:
      - backend
    depends_on:
      - service-b
  service-b:
    image: redis
    networks:
      - backend
networks:
  backend:
`
	if err := os.WriteFile(filepath.Join(tmpDir, "docker-compose.yml"), []byte(composeContent), 0644); err != nil {
		t.Fatalf("failed to write docker-compose.yml: %v", err)
	}

	t.Chdir(tmpDir)

	topologyCmd.Run(topologyCmd, []string{})
}

// TestTopologyCmdRun_NoComposeFile verifies the error path returns cleanly.
func TestTopologyCmdRun_NoComposeFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	topologyCmd.Run(topologyCmd, []string{})
}
