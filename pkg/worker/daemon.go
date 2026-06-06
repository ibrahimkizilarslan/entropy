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

func RunDaemon(configPath string, runtimeType string, logFormat string, dryRun *bool, maxDown *int, cooldown *int) error {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return err
	}

	if dryRun != nil {
		cfg.Safety.DryRun = *dryRun
	}
	if maxDown != nil {
		cfg.Safety.MaxDown = *maxDown
	}
	if cooldown != nil {
		cfg.Safety.Cooldown = *cooldown
	}

	state := utils.NewStateManager("")
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

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	if err := chaosEngine.Start(); err != nil {
		return err
	}

	<-c
	chaosEngine.Stop()
	if err := state.Clear(); err != nil {
		logger.LogError("Failed to clear state file on exit: " + err.Error())
	}
	return nil
}
