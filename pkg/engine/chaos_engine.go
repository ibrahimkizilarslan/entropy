package engine

import (
	"fmt"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/ibrahimkizilarslan/entropy-cli/pkg/config"
	"github.com/ibrahimkizilarslan/entropy-cli/pkg/utils"
)

type EngineStatus struct {
	Running           bool
	Config            *config.ChaosConfig
	CycleCount        int
	DownContainers    []string
	LastEvent         *utils.EventRecord
	History           []utils.EventRecord
	LastInjectionTime time.Time
	CooldownRemaining float64
}

type ChaosEngine struct {
	config  *config.ChaosConfig
	onEvent func(utils.EventRecord)
	logger  *utils.ChaosLogger

	mu                sync.Mutex
	running           bool
	stopEvent         chan struct{}
	cycleCount        int
	downSet           map[string]bool
	lastEvent         *utils.EventRecord
	history           []utils.EventRecord
	lastInjectionTime time.Time
}

func NewChaosEngine(cfg *config.ChaosConfig, onEvent func(utils.EventRecord), logger *utils.ChaosLogger) *ChaosEngine {
	return &ChaosEngine{
		config:    cfg,
		onEvent:   onEvent,
		logger:    logger,
		downSet:   make(map[string]bool),
		stopEvent: make(chan struct{}),
	}
}

func (e *ChaosEngine) Start() error {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return fmt.Errorf("chaos engine is already running")
	}
	e.running = true
	e.stopEvent = make(chan struct{})
	e.mu.Unlock()

	go e.runLoop()
	return nil
}

func (e *ChaosEngine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running {
		close(e.stopEvent)
		e.running = false
	}
}

func (e *ChaosEngine) Status() EngineStatus {
	e.mu.Lock()
	defer e.mu.Unlock()

	cooldown := float64(e.config.Safety.Cooldown)
	var remaining float64
	if cooldown > 0 && !e.lastInjectionTime.IsZero() {
		elapsed := time.Since(e.lastInjectionTime).Seconds()
		remaining = cooldown - elapsed
		if remaining < 0 {
			remaining = 0
		}
	}

	down := make([]string, 0, len(e.downSet))
	for k := range e.downSet {
		down = append(down, k)
	}

	hist := make([]utils.EventRecord, len(e.history))
	copy(hist, e.history)

	return EngineStatus{
		Running:           e.running,
		Config:            e.config,
		CycleCount:        e.cycleCount,
		DownContainers:    down,
		LastEvent:         e.lastEvent,
		History:           hist,
		LastInjectionTime: e.lastInjectionTime,
		CooldownRemaining: remaining,
	}
}

func (e *ChaosEngine) runLoop() {
	if e.logger != nil {
		e.logger.LogStart(e.config)
	}

	dc, err := NewDockerClient(e.config.Targets)
	if err == nil {
		defer dc.Close()
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	ticksSinceLastCycle := 0

	for {
		select {
		case <-e.stopEvent:
			e.cleanup()
			return
		case <-ticker.C:
			ticksSinceLastCycle++
			if ticksSinceLastCycle >= e.config.Interval {
				ticksSinceLastCycle = 0
				if dc != nil {
					e.runCycle(dc)
				}
			}
		}
	}
}

func (e *ChaosEngine) cleanup() {
	e.mu.Lock()
	cycles := e.cycleCount
	injections := len(e.history)
	e.mu.Unlock()

	CleanupAll()

	if e.logger != nil {
		e.logger.LogStop(cycles, injections)
	}
}

func (e *ChaosEngine) formatActionName(action config.ActionSpec) string {
	res := action.Name
	if action.Name == "delay" {
		res += fmt.Sprintf(" (%dms", action.LatencyMs)
		if action.JitterMs > 0 {
			res += fmt.Sprintf("±%dms", action.JitterMs)
		}
		res += ")"
	} else if action.Name == "loss" {
		res += fmt.Sprintf(" (%d%%)", action.LossPercent)
	} else if action.Name == "limit_cpu" {
		res += fmt.Sprintf(" (%.2f CPUs)", action.CPUs)
	} else if action.Name == "limit_memory" {
		res += fmt.Sprintf(" (%dMB)", action.MemoryMB)
	}
	return res
}

func (e *ChaosEngine) runCycle(dc *DockerClient) {
	e.mu.Lock()
	e.cycleCount++
	downCount := len(e.downSet)
	lastInj := e.lastInjectionTime
	e.mu.Unlock()

	cooldown := float64(e.config.Safety.Cooldown)
	if cooldown > 0 && !lastInj.IsZero() {
		elapsed := time.Since(lastInj).Seconds()
		remaining := cooldown - elapsed
		if remaining > 0 {
			if e.logger != nil {
				e.logger.LogCooldownSkip(remaining)
			}
			return
		}
	}

	if downCount >= e.config.Safety.MaxDown {
		if e.logger != nil {
			e.mu.Lock()
			var downs []string
			for k := range e.downSet {
				downs = append(downs, k)
			}
			e.mu.Unlock()
			e.logger.LogMaxDownSkip(downs)
		}
		return
	}

	e.mu.Lock()
	var available []string
	for _, t := range e.config.Targets {
		if !e.downSet[t] {
			available = append(available, t)
		}
	}
	e.mu.Unlock()

	if len(available) == 0 {
		return
	}

	target := available[rand.IntN(len(available))]
	actionSpec := e.config.Actions[rand.IntN(len(e.config.Actions))]
	actionName := e.formatActionName(actionSpec)

	event := e.execute(dc, actionSpec, target, actionName)

	e.mu.Lock()
	e.lastInjectionTime = event.Timestamp
	if event.ResultStatus != "" && event.Error == "" {
		if actionSpec.Name == "stop" || actionSpec.Name == "pause" {
			e.downSet[target] = true
		} else if actionSpec.Name == "restart" || actionSpec.Name == "unpause" {
			delete(e.downSet, target)
		}
	}
	e.lastEvent = &event
	e.history = append(e.history, event)
	e.mu.Unlock()

	if e.logger != nil {
		e.logger.LogInjection(utils.InjectionEvent{
			Action:       actionName,
			Target:       target,
			Success:      event.Error == "",
			ResultStatus: event.ResultStatus,
			Error:        event.Error,
			DryRun:       event.DryRun,
		})
	}

	if e.onEvent != nil {
		e.onEvent(event)
	}
}

func (e *ChaosEngine) execute(dc *DockerClient, spec config.ActionSpec, target, actionName string) utils.EventRecord {
	now := time.Now().UTC()
	dryRun := e.config.Safety.DryRun

	if dryRun {
		return utils.EventRecord{
			Timestamp:    now,
			Action:       actionName,
			Target:       target,
			DryRun:       true,
			ResultStatus: "(dry-run)",
		}
	}

	info, err := Dispatch(spec, dc, target)
	if err != nil {
		return utils.EventRecord{
			Timestamp: now,
			Action:    actionName,
			Target:    target,
			DryRun:    false,
			Error:     err.Error(),
		}
	}

	return utils.EventRecord{
		Timestamp:    now,
		Action:       actionName,
		Target:       target,
		DryRun:       false,
		ResultStatus: info.Status,
	}
}
