package cli

import (
	"fmt"
	"os"
	"os/exec"


	"github.com/ibrahimkizilarslan/entropy/pkg/config"
	"github.com/ibrahimkizilarslan/entropy/pkg/engine"
	"github.com/ibrahimkizilarslan/entropy/pkg/utils"
	"github.com/ibrahimkizilarslan/entropy/pkg/worker"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the chaos engine. Use --detach / -d to run in the background.",
	Run: func(cmd *cobra.Command, args []string) {
		configPath, _ := cmd.Flags().GetString("config")
		detach, _ := cmd.Flags().GetBool("detach")

		cfg, err := config.LoadConfig(configPath)
		if err != nil {
			pterm.Error.Println(err)
			os.Exit(1)
		}

		if cmd.Flags().Changed("dry-run") {
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			cfg.Safety.DryRun = dryRun
		}
		if cmd.Flags().Changed("max-down") {
			maxDown, _ := cmd.Flags().GetInt("max-down")
			cfg.Safety.MaxDown = maxDown
		}
		if cmd.Flags().Changed("cooldown") {
			cooldown, _ := cmd.Flags().GetInt("cooldown")
			cfg.Safety.Cooldown = cooldown
		}

		state := utils.NewStateManager("")
		if pid := state.RunningPID(); pid != nil {
			pterm.Error.Printf("Chaos engine is already running (PID %d).\nRun 'entropy stop' first.\n", *pid)
			os.Exit(1)
		}

		if detach {
			_ = state.EnsureDir()

			cmdArgs := []string{"run-worker", "--config", configPath, "--max-down", fmt.Sprintf("%d", cfg.Safety.MaxDown), "--cooldown", fmt.Sprintf("%d", cfg.Safety.Cooldown)}
			if cfg.Safety.DryRun {
				cmdArgs = append(cmdArgs, "--dry-run")
			}

			daemonCmd := exec.Command(os.Args[0], cmdArgs...)
			logFile, err := os.OpenFile(state.LogFile(), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
			if err != nil {
				pterm.Error.Println(err)
				os.Exit(1)
			}
			daemonCmd.Stdout = logFile
			daemonCmd.Stderr = logFile
			setupDaemon(daemonCmd)

			if err := daemonCmd.Start(); err != nil {
				logFile.Close()
				pterm.Error.Println(err)
				os.Exit(1)
			}
			logFile.Close()
			pterm.Success.Printf("Chaos Engine Started (background) PID: %d\n", daemonCmd.Process.Pid)
		} else {
			dryRunOpt := cfg.Safety.DryRun
			maxDownOpt := cfg.Safety.MaxDown
			cooldownOpt := cfg.Safety.Cooldown
			if err := worker.RunDaemon(configPath, &dryRunOpt, &maxDownOpt, &cooldownOpt); err != nil {
				pterm.Error.Println(err)
				os.Exit(1)
			}
		}
	},
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop a running background chaos engine",
	Run: func(cmd *cobra.Command, args []string) {
		state := utils.NewStateManager("")
		pid := state.RunningPID()

		if pid == nil {
			_ = state.Clear()
			pterm.Error.Println("No chaos engine is running.")
			os.Exit(1)
		}

		pterm.Info.Printf("Sending SIGTERM to PID %d...\n", *pid)
		proc, err := os.FindProcess(*pid)
		if err != nil {
			pterm.Warning.Printf("Could not find process %d: %v\n", *pid, err)
		} else if err := proc.Signal(getTerminateSignal()); err != nil {
			pterm.Warning.Printf("Could not send signal to PID %d: %v\n", *pid, err)
		}

		_ = state.Clear()
		pterm.Success.Printf("Chaos engine (PID %d) has been stopped.\n", *pid)
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the status of the chaos engine",
	Run: func(cmd *cobra.Command, args []string) {
		state := utils.NewStateManager("")
		data, err := state.Read()
		if err != nil || data == nil {
			pterm.Info.Println("No chaos engine is currently running.")
			return
		}

		pterm.DefaultBasicText.Printf("PID: %d\nConfig: %s\nMode: %v\nCycles: %d\nDown: %v\nCooldown Remaining: %.1f\n",
			data.PID, data.ConfigPath, data.DryRun, data.CycleCount, data.DownContainers, data.CooldownRemaining)
	},
}

var injectCmd = &cobra.Command{
	Use:   "inject [action] [target]",
	Short: "Manually inject a single chaos action into a target container",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		action := args[0]
		target := args[1]

		skipVal, _ := cmd.Flags().GetBool("skip-validation")
		configPath, _ := cmd.Flags().GetString("config")
		latency, _ := cmd.Flags().GetInt("latency")
		jitter, _ := cmd.Flags().GetInt("jitter")
		loss, _ := cmd.Flags().GetInt("loss")
		cpu, _ := cmd.Flags().GetFloat64("cpu")
		memory, _ := cmd.Flags().GetInt("memory")
		duration, _ := cmd.Flags().GetInt("duration")

		spec := config.ActionSpec{
			Name:        action,
			LatencyMs:   latency,
			JitterMs:    jitter,
			LossPercent: loss,
			CPUs:        cpu,
			MemoryMB:    memory,
		}
		if cmd.Flags().Changed("duration") {
			spec.Duration = &duration
		}

		var allowedTargets []string
		if !skipVal {
			cfg, err := config.LoadConfig(configPath)
			if err == nil {
				allowedTargets = cfg.Targets
				found := false
				for _, t := range allowedTargets {
					if t == target {
						found = true
						break
					}
				}
				if !found {
					pterm.Error.Printf("'%s' is not in the targets list of %s.\n", target, configPath)
					os.Exit(1)
				}
			}
		}

		dc, err := engine.NewDockerClient(allowedTargets)
		if err != nil {
			pterm.Error.Println(err)
			os.Exit(1)
		}
		defer dc.Close()

		info, err := engine.Dispatch(spec, dc, target)
		if err != nil {
			pterm.Error.Println(err)
			os.Exit(1)
		}

		pterm.Success.Printf("Injected %s into %s (Status: %s)\n", action, target, info.Status)
	},
}

func init() {
	rootCmd.AddCommand(startCmd)
	startCmd.Flags().StringP("config", "c", "chaos.yaml", "Path to chaos config file")
	startCmd.Flags().BoolP("detach", "d", false, "Run engine in background")
	startCmd.Flags().Bool("dry-run", false, "Override config: log actions without executing them")
	startCmd.Flags().Int("max-down", 1, "Override config: max containers stopped simultaneously")
	startCmd.Flags().Int("cooldown", 0, "Override config: min seconds between injections")

	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(cleanupCmd)

	rootCmd.AddCommand(injectCmd)
	injectCmd.Flags().StringP("config", "c", "chaos.yaml", "Path to chaos config file")
	injectCmd.Flags().Bool("skip-validation", false, "Bypass allow-list check")
	injectCmd.Flags().Int("latency", 300, "Latency in ms")
	injectCmd.Flags().Int("jitter", 0, "Jitter in ms")
	injectCmd.Flags().Int("loss", 20, "Packet loss %")
	injectCmd.Flags().Float64("cpu", 0.25, "CPU quota")
	injectCmd.Flags().Int("memory", 128, "Memory limit MB")
	injectCmd.Flags().Int("duration", 0, "Auto-restore after N seconds")
}

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Emergency cleanup: revert all active faults",
	Run: func(cmd *cobra.Command, args []string) {
		engine.CleanupAll()
		pterm.Success.Println("Emergency cleanup completed. All network and resource constraints removed.")
	},
}
