package cli

import (
	"fmt"
	"os"

	"github.com/ibrahimkizilarslan/entropy/pkg/config"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var validateCmd = &cobra.Command{
	Use:   "validate [file]",
	Short: "Validate a chaos configuration file without executing it",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		configPath := args[0]
		
		pterm.Info.Printf("Validating config file: %s\n", configPath)
		
		cfg, err := config.LoadConfig(configPath)
		if err != nil {
			pterm.Error.Printf("Validation failed: %v\n", err)
			os.Exit(1)
		}

		if err := cfg.Validate(); err != nil {
			pterm.Error.Printf("Config is invalid: %v\n", err)
			os.Exit(1)
		}

		pterm.Success.Println("Configuration is valid.")
		
		fmt.Printf("\nSummary:\n")
		fmt.Printf("  Targets:  %d\n", len(cfg.Targets))
		fmt.Printf("  Actions:  %d\n", len(cfg.Actions))
		fmt.Printf("  Interval: %ds\n", cfg.Interval)
		fmt.Printf("  Safety:   MaxDown=%d, Cooldown=%ds\n", cfg.Safety.MaxDown, cfg.Safety.Cooldown)
	},
}

func init() {
	rootCmd.AddCommand(validateCmd)
}
