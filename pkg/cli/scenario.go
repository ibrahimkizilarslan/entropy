package cli

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ibrahimkizilarslan/entropy/pkg/config"
	"github.com/ibrahimkizilarslan/entropy/pkg/engine"
	"github.com/ibrahimkizilarslan/entropy/pkg/reporter"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// Version can be set at build time via ldflags:
//
//	go build -ldflags "-X github.com/ibrahimkizilarslan/entropy/pkg/cli.Version=v1.0.0"
var Version = "dev"

var (
	reportFormat string
	reportOutput string
)

var scenarioCmd = &cobra.Command{
	Use:   "scenario",
	Short: "Run deterministic chaos scenarios",
}

var scenarioRunCmd = &cobra.Command{
	Use:   "run [file]",
	Short: "Run a deterministic chaos scenario and verify hypotheses",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		scenarioPath := args[0]
		cfg, err := config.LoadScenario(scenarioPath)
		if err != nil {
			pterm.Error.Println(err)
			os.Exit(1)
		}

		pterm.Println()
		runner := engine.NewScenarioRunner(cfg, runtimeType, func(msg string) {
			pterm.Printf("  %s\n", msg)
		})

		// Always revert injected faults when scenario finishes, regardless of success or failure.
		// This prevents orphaned chaos faults from persisting after a failed scenario.
		defer runner.RevertAll()

		// Setup signal trap for graceful rollback
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sigChan
			pterm.Warning.Println("\n[!] Received interrupt signal. Aborting scenario...")
			runner.RevertAll()
			os.Exit(130)
		}()

		result := runner.Run()

		pterm.Println()
		if result.Success {
			pterm.Success.Println("Hypothesis Confirmed")
		} else {
			pterm.Error.Println("Hypothesis Failed")
		}

		pterm.Printf("Probes passed: %d / %d\n", result.ProbesPassed, result.ProbesTotal)
		pterm.Printf("Steps run:     %d / %d\n", result.ExecutedSteps, result.TotalSteps)

		if result.Error != "" {
			pterm.Error.Printf("Error: %s\n", result.Error)
		}

		if reportOutput != "" {
			reportData := reporter.ReportData{
				ScenarioName:   cfg.Name,
				Hypothesis:     cfg.Hypothesis,
				Result:         result,
				Timestamp:      time.Now().Format(time.RFC1123),
				EntropyVersion: Version,
			}

			var err error
			if reportFormat == "html" {
				err = reporter.GenerateHTMLReport(reportData, reportOutput)
			} else if reportFormat == "json" {
				err = reporter.GenerateJSONReport(reportData, reportOutput)
			}

			if err != nil {
				pterm.Error.Printf("Failed to generate report: %v\n", err)
			} else {
				pterm.Success.Printf("Report generated: %s\n", reportOutput)
			}
		}

		pterm.Println()

		if !result.Success {
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(scenarioCmd)
	scenarioCmd.AddCommand(scenarioRunCmd)

	scenarioRunCmd.Flags().StringVar(&reportFormat, "report", "html", "Report format (html, json)")
	scenarioRunCmd.Flags().StringVar(&reportOutput, "output", "", "Output path for the report")
}
