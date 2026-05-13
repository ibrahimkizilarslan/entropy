package engine

import (
	"fmt"
	"time"

	"github.com/ibrahimkizilarslan/entropy/pkg/config"
)

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
	config  *config.ScenarioConfig
	logCb   func(string)
	runtime ContainerRuntime
	stopped     []string
	paused      []string
	runtimeType string
}

func NewScenarioRunner(cfg *config.ScenarioConfig, runtimeType string, logCb func(string)) *ScenarioRunner {
	if logCb == nil {
		logCb = func(string) {}
	}
	return &ScenarioRunner{
		config:      cfg,
		logCb:       logCb,
		runtimeType: runtimeType,
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

	dc, err := GetRuntime(r.runtimeType, nil)
	if err != nil {
		res.Success = false
		res.Error = fmt.Sprintf("failed to connect to docker: %v", err)
		return res
	}
	r.runtime = dc
	defer dc.Close()

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
		res.ExecutedSteps++
		r.logCb(fmt.Sprintf("\nStep %d/%d: %s", i+1, res.TotalSteps, step.Type))

		if step.Type == "wait" {
			r.logCb(fmt.Sprintf("Waiting for %ds...", step.DurationS))
			time.Sleep(time.Duration(step.DurationS) * time.Second)
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

			info, err := Dispatch(*step.Action, dc, step.Target)
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

			probeRes := RunProbe(step.Probe, r.runtime)
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
	CleanupAll() // network and resource chaos

	if r.runtime == nil {
		return
	}

	for _, target := range r.stopped {
		r.logCb(fmt.Sprintf("Rollback: Restarting container %s", target))
		_, _ = r.runtime.RestartContainer(target, 10)
	}
	for _, target := range r.paused {
		r.logCb(fmt.Sprintf("Rollback: Unpausing container %s", target))
		_, _ = r.runtime.UnpauseContainer(target)
	}
	r.logCb("[System] Rollback complete.")
}
