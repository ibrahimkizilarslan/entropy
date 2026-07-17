package cli

import (
	"context"
	"os"

	"github.com/ibrahimkizilarslan/entropy/pkg/engine"
	"github.com/ibrahimkizilarslan/entropy/pkg/registry"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var registryCmd = &cobra.Command{
	Use:   "registry",
	Short: "Manage the persistent fault registry",
	Long:  `The registry tracks all active chaos injections to ensure they are properly cleaned up even if the engine crashes.`,
}

var registryListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all active (orphaned) chaos injections",
	Run: func(cmd *cobra.Command, args []string) {
		regPath := os.Getenv("ENTROPY_REGISTRY_PATH")
		reg, err := registry.Open(regPath)
		if err != nil {
			pterm.Error.Printf("Failed to open registry: %v\n", err)
			return
		}

		active := reg.ListActive()
		if len(active) == 0 {
			pterm.Success.Println("No active chaos injections found in registry.")
			return
		}

		tableData := pterm.TableData{
			{"ID", "Target", "Runtime", "Type", "Injected At"},
		}

		for _, rec := range active {
			tableData = append(tableData, []string{
				rec.ID, rec.Target, rec.Runtime, string(rec.FaultType), rec.InjectedAt.Format("2006-01-02 15:04:05"),
			})
		}

		_ = pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()
	},
}

var registryClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Manually revert all active chaos injections across all runtimes",
	Run: func(cmd *cobra.Command, args []string) {
		regPath := os.Getenv("ENTROPY_REGISTRY_PATH")
		reg, err := registry.Open(regPath)
		if err != nil {
			pterm.Error.Printf("Failed to open registry: %v\n", err)
			return
		}

		active := reg.ListActive()
		if len(active) == 0 {
			pterm.Success.Println("No active chaos injections to clear.")
			return
		}

		ctx := context.Background()
		netIface := os.Getenv("ENTROPY_NET_INTERFACE")

		// Track which runtimes we have records for to avoid instantiating unneeded ones
		needsDocker := false
		needsK8s := false
		for _, rec := range active {
			if rec.Runtime == "docker" {
				needsDocker = true
			} else if rec.Runtime == "kubernetes" {
				needsK8s = true
			}
		}

		spinner, _ := pterm.DefaultSpinner.Start("Reverting orphaned faults...")

		var results []registry.RecoveryResult

		if needsDocker {
			rt, err := engine.GetRuntime("docker", nil)
			if err == nil {
				res := reg.RecoverOrphans(ctx, "docker", nil, true, rt, netIface)
				results = append(results, res...)
				rt.Close()
			} else {
				spinner.Fail("Docker runtime error")
				pterm.Warning.Printf("Failed to initialize Docker runtime for cleanup: %v\n", err)
			}
		}

		if needsK8s {
			rt, err := engine.GetRuntime("kubernetes", nil)
			if err == nil {
				res := reg.RecoverOrphans(ctx, "kubernetes", nil, true, rt, netIface)
				results = append(results, res...)
				rt.Close()
			} else {
				spinner.Fail("Kubernetes runtime error")
				pterm.Warning.Printf("Failed to initialize Kubernetes runtime for cleanup: %v\n", err)
			}
		}

		spinner.Success("Clear operation finished.")

		var failed int
		for _, r := range results {
			if r.Err != nil {
				failed++
				pterm.Error.Printf("[%s] Failed to revert %s: %v\n", r.Target, string(r.FaultType), r.Err)
			} else {
				pterm.Success.Printf("[%s] Reverted %s\n", r.Target, string(r.FaultType))
			}
		}

		if failed > 0 {
			pterm.Warning.Printf("%d faults could not be reverted automatically. Manual intervention may be required.\n", failed)
		} else if len(results) > 0 {
			pterm.Success.Println("All orphaned faults have been successfully reverted.")
		}
	},
}

func init() {
	rootCmd.AddCommand(registryCmd)
	registryCmd.AddCommand(registryListCmd)
	registryCmd.AddCommand(registryClearCmd)
}
