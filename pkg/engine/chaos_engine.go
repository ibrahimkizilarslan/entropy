package engine

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ibrahimkizilarslan/entropy/pkg/config"
	"github.com/ibrahimkizilarslan/entropy/pkg/registry"
	"github.com/ibrahimkizilarslan/entropy/pkg/utils"
)

// MaxHistorySize limits the number of event records kept in memory.
// This prevents unbounded memory growth during long-running daemon sessions.
const MaxHistorySize = 1000

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
	runtimeType       string

	// cycleInFlight and cycleWG coordinate the async execution of runCycle
	// (see startCycleAsync). cycleInFlight prevents overlapping cycles from
	// racing on the cooldown/max_down safety checks; cycleWG lets runLoop
	// wait for an in-flight cycle to finish before cleanup runs, so cleanup
	// never races with a cycle that's still injecting/reverting chaos.
	cycleInFlight atomic.Bool
	cycleWG       sync.WaitGroup
}

func NewChaosEngine(cfg *config.ChaosConfig, runtimeType string, onEvent func(utils.EventRecord), logger *utils.ChaosLogger) *ChaosEngine {
	return &ChaosEngine{
		config:      cfg,
		runtimeType: runtimeType,
		onEvent:     onEvent,
		logger:      logger,
		downSet:     make(map[string]bool),
		stopEvent:   make(chan struct{}),
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
	var runtime ContainerRuntime

	// Prevent silent daemon death: recover from unexpected panics in the background goroutine
	defer func() {
		if r := recover(); r != nil {
			if e.logger != nil {
				e.logger.LogError(fmt.Sprintf("PANIC recovered in chaos engine: %v", r))
			}
			e.mu.Lock()
			e.running = false
			e.mu.Unlock()
			e.cleanup(runtime)
		}
	}()

	if e.logger != nil {
		e.logger.LogStart(e.config)
	}

	rt, err := GetRuntime(e.runtimeType, e.config.Targets)
	if err == nil {
		runtime = rt
		defer runtime.Close()
	}

	// Create a cancellable context tied to the engine's stop signal
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Open the persistent fault registry and attach it to the runtime
	registryPath := os.Getenv("ENTROPY_REGISTRY_PATH")
	reg, regErr := registry.Open(registryPath)
	if regErr != nil {
		if e.logger != nil {
			e.logger.LogError(fmt.Sprintf("failed to open fault registry: %v — crash-recovery disabled", regErr))
		}
	} else {
		// Wire registry into the runtime's chaos managers
		type registryAware interface {
			SetRegistry(r *registry.FaultRegistry)
		}
		if ra, ok := runtime.(registryAware); ok {
			ra.SetRegistry(reg)
		}

		// Recover any orphaned chaos from a previous crash before starting fresh
		if runtime != nil {
			allowedTargets := e.config.Targets
			netIface := os.Getenv("ENTROPY_NET_INTERFACE")
			reg.RecoverOrphans(ctx, e.runtimeType, allowedTargets, false, runtime, netIface)
			// Start background watcher to sweep expired faults that timers missed
			reg.StartExpiryWatcher(ctx, e.runtimeType, runtime, netIface, 30*time.Second)
		}
	}


	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	ticksSinceLastCycle := 0

	for {
		select {
		case <-e.stopEvent:
			// Cancel first so any in-flight cycle's Docker/K8s API call
			// unwinds quickly instead of running to completion, then wait
			// for it to actually finish before cleanup — otherwise cleanup
			// could race with a cycle that's still injecting or reverting
			// chaos, leaving inconsistent state.
			cancel()
			e.cycleWG.Wait()
			e.cleanup(runtime)
			return
		case <-ticker.C:
			ticksSinceLastCycle++
			if ticksSinceLastCycle >= e.config.Interval {
				ticksSinceLastCycle = 0
				if runtime != nil {
					e.startCycleAsync(ctx, runtime)
				}
			}
		}
	}
}

// startCycleAsync runs runCycle in a background goroutine so a slow
// injection (e.g. a stalled Docker/K8s API call) never blocks runLoop's
// select from observing e.stopEvent. If a previous cycle is still in
// flight, this tick is skipped rather than overlapping concurrent cycles,
// which could race on the cooldown/max_down safety checks in runCycle.
func (e *ChaosEngine) startCycleAsync(ctx context.Context, runtime ContainerRuntime) {
	if !e.cycleInFlight.CompareAndSwap(false, true) {
		return
	}
	e.cycleWG.Add(1)
	go func() {
		defer e.cycleWG.Done()
		defer e.cycleInFlight.Store(false)
		// runLoop's own recover() only protects its own goroutine — it
		// cannot see a panic in this one. Recover here too so a panic in
		// runCycle (e.g. an unexpected nil from a runtime response) logs
		// and lets the daemon keep running, instead of silently crashing
		// the whole process.
		//
		// Deliberately just log rather than also calling e.Stop(): if the
		// panic happened while runCycle held e.mu (its Lock/Unlock pairs
		// aren't deferred), Stop()'s own e.mu.Lock() would deadlock waiting
		// on a mutex whose owner is gone.
		defer func() {
			if r := recover(); r != nil {
				if e.logger != nil {
					e.logger.LogError(fmt.Sprintf("PANIC recovered in chaos cycle: %v", r))
				}
			}
		}()
		e.runCycle(ctx, runtime)
	}()
}

func (e *ChaosEngine) cleanup(runtime ContainerRuntime) {
	e.mu.Lock()
	cycles := e.cycleCount
	injections := len(e.history)
	e.mu.Unlock()

	if runtime != nil {
		runtime.CleanupAll(context.Background())
	}

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

func (e *ChaosEngine) runCycle(ctx context.Context, runtime ContainerRuntime) {
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

	event := e.execute(ctx, runtime, actionSpec, target, actionName)

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
	if len(e.history) > MaxHistorySize {
		// Prevent memory leak by creating a new slice instead of just slicing
		// which would keep the underlying ever-growing array in memory.
		trimmed := make([]utils.EventRecord, MaxHistorySize)
		copy(trimmed, e.history[len(e.history)-MaxHistorySize:])
		e.history = trimmed
	}
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

func (e *ChaosEngine) execute(ctx context.Context, runtime ContainerRuntime, spec config.ActionSpec, target, actionName string) utils.EventRecord {
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

	info, err := Dispatch(ctx, spec, runtime, target)
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
