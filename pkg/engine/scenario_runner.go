package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/ibrahimkizilarslan/entropy/pkg/config"
)

// DefaultStepTimeout is the maximum time a single inject or probe step can take
// before being cancelled. This prevents indefinite hangs from stuck API calls.
const DefaultStepTimeout = 60 * time.Second

type ScenarioResult struct {
	Success           bool
	ProbesPassed      int
	ProbesTotal       int
	SteadyStatePassed int
	SteadyStateTotal  int
	ExecutedSteps     int
	TotalSteps        int
	Error             string
	SteadyStateError  string
}

type ScenarioRunner struct {
	config      *config.ScenarioConfig
	logCb       func(string)
	runtime     ContainerRuntime
	stopped     []string
	paused      []string
	runtimeType string
	ctx         context.Context
	cancel      context.CancelFunc
}

func NewScenarioRunner(cfg *config.ScenarioConfig, runtimeType string, logCb func(string)) *ScenarioRunner {
	if logCb == nil {
		logCb = func(string) {}
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &ScenarioRunner{
		config:      cfg,
		logCb:       logCb,
		runtimeType: runtimeType,
		ctx:         ctx,
		cancel:      cancel,
	}
}

func (r *ScenarioRunner) Run() ScenarioResult {
	res := ScenarioResult{
		TotalSteps: len(r.config.Steps),
		Success:    true,
	}

	r.logCb(fmt.Sprintf("Running Scenario: %s", r.config.Name))
	if r.config.Hypothesis != "" {
		r.logCb(fmt.Sprintf("Hypothesis: %s", r.config.Hypothesis))
	}

	if r.runtime == nil {
		dc, err := GetRuntime(r.runtimeType, nil)
		if err != nil {
			res.Success = false
			res.Error = fmt.Sprintf("failed to connect to %s: %v", r.runtimeType, err)
			return res
		}
		r.runtime = dc
		defer dc.Close()
	} else {
		// If runtime was already injected (e.g. by tests), we still want to ensure it gets closed
		defer r.runtime.Close()
	}

	// 2A: Pre-scenario steady-state check
	if len(r.config.SteadyState) > 0 {
		r.logCb("\nVerifying Pre-scenario Steady-State...")
		res.SteadyStateTotal = len(r.config.SteadyState)
		if err := EvaluateSteadyState(r.config.SteadyState); err != nil {
			res.Success = false
			res.SteadyStateError = err.Error()
			r.logCb(fmt.Sprintf("❌ Pre-scenario Steady-State check failed: %v", err))
			return res
		}
		r.logCb("✅ Pre-scenario Steady-State verified.")
	}

	for i, step := range r.config.Steps {
		// Check if the scenario has been cancelled (e.g. via Ctrl+C)
		select {
		case <-r.ctx.Done():
			res.Success = false
			res.Error = "scenario cancelled"
			return res
		default:
		}

		res.ExecutedSteps++
		r.logCb(fmt.Sprintf("\nStep %d/%d: %s", i+1, res.TotalSteps, step.Type))

		if step.Type == "wait" {
			r.logCb(fmt.Sprintf("Waiting for %ds...", step.DurationS))
			// Use a select so that wait steps can be cancelled
			select {
			case <-time.After(time.Duration(step.DurationS) * time.Second):
			case <-r.ctx.Done():
				res.Success = false
				res.Error = "scenario cancelled during wait"
				return res
			}
		} else if step.Type == "inject" {
			actionName := step.Action.Name
			r.logCb(fmt.Sprintf("Injecting %s into %s", actionName, step.Target))

			if actionName == "stop" {
				r.stopped = append(r.stopped, step.Target)
			} else if actionName == "pause" {
				r.paused = append(r.paused, step.Target)
			} else if actionName == "restart" {
				// Remove from stopped if it's restarted during the scenario
				for j, t := range r.stopped {
					if t == step.Target {
						r.stopped = append(r.stopped[:j], r.stopped[j+1:]...)
						break
					}
				}
			}

			// Create a per-step context with timeout to prevent indefinite hangs
			stepCtx, stepCancel := context.WithTimeout(r.ctx, DefaultStepTimeout)
			info, err := Dispatch(stepCtx, *step.Action, r.runtime, step.Target)
			stepCancel()

			if err != nil {
				res.Success = false
				res.Error = fmt.Sprintf("injection failed: %v", err)
				r.logCb(fmt.Sprintf("❌ %s", res.Error))
				return res
			}
			r.logCb(fmt.Sprintf("✅ Container status: %s", info.Status))
		} else if step.Type == "probe" {
			res.ProbesTotal++

			probeTarget := step.Probe.URL
			if step.Probe.Type == "tcp" {
				probeTarget = step.Probe.HostPort
			} else if step.Probe.Type == "exec" {
				probeTarget = fmt.Sprintf("exec '%s' on %s", step.Probe.Command, step.Probe.Target)
			}
			r.logCb(fmt.Sprintf("Probing %s", probeTarget))

			probeRes := RunProbeWithContext(r.ctx, step.Probe, r.runtime)
			if probeRes.Success {
				res.ProbesPassed++
				r.logCb(fmt.Sprintf("✅ %s", probeRes.Message))
			} else {
				res.Success = false
				res.Error = fmt.Sprintf("probe failed: %s", probeRes.Message)
				r.logCb(fmt.Sprintf("❌ %s", res.Error))
				return res
			}
		}
	}

	// 2A: Post-scenario steady-state check
	if len(r.config.SteadyState) > 0 {
		r.logCb("\nVerifying Post-scenario Steady-State...")
		if err := EvaluateSteadyState(r.config.SteadyState); err != nil {
			res.Success = false
			res.SteadyStateError = err.Error()
			r.logCb(fmt.Sprintf("❌ Post-scenario Steady-State check failed: %v", err))
			return res
		}
		res.SteadyStatePassed = len(r.config.SteadyState)
		r.logCb("✅ Post-scenario Steady-State verified. System recovered.")
	}

	return res
}

func (r *ScenarioRunner) RevertAll() {
	r.logCb("\n[System] Initiating graceful rollback...")
	// Cancel any in-flight steps first
	if r.cancel != nil {
		r.cancel()
	}

	if r.runtime == nil {
		return
	}

	r.runtime.CleanupAll(context.Background()) // network and resource chaos

	if r.runtimeType == "docker" {
		for _, target := range r.stopped {
			r.logCb(fmt.Sprintf("Rollback: Restarting container %s", target))
			if _, err := r.runtime.RestartContainer(context.Background(), target, 10); err != nil {
				r.logCb(fmt.Sprintf("[Error] Failed to restart container %s: %v", target, err))
			}
		}
		for _, target := range r.paused {
			r.logCb(fmt.Sprintf("Rollback: Unpausing container %s", target))
			if _, err := r.runtime.UnpauseContainer(context.Background(), target); err != nil {
				r.logCb(fmt.Sprintf("[Error] Failed to unpause container %s: %v", target, err))
			}
		}
	} else if r.runtimeType == "kubernetes" {
		r.logCb("[System] Kubernetes controllers automatically recreate deleted pods; no manual restart required.")
	}
	r.logCb("[System] Rollback complete.")
}
