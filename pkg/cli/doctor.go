package cli

import (
	"fmt"
	"os"

	"github.com/ibrahimkizilarslan/entropy/pkg/engine"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Analyze your docker-compose topology for enterprise resilience and anti-patterns",
	Long: `Doctor scans your local docker-compose configuration and analyzes it against 
enterprise-level resilience rules (SPOF, Resource Limits, Recovery, Observability, Security).
It helps you identify weak points before running chaos experiments.`,
	Run: func(cmd *cobra.Command, args []string) {
		pterm.Info.Println("Starting Entropy Doctor analysis...")

		cwd, err := os.Getwd()
		if err != nil {
			pterm.Error.Printf("Failed to get current directory: %v\n", err)
			return
		}

		results, err := engine.AnalyzeTopology(cwd)
		if err != nil {
			pterm.Error.Printf("Analysis failed: %v\n", err)
			return
		}

		if len(results) == 0 {
			pterm.Success.Println("No services found to analyze.")
			return
		}

		totalIssues := 0
		criticalIssues := 0

		for _, result := range results {
			if len(result.Issues) == 0 {
				pterm.Success.Printf("Service '%s' looks healthy! ✅\n", result.ServiceName)
				continue
			}

			pterm.DefaultHeader.WithFullWidth().WithBackgroundStyle(pterm.NewStyle(pterm.BgDarkGray)).Printf("Service: %s", result.ServiceName)

			for _, issue := range result.Issues {
				totalIssues++

				prefix := ""
				switch issue.Severity {
				case "CRITICAL":
					criticalIssues++
					prefix = pterm.Red(" [CRITICAL] ")
				case "WARNING":
					prefix = pterm.Yellow(" [WARNING] ")
				default:
					prefix = pterm.Gray(" [INFO] ")
				}

				category := pterm.Cyan(fmt.Sprintf("[%s]", issue.Category))
				pterm.Printf("%s %s %s\n", prefix, category, issue.Message)
			}
			fmt.Println()
		}

		fmt.Println()
		if totalIssues == 0 {
			pterm.Success.Println("🎉 Amazing! Zero resilience issues found in your topology. You are production-ready!")
		} else {
			summaryBox := pterm.DefaultBox.WithTitle("Analysis Summary").WithTitleTopCenter()
			summaryText := fmt.Sprintf("Total Services Scanned: %d\nTotal Issues Found: %d\nCritical Issues: %d",
				len(results), totalIssues, criticalIssues)

			if criticalIssues > 0 {
				pterm.Error.Println(summaryBox.Sprint(summaryText))
				pterm.Warning.Println("Fix the critical issues before running chaos scenarios in production.")
			} else {
				pterm.Warning.Println(summaryBox.Sprint(summaryText))
				pterm.Info.Println("Consider fixing the warnings to improve your system's resilience.")
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
