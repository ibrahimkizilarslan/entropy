package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func setDefaults(cfg *ChaosConfig) {
	if cfg.Safety.MaxDown == 0 {
		cfg.Safety.MaxDown = 1
	}
	if cfg.Safety.Cooldown == 0 {
		cfg.Safety.Cooldown = 30
	}
	if len(cfg.Actions) == 0 {
		cfg.Actions = []ActionSpec{{Name: "stop"}}
	}

	for i := range cfg.Actions {
		if cfg.Actions[i].Name == "delay" {
			if cfg.Actions[i].LatencyMs == 0 {
				cfg.Actions[i].LatencyMs = 300
			}
		}
		if cfg.Actions[i].Name == "loss" {
			if cfg.Actions[i].LossPercent == 0 {
				cfg.Actions[i].LossPercent = 20
			}
		}
		if cfg.Actions[i].Name == "limit_cpu" {
			if cfg.Actions[i].CPUs == 0 {
				cfg.Actions[i].CPUs = 0.25
			}
		}
		if cfg.Actions[i].Name == "limit_memory" {
			if cfg.Actions[i].MemoryMB == 0 {
				cfg.Actions[i].MemoryMB = 128
			}
		}
	}
}

func LoadConfig(path string) (*ChaosConfig, error) {
	if path == "" {
		path = "chaos.yaml"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config file not found: '%s'\n  → Copy chaos.example.yaml to chaos.yaml and edit it", path)
	}
	var cfg ChaosConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %v", err)
	}

	setDefaults(&cfg)

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config in '%s': %v", path, err)
	}

	return &cfg, nil
}

func LoadScenario(path string) (*ScenarioConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("scenario file not found: '%s'", path)
	}
	var cfg ScenarioConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse scenario: %v", err)
	}
	if cfg.Name == "" {
		cfg.Name = filepath.Base(path)
	}
	return &cfg, nil
}
