package config

import (
	"strings"
	"testing"

	yaml "gopkg.in/yaml.v3"
)

func TestActionSpec_IsNetwork(t *testing.T) {
	tests := []struct {
		name   string
		action string
		want   bool
	}{
		{"delay is network", "delay", true},
		{"loss is network", "loss", true},
		{"stop is not network", "stop", false},
		{"restart is not network", "restart", false},
		{"limit_cpu is not network", "limit_cpu", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := ActionSpec{Name: tt.action}
			if got := a.IsNetwork(); got != tt.want {
				t.Errorf("IsNetwork() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestActionSpec_IsResource(t *testing.T) {
	tests := []struct {
		name   string
		action string
		want   bool
	}{
		{"limit_cpu is resource", "limit_cpu", true},
		{"limit_memory is resource", "limit_memory", true},
		{"stop is not resource", "stop", false},
		{"delay is not resource", "delay", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := ActionSpec{Name: tt.action}
			if got := a.IsResource(); got != tt.want {
				t.Errorf("IsResource() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChaosConfig_Validate(t *testing.T) {
	validConfig := ChaosConfig{
		Interval: 10,
		Targets:  []string{"service-a"},
		Actions:  []ActionSpec{{Name: "stop"}},
		Safety:   SafetyConfig{MaxDown: 1, Cooldown: 0},
	}

	t.Run("valid config passes", func(t *testing.T) {
		if err := validConfig.Validate(); err != nil {
			t.Errorf("expected valid config, got error: %v", err)
		}
	})

	t.Run("interval < 1 fails", func(t *testing.T) {
		cfg := validConfig
		cfg.Interval = 0
		if err := cfg.Validate(); err == nil {
			t.Error("expected error for interval < 1")
		}
	})

	t.Run("empty targets fails", func(t *testing.T) {
		cfg := validConfig
		cfg.Targets = []string{}
		if err := cfg.Validate(); err == nil {
			t.Error("expected error for empty targets")
		}
	})

	t.Run("empty actions fails", func(t *testing.T) {
		cfg := validConfig
		cfg.Actions = []ActionSpec{}
		if err := cfg.Validate(); err == nil {
			t.Error("expected error for empty actions")
		}
	})

	t.Run("unknown action fails", func(t *testing.T) {
		cfg := validConfig
		cfg.Actions = []ActionSpec{{Name: "explode"}}
		if err := cfg.Validate(); err == nil {
			t.Error("expected error for unknown action")
		}
	})

	t.Run("max_down < 1 fails", func(t *testing.T) {
		cfg := validConfig
		cfg.Safety.MaxDown = 0
		if err := cfg.Validate(); err == nil {
			t.Error("expected error for max_down < 1")
		}
	})

	t.Run("cooldown < 0 fails", func(t *testing.T) {
		cfg := validConfig
		cfg.Safety.Cooldown = -1
		if err := cfg.Validate(); err == nil {
			t.Error("expected error for cooldown < 0")
		}
	})
}

func TestActionSpec_UnmarshalYAML_Scalar(t *testing.T) {
	yamlStr := `stop`
	var a ActionSpec
	if err := yaml.Unmarshal([]byte(yamlStr), &a); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if a.Name != "stop" {
		t.Errorf("expected Name='stop', got '%s'", a.Name)
	}
}

func TestActionSpec_UnmarshalYAML_Object(t *testing.T) {
	yamlStr := `
name: delay
latency_ms: 500
jitter_ms: 100
`
	var a ActionSpec
	if err := yaml.Unmarshal([]byte(yamlStr), &a); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if a.Name != "delay" {
		t.Errorf("expected Name='delay', got '%s'", a.Name)
	}
	if a.LatencyMs != 500 {
		t.Errorf("expected LatencyMs=500, got %d", a.LatencyMs)
	}
	if a.JitterMs != 100 {
		t.Errorf("expected JitterMs=100, got %d", a.JitterMs)
	}
}

func TestActionSpec_UnmarshalYAML_PercentAlias(t *testing.T) {
	yamlStr := `
name: loss
percent: 30
`
	var a ActionSpec
	if err := yaml.Unmarshal([]byte(yamlStr), &a); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if a.LossPercent != 30 {
		t.Errorf("expected LossPercent=30, got %d", a.LossPercent)
	}
}

func TestScenarioStep_UnmarshalYAML(t *testing.T) {
	t.Run("wait step", func(t *testing.T) {
		yamlStr := `wait: 5s`
		var s ScenarioStep
		if err := yaml.Unmarshal([]byte(yamlStr), &s); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if s.Type != "wait" {
			t.Errorf("expected Type='wait', got '%s'", s.Type)
		}
		if s.DurationS != 5 {
			t.Errorf("expected DurationS=5, got %d", s.DurationS)
		}
	})

	t.Run("inject step", func(t *testing.T) {
		yamlStr := `
inject:
  action: stop
  target: my-service
`
		var s ScenarioStep
		if err := yaml.Unmarshal([]byte(yamlStr), &s); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if s.Type != "inject" {
			t.Errorf("expected Type='inject', got '%s'", s.Type)
		}
		if s.Target != "my-service" {
			t.Errorf("expected Target='my-service', got '%s'", s.Target)
		}
		if s.Action == nil || s.Action.Name != "stop" {
			t.Error("expected Action.Name='stop'")
		}
	})

	t.Run("http probe step", func(t *testing.T) {
		yamlStr := `
probe:
  type: http
  url: "http://localhost:8080/health"
  expect_status: 200
`
		var s ScenarioStep
		if err := yaml.Unmarshal([]byte(yamlStr), &s); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if s.Type != "probe" {
			t.Errorf("expected Type='probe', got '%s'", s.Type)
		}
		if s.Probe == nil || s.Probe.URL != "http://localhost:8080/health" {
			t.Error("expected Probe.URL to be set")
		}
		if s.Probe.ExpectStatus == nil || *s.Probe.ExpectStatus != 200 {
			t.Error("expected Probe.ExpectStatus=200")
		}
	})

	t.Run("tcp probe step", func(t *testing.T) {
		yamlStr := `
probe:
  type: tcp
  host_port: "localhost:6379"
`
		var s ScenarioStep
		if err := yaml.Unmarshal([]byte(yamlStr), &s); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if s.Type != "probe" || s.Probe.Type != "tcp" {
			t.Error("expected tcp probe")
		}
		if s.Probe.HostPort != "localhost:6379" {
			t.Errorf("expected HostPort='localhost:6379', got '%s'", s.Probe.HostPort)
		}
	})

	t.Run("probe defaults", func(t *testing.T) {
		yamlStr := `
probe:
  url: "http://localhost:8080"
`
		var s ScenarioStep
		if err := yaml.Unmarshal([]byte(yamlStr), &s); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if s.Probe.Type != "http" {
			t.Errorf("expected default type='http', got '%s'", s.Probe.Type)
		}
		if s.Probe.Timeout != 5 {
			t.Errorf("expected default timeout=5, got %d", s.Probe.Timeout)
		}
	})

	t.Run("unknown step fails", func(t *testing.T) {
		yamlStr := `unknown_field: value`
		var s ScenarioStep
		if err := yaml.Unmarshal([]byte(yamlStr), &s); err == nil {
			t.Error("expected error for unknown step format")
		}
	})
}

// TestScenarioStep_UnmarshalYAML_MalformedSteps is a regression test for the
// P2 key-based-dispatch fix: a step with a recognized key (wait/inject/
// probe) but a missing required field must produce a specific,
// actionable error — not the old generic "unknown scenario step format",
// which field-sniffing produced because the malformed step never matched
// any of the three shapes it tried.
func TestScenarioStep_UnmarshalYAML_MalformedSteps(t *testing.T) {
	tests := []struct {
		name          string
		yamlStr       string
		wantErrSubstr string
	}{
		{
			name: "inject missing target",
			yamlStr: `
inject:
  action: stop
`,
			wantErrSubstr: "target",
		},
		{
			name: "inject missing action",
			yamlStr: `
inject:
  target: my-service
`,
			wantErrSubstr: "action",
		},
		{
			name:          "wait with empty value",
			yamlStr:       `wait: ""`,
			wantErrSubstr: "duration",
		},
		{
			name: "probe with no url/host_port/command",
			yamlStr: `
probe:
  type: http
  timeout: 5
`,
			wantErrSubstr: "url",
		},
		{
			name: "multiple step keys in one step",
			yamlStr: `
wait: 5s
inject:
  action: stop
  target: my-service
`,
			wantErrSubstr: "multiple step types",
		},
		{
			name:          "no recognized step key",
			yamlStr:       `some_typo: value`,
			wantErrSubstr: "wait",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s ScenarioStep
			err := yaml.Unmarshal([]byte(tt.yamlStr), &s)
			if err == nil {
				t.Fatalf("expected an error, got none (Type=%q)", s.Type)
			}
			if !strings.Contains(err.Error(), tt.wantErrSubstr) {
				t.Errorf("expected error to mention %q, got: %v", tt.wantErrSubstr, err)
			}
		})
	}
}

// TestScenarioStep_UnmarshalYAML_ValidStepsStillWork is a regression test
// ensuring the key-based dispatch didn't change behavior for well-formed
// steps — same YAML shapes as the pre-existing tests above, checked in bulk.
func TestScenarioStep_UnmarshalYAML_ValidStepsStillWork(t *testing.T) {
	tests := []struct {
		name     string
		yamlStr  string
		wantType string
	}{
		{"wait", `wait: 10s`, "wait"},
		{"inject", "inject:\n  action: pause\n  target: svc\n", "inject"},
		{"probe url", "probe:\n  url: \"http://localhost/health\"\n", "probe"},
		{"probe host_port", "probe:\n  host_port: \"localhost:6379\"\n", "probe"},
		{"probe command", "probe:\n  type: exec\n  target: svc\n  command: \"cat /proc/uptime\"\n", "probe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s ScenarioStep
			if err := yaml.Unmarshal([]byte(tt.yamlStr), &s); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if s.Type != tt.wantType {
				t.Errorf("expected Type=%q, got %q", tt.wantType, s.Type)
			}
		})
	}
}
