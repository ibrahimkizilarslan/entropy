package cli

import (
	"os"

	"github.com/ibrahimkizilarslan/entropy/pkg/worker"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var runWorkerCmd = &cobra.Command{
	Use:    "run-worker",
	Short:  "Internal command to run the chaos daemon",
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		configPath, _ := cmd.Flags().GetString("config")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		maxDown, _ := cmd.Flags().GetInt("max-down")
		cooldown, _ := cmd.Flags().GetInt("cooldown")
		logFormat, _ := cmd.Flags().GetString("log-format")
		runtime, _ := cmd.Flags().GetString("runtime")

		opts := worker.DaemonOptions{
			ConfigPath:  configPath,
			RuntimeType: runtime,
			LogFormat:   logFormat,
			Overrides: worker.SafetyOverrides{
				DryRun:   &dryRun,
				MaxDown:  &maxDown,
				Cooldown: &cooldown,
			},
		}
		if err := worker.RunDaemon(opts); err != nil {
			pterm.Error.Println(err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(runWorkerCmd)
	runWorkerCmd.Flags().StringP("config", "c", "chaos.yaml", "Path to chaos config file")
	runWorkerCmd.Flags().Bool("dry-run", false, "Override config: log actions without executing them")
	runWorkerCmd.Flags().Int("max-down", 1, "Override config: max containers stopped simultaneously")
	runWorkerCmd.Flags().Int("cooldown", 0, "Override config: min seconds between injections")
	runWorkerCmd.Flags().String("runtime", "docker", "Container runtime to use")
	runWorkerCmd.Flags().String("log-format", "text", "Log format: text or json")
}
