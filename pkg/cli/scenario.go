package cli

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/ibrahimkizilarslan/entropy-cli/pkg/config"
	"github.com/ibrahimkizilarslan/entropy-cli/pkg/engine"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
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
		runner := engine.NewScenarioRunner(cfg, func(msg string) {
			pterm.Printf("  %s\n", msg)
		})

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
		pterm.Println()
		
		if !result.Success {
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(scenarioCmd)
	scenarioCmd.AddCommand(scenarioRunCmd)
}
