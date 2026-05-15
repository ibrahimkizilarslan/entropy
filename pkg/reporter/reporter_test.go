package reporter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ibrahimkizilarslan/entropy/pkg/engine"
)

func TestGenerateJSONReport(t *testing.T) {
	dir := t.TempDir()
	reportPath := filepath.Join(dir, "report.json")

	data := ReportData{
		ScenarioName:   "Test Scenario",
		Hypothesis:     "System Survives",
		Timestamp:      "2023-10-27T10:00:00Z",
		EntropyVersion: "v1.0.0",
		Result: engine.ScenarioResult{
			Success:       true,
			ExecutedSteps: 5,
			TotalSteps:    5,
		},
	}

	err := GenerateJSONReport(data, reportPath)
	if err != nil {
		t.Fatalf("GenerateJSONReport failed: %v", err)
	}

	content, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("Failed to read generated report: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("Generated report is not valid JSON: %v", err)
	}

	if parsed["ScenarioName"] != "Test Scenario" {
		t.Errorf("Expected ScenarioName to be 'Test Scenario', got %v", parsed["ScenarioName"])
	}
}

func TestGenerateHTMLReport(t *testing.T) {
	dir := t.TempDir()
	reportPath := filepath.Join(dir, "report.html")

	data := ReportData{
		ScenarioName:   "Test Scenario",
		Hypothesis:     "System Survives",
		Timestamp:      "2023-10-27T10:00:00Z",
		EntropyVersion: "v1.0.0",
		Result: engine.ScenarioResult{
			Success:       false,
			Error:         "Target container crashed",
			ExecutedSteps: 2,
			TotalSteps:    5,
		},
	}

	err := GenerateHTMLReport(data, reportPath)
	if err != nil {
		t.Fatalf("GenerateHTMLReport failed: %v", err)
	}

	contentBytes, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("Failed to read generated report: %v", err)
	}
	content := string(contentBytes)

	// Verify HTML structure and content
	if !strings.Contains(content, "<html") {
		t.Error("Expected HTML output to contain <html> tag")
	}
	if !strings.Contains(content, "Test Scenario") {
		t.Error("Expected HTML output to contain ScenarioName")
	}
	if !strings.Contains(content, "FAILED") {
		t.Error("Expected HTML output to show FAILED status")
	}
	if !strings.Contains(content, "Target container crashed") {
		t.Error("Expected HTML output to contain the error message")
	}
}
