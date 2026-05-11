package config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	ValidActions    = []string{"stop", "restart", "pause", "delay", "loss", "limit_cpu", "limit_memory"}
	NetworkActions  = []string{"delay", "loss"}
	ResourceActions = []string{"limit_cpu", "limit_memory"}
)

type ActionSpec struct {
	Name        string  `yaml:"name"`
	LatencyMs   int     `yaml:"latency_ms,omitempty"`
	JitterMs    int     `yaml:"jitter_ms,omitempty"`
	LossPercent int     `yaml:"loss_percent,omitempty"`
	CPUs        float64 `yaml:"cpus,omitempty"`
	MemoryMB    int     `yaml:"memory_mb,omitempty"`
	Duration    *int    `yaml:"duration,omitempty"`
}

func (a *ActionSpec) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		a.Name = value.Value
		return nil
	}
	type plain ActionSpec
	if err := value.Decode((*plain)(a)); err != nil {
		return err
	}
	for i := 0; i < len(value.Content); i += 2 {
		if value.Content[i].Value == "percent" && a.LossPercent == 0 {
			_ = value.Content[i+1].Decode(&a.LossPercent)
		}
	}
	return nil
}

func (a *ActionSpec) IsNetwork() bool {
	for _, na := range NetworkActions {
		if a.Name == na {
			return true
		}
	}
	return false
}

func (a *ActionSpec) IsResource() bool {
	for _, ra := range ResourceActions {
		if a.Name == ra {
			return true
		}
	}
	return false
}

type SafetyConfig struct {
	MaxDown  int  `yaml:"max_down"`
	Cooldown int  `yaml:"cooldown"`
	DryRun   bool `yaml:"dry_run"`
}

type ChaosConfig struct {
	Interval int          `yaml:"interval"`
	Targets  []string     `yaml:"targets"`
	Actions  []ActionSpec `yaml:"actions"`
	Safety   SafetyConfig `yaml:"safety"`
}

func (c *ChaosConfig) Validate() error {
	if c.Interval < 1 {
		return fmt.Errorf("interval must be >= 1 second")
	}
	if len(c.Targets) == 0 {
		return fmt.Errorf("targets must contain at least one container name")
	}
	if len(c.Actions) == 0 {
		return fmt.Errorf("actions must contain at least one action")
	}
	for _, a := range c.Actions {
		valid := false
		for _, va := range ValidActions {
			if a.Name == va {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("unknown action: %s", a.Name)
		}
	}
	if c.Safety.MaxDown < 1 {
		return fmt.Errorf("safety.max_down must be >= 1")
	}
	if c.Safety.Cooldown < 0 {
		return fmt.Errorf("safety.cooldown must be >= 0")
	}
	return nil
}

type ProbeSpec struct {
	Type            string `yaml:"type"`
	URL             string `yaml:"url,omitempty"`               // For HTTP
	ExpectStatus    *int   `yaml:"expect_status,omitempty"`     // For HTTP
	ExpectNotStatus *int   `yaml:"expect_not_status,omitempty"` // For HTTP
	Timeout         int    `yaml:"timeout,omitempty"`           // Global
	HostPort        string `yaml:"host_port,omitempty"`         // For TCP
	Target          string `yaml:"target,omitempty"`            // For Exec
	Command         string `yaml:"command,omitempty"`           // For Exec
}

type SteadyStateProbe struct {
	Metric    string `yaml:"metric"`
	Source    string `yaml:"source"` // e.g., "prometheus"
	Query     string `yaml:"query"`
	Threshold string `yaml:"threshold"` // e.g., "< 250"
}

type ScenarioStep struct {
	Type      string      `yaml:"-"`
	DurationS int         `yaml:"-"`
	Action    *ActionSpec `yaml:"-"`
	Target    string      `yaml:"-"`
	Probe     *ProbeSpec  `yaml:"-"`
}

type waitStep struct {
	Wait string `yaml:"wait"`
}
type injectStep struct {
	Inject struct {
		Action ActionSpec `yaml:"action"`
		Target string     `yaml:"target"`
	} `yaml:"inject"`
}
type probeStep struct {
	Probe ProbeSpec `yaml:"probe"`
}

func (s *ScenarioStep) UnmarshalYAML(value *yaml.Node) error {
	var w waitStep
	if err := value.Decode(&w); err == nil && w.Wait != "" {
		s.Type = "wait"
		w.Wait = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(w.Wait)), "s")
		_, _ = fmt.Sscanf(w.Wait, "%d", &s.DurationS)
		return nil
	}
	var i injectStep
	if err := value.Decode(&i); err == nil && i.Inject.Target != "" {
		s.Type = "inject"
		s.Action = &i.Inject.Action
		s.Target = i.Inject.Target
		return nil
	}
	var p probeStep
	if err := value.Decode(&p); err == nil && (p.Probe.URL != "" || p.Probe.HostPort != "" || p.Probe.Command != "") {
		s.Type = "probe"
		s.Probe = &p.Probe
		if s.Probe.Type == "" {
			s.Probe.Type = "http"
		}
		if s.Probe.Timeout == 0 {
			s.Probe.Timeout = 5
		}
		return nil
	}
	return fmt.Errorf("unknown scenario step format")
}

type ScenarioConfig struct {
	Name        string             `yaml:"name"`
	Description string             `yaml:"description"`
	Hypothesis  string             `yaml:"hypothesis"`
	SteadyState []SteadyStateProbe `yaml:"steady_state,omitempty"`
	Steps       []ScenarioStep     `yaml:"steps"`
}
