package worker

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ibrahimkizilarslan/entropy/pkg/config"
	"github.com/ibrahimkizilarslan/entropy/pkg/engine"
	"github.com/ibrahimkizilarslan/entropy/pkg/utils"
)

// RunDaemon loads the config at configPath, starts the chaos engine, and
// blocks until an interrupt/terminate signal is received.
func RunDaemon(configPath string, runtimeType string, logFormat string, dryRun *bool, maxDown *int, cooldown *int) error {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return err
	}
	applySafetyOverrides(cfg, dryRun, maxDown, cooldown)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	return runDaemonLoop(cfg, runtimeType, logFormat, configPath, "", stop)
}

// applySafetyOverrides applies CLI flag overrides (when non-nil) on top of
// the values loaded from the config file.
func applySafetyOverrides(cfg *config.ChaosConfig, dryRun *bool, maxDown *int, cooldown *int) {
	if dryRun != nil {
		cfg.Safety.DryRun = *dryRun
	}
	if maxDown != nil {
		cfg.Safety.MaxDown = *maxDown
	}
	if cooldown != nil {
		cfg.Safety.Cooldown = *cooldown
	}
}

// runDaemonLoop contains the daemon's core lifecycle: wire up state/logging,
// start the chaos engine, wait for a stop signal, then shut down cleanly.
//
// stateDir and stop are injectable so this can be exercised in tests without
// touching the real CWD-relative .entropy directory or depending on OS
// signal delivery. stateDir="" uses the default (CWD-relative) directory,
// matching RunDaemon's production behavior.
func runDaemonLoop(cfg *config.ChaosConfig, runtimeType, logFormat, configPath, stateDir string, stop <-chan os.Signal) error {
	state := utils.NewStateManager(stateDir)
	logger, err := utils.NewChaosLogger(state.LogFile(), logFormat)
	if err != nil {
		return err
	}
	defer logger.Close()

	myPid := os.Getpid()
	startedAt := time.Now().UTC()

	var chaosEngine *engine.ChaosEngine

	onEvent := func(e utils.EventRecord) {
		status := chaosEngine.Status()
		err := state.Write(&utils.EngineState{
			PID:               myPid,
			StartedAt:         startedAt,
			ConfigPath:        configPath,
			DryRun:            cfg.Safety.DryRun,
			CycleCount:        status.CycleCount,
			DownContainers:    status.DownContainers,
			CooldownRemaining: status.CooldownRemaining,
			CooldownTotal:     cfg.Safety.Cooldown,
			LastEvent:         status.LastEvent,
			History:           status.History,
		})
		if err != nil {
			logger.LogError("Failed to write state file: " + err.Error())
		}
	}

	chaosEngine = engine.NewChaosEngine(cfg, runtimeType, onEvent, logger)

	err = state.Write(&utils.EngineState{
		PID:               myPid,
		StartedAt:         startedAt,
		ConfigPath:        configPath,
		DryRun:            cfg.Safety.DryRun,
		CycleCount:        0,
		DownContainers:    []string{},
		CooldownRemaining: 0,
		CooldownTotal:     cfg.Safety.Cooldown,
		LastEvent:         nil,
		History:           []utils.EventRecord{},
	})
	if err != nil {
		logger.LogError("Failed to initialize state file: " + err.Error())
	}

	if err := chaosEngine.Start(); err != nil {
		return err
	}

	<-stop
	chaosEngine.Stop()
	if err := state.Clear(); err != nil {
		logger.LogError("Failed to clear state file on exit: " + err.Error())
	}
	return nil
}
