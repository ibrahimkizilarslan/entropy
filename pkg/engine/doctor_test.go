package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAnalyzeTopology_AllIssues(t *testing.T) {
	dir := t.TempDir()
	compose := `services:
  web:
    image: nginx:latest
  db:
    image: postgres:15
    privileged: true
`
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(compose), 0644); err != nil {
		t.Fatal(err)
	}

	results, err := AnalyzeTopology(dir)
	if err != nil {
		t.Fatalf("AnalyzeTopology failed: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("Expected 2 service results, got %d", len(results))
	}

	// Find db service result
	var dbResult *DoctorResult
	for i, r := range results {
		if r.ServiceName == "db" {
			dbResult = &results[i]
			break
		}
	}
	if dbResult == nil {
		t.Fatal("Expected to find 'db' service in results")
	}

	// db should have: SPOF, RESOURCES, RECOVERY, OBSERVABILITY, SECURITY
	hasCategory := func(cat string) bool {
		for _, issue := range dbResult.Issues {
			if issue.Category == cat {
				return true
			}
		}
		return false
	}

	expectedCategories := []string{"SPOF", "RESOURCES", "RECOVERY", "OBSERVABILITY", "SECURITY"}
	for _, cat := range expectedCategories {
		if !hasCategory(cat) {
			t.Errorf("Expected db to have '%s' issue, but it was missing", cat)
		}
	}
}

func TestAnalyzeTopology_HealthyService(t *testing.T) {
	dir := t.TempDir()
	compose := `services:
  web:
    image: nginx:latest
    restart: always
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost"]
    deploy:
      replicas: 3
      resources:
        limits:
          cpus: "0.5"
          memory: "256M"
`
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(compose), 0644); err != nil {
		t.Fatal(err)
	}

	results, err := AnalyzeTopology(dir)
	if err != nil {
		t.Fatalf("AnalyzeTopology failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	// A healthy service should have zero issues
	if len(results[0].Issues) != 0 {
		t.Errorf("Expected 0 issues for healthy service, got %d:", len(results[0].Issues))
		for _, issue := range results[0].Issues {
			t.Errorf("  - [%s] %s: %s", issue.Severity, issue.Category, issue.Message)
		}
	}
}

func TestAnalyzeTopology_NoFile(t *testing.T) {
	dir := t.TempDir()
	_, err := AnalyzeTopology(dir)
	if err == nil {
		t.Error("Expected error for missing compose file")
	}
}

func TestAnalyzeTopology_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("invalid: [yaml: {"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := AnalyzeTopology(dir)
	if err == nil {
		t.Error("Expected error for invalid YAML")
	}
}

func TestAnalyzeTopology_RestartPolicies(t *testing.T) {
	dir := t.TempDir()
	compose := `services:
  with-restart:
    image: nginx
    restart: unless-stopped
  without-restart:
    image: redis
  restart-no:
    image: postgres
    restart: "no"
`
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(compose), 0644); err != nil {
		t.Fatal(err)
	}

	results, err := AnalyzeTopology(dir)
	if err != nil {
		t.Fatalf("AnalyzeTopology failed: %v", err)
	}

	hasRecoveryIssue := func(name string) bool {
		for _, r := range results {
			if r.ServiceName == name {
				for _, issue := range r.Issues {
					if issue.Category == "RECOVERY" {
						return true
					}
				}
			}
		}
		return false
	}

	if hasRecoveryIssue("with-restart") {
		t.Error("with-restart should NOT have RECOVERY issue")
	}
	if !hasRecoveryIssue("without-restart") {
		t.Error("without-restart SHOULD have RECOVERY issue")
	}
	if !hasRecoveryIssue("restart-no") {
		t.Error("restart-no SHOULD have RECOVERY issue")
	}
}

func TestAnalyzeTopology_ComposeYamlVariants(t *testing.T) {
	// Test that compose.yaml filename is also detected
	dir := t.TempDir()
	compose := `services:
  web:
    image: nginx
    restart: always
`
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(compose), 0644); err != nil {
		t.Fatal(err)
	}

	results, err := AnalyzeTopology(dir)
	if err != nil {
		t.Fatalf("AnalyzeTopology failed with compose.yaml: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
}
