package cli

import (
	"os"

	"github.com/ibrahimkizilarslan/entropy-cli/pkg/engine"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var topologyCmd = &cobra.Command{
	Use:   "topology",
	Short: "Visualize the cluster topology and blast radius",
	Long: `Reads your docker-compose topology and generates a visual tree representing 
networks and dependencies. This helps you understand the 'blast radius'—which 
services will be affected if you inject chaos into a specific network or container.`,
	Run: func(cmd *cobra.Command, args []string) {
		cwd, err := os.Getwd()
		if err != nil {
			pterm.Error.Printf("Failed to get current directory: %v\n", err)
			return
		}

		tree, err := engine.GenerateTopologyTree(cwd)
		if err != nil {
			pterm.Error.Printf("Topology generation failed: %v\n", err)
			return
		}

		pterm.DefaultTree.WithRoot(*tree).Render()
		
		pterm.Info.Println("💡 Tip: Containers in the same network share the same blast radius for network chaos (delay/loss).")
	},
}

func init() {
	rootCmd.AddCommand(topologyCmd)
}
