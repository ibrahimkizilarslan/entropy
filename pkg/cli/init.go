package cli

import (
	"os"

	"github.com/ibrahimkizilarslan/entropy-cli/pkg/engine"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Auto-discover docker-compose.yml and generate a chaos.yaml file",
	Run: func(cmd *cobra.Command, args []string) {
		force, _ := cmd.Flags().GetBool("force")
		
		outFile := "chaos.yaml"
		if _, err := os.Stat(outFile); err == nil && !force {
			pterm.Error.Printf("'%s' already exists. Use --force to overwrite.\n", outFile)
			os.Exit(1)
		}

		services, foundPath, err := engine.DiscoverComposeServices(".")
		if err != nil {
			pterm.Error.Println(err)
			os.Exit(1)
		}

		pterm.Success.Printf("Found %s\n", foundPath)
		pterm.Success.Printf("Discovered %d services: %v\n", len(services), services)

		if err := engine.GenerateDefaultChaosConfig(services, outFile); err != nil {
			pterm.Error.Printf("Failed to write %s: %v\n", outFile, err)
			os.Exit(1)
		}

		pterm.Success.Printf("Generated chaos.yaml with all discovered services.\nYou can now run 'entropy start' to begin testing.\n")
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().BoolP("force", "f", false, "Overwrite chaos.yaml if it exists")
}
