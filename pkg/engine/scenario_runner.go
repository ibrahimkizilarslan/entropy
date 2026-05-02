package engine

import (
	"fmt"
	"time"

	"github.com/ibrahimkizilarslan/entropy-cli/pkg/config"
)

type ScenarioResult struct {
	Success       bool
	ProbesPassed  int
	ProbesTotal   int
	ExecutedSteps int
	TotalSteps    int
	Error         string
}

type ScenarioRunner struct {
	config *config.ScenarioConfig
	logCb  func(string)
}

func NewScenarioRunner(cfg *config.ScenarioConfig, logCb func(string)) *ScenarioRunner {
	if logCb == nil {
		logCb = func(string) {}
	}
	return &ScenarioRunner{
		config: cfg,
		logCb:  logCb,
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

	dc, err := NewDockerClient(nil)
	if err != nil {
		res.Success = false
		res.Error = fmt.Sprintf("failed to connect to docker: %v", err)
		return res
	}
	defer dc.Close()

	for i, step := range r.config.Steps {
		res.ExecutedSteps++
		r.logCb(fmt.Sprintf("\nStep %d/%d: %s", i+1, res.TotalSteps, step.Type))

		if step.Type == "wait" {
			r.logCb(fmt.Sprintf("Waiting for %ds...", step.DurationS))
			time.Sleep(time.Duration(step.DurationS) * time.Second)
		} else if step.Type == "inject" {
			actionName := step.Action.Name
			r.logCb(fmt.Sprintf("Injecting %s into %s", actionName, step.Target))
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
			r.logCb(fmt.Sprintf("Probing %s", step.Probe.URL))
			probeRes := RunProbe(step.Probe)
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

	return res
}
