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

// SafetyOverrides holds CLI-flag overrides applied on top of the safety
// values loaded from the config file. Each field is a pointer so that "flag
// not passed" (nil, keep the config file's value) can be distinguished from
// "flag passed with a zero value" (non-nil, pointing at the zero value).
type SafetyOverrides struct {
	DryRun   *bool
	MaxDown  *int
	Cooldown *int
}

// DaemonOptions bundles the parameters needed to run the chaos daemon.
type DaemonOptions struct {
	ConfigPath  string
	RuntimeType string
	LogFormat   string
	Overrides   SafetyOverrides
}

// RunDaemon loads the config at opts.ConfigPath, starts the chaos engine,
// and blocks until an interrupt/terminate signal is received.
func RunDaemon(opts DaemonOptions) error {
	cfg, err := config.LoadConfig(opts.ConfigPath)
	if err != nil {
		return err
	}
	applySafetyOverrides(cfg, opts.Overrides)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	return runDaemonLoop(cfg, opts.RuntimeType, opts.LogFormat, opts.ConfigPath, "", stop)
}

// applySafetyOverrides applies CLI flag overrides (when non-nil) on top of
// the values loaded from the config file.
func applySafetyOverrides(cfg *config.ChaosConfig, overrides SafetyOverrides) {
	if overrides.DryRun != nil {
		cfg.Safety.DryRun = *overrides.DryRun
	}
	if overrides.MaxDown != nil {
		cfg.Safety.MaxDown = *overrides.MaxDown
	}
	if overrides.Cooldown != nil {
		cfg.Safety.Cooldown = *overrides.Cooldown
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
