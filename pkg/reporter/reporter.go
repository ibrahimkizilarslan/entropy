package reporter

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"html/template"
	"os"

	"github.com/ibrahimkizilarslan/entropy/pkg/engine"
)

type ReportData struct {
	ScenarioName   string
	Hypothesis     string
	Result         engine.ScenarioResult
	Timestamp      string
	EntropyVersion string
}

//go:embed report.gohtml
var htmlTemplate string

func GenerateHTMLReport(data ReportData, filePath string) error {
	tmpl, err := template.New("report").Parse(htmlTemplate)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return err
	}

	return os.WriteFile(filePath, buf.Bytes(), 0644)
}

func GenerateJSONReport(data ReportData, filePath string) error {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, b, 0644)
}
