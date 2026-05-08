package cli

import (
	"fmt"
	"os"

	"github.com/ibrahimkizilarslan/entropy-cli/pkg/engine"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var dockerCmd = &cobra.Command{
	Use:   "docker",
	Short: "Docker utility commands",
}

var dockerListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all running Docker containers",
	Run: func(cmd *cobra.Command, args []string) {
		dc, err := engine.NewDockerClient(nil)
		if err != nil {
			pterm.Error.Println(err)
			os.Exit(1)
		}
		defer dc.Close()

		containers, err := dc.ListContainers(true)
		if err != nil {
			pterm.Error.Println(err)
			os.Exit(1)
		}

		tableData := pterm.TableData{
			{"ID", "Name", "Image", "Status", "Ports"},
		}

		for _, c := range containers {
			tableData = append(tableData, []string{
				c.ID, c.Name, c.Image, c.Status, fmt.Sprintf("%v", c.Ports),
			})
		}

		pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()
	},
}

func init() {
	rootCmd.AddCommand(dockerCmd)
	dockerCmd.AddCommand(dockerListCmd)
}


