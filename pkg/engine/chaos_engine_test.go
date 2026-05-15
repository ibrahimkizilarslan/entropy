package engine

import (
	"context"
	"testing"
	"time"

	"github.com/ibrahimkizilarslan/entropy/pkg/config"
	"github.com/ibrahimkizilarslan/entropy/pkg/utils"
)

func TestChaosEngine_RunCycle_DryRun(t *testing.T) {
	mock := NewMockRuntime()
	cfg := &config.ChaosConfig{
		Targets:  []string{"service-a"},
		Interval: 1,
		Actions:  []config.ActionSpec{{Name: "stop"}},
		Safety:   config.SafetyConfig{DryRun: true, MaxDown: 1, Cooldown: 0},
	}

	events := []utils.EventRecord{}
	onEvent := func(e utils.EventRecord) {
		events = append(events, e)
	}

	engine := NewChaosEngine(cfg, "docker", onEvent, nil)

	engine.runCycle(context.Background(), mock) // Pass mock runtime

	// Dry run shouldn't execute actual stops
	if mock.CallCount("StopContainer") != 0 {
		t.Errorf("Expected 0 StopContainer calls in dry run, got %d", mock.CallCount("StopContainer"))
	}

	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	if !events[0].DryRun {
		t.Error("Expected event to be marked as DryRun")
	}
	
	status := engine.Status()
	if status.CycleCount != 1 {
		t.Errorf("Expected cycle count 1, got %d", status.CycleCount)
	}
}

func TestChaosEngine_RunCycle_Cooldown(t *testing.T) {
	mock := NewMockRuntime()
	cfg := &config.ChaosConfig{
		Targets:  []string{"service-a"},
		Interval: 1,
		Actions:  []config.ActionSpec{{Name: "stop"}},
		Safety:   config.SafetyConfig{DryRun: false, MaxDown: 1, Cooldown: 60},
	}

	engine := NewChaosEngine(cfg, "docker", nil, nil)

	// First cycle should trigger injection
	engine.runCycle(context.Background(), mock)
	if mock.CallCount("StopContainer") != 1 {
		t.Errorf("Expected 1 StopContainer call, got %d", mock.CallCount("StopContainer"))
	}

	// Second cycle should be blocked by cooldown
	engine.runCycle(context.Background(), mock)
	if mock.CallCount("StopContainer") != 1 {
		t.Errorf("Expected StopContainer call count to remain 1 due to cooldown, got %d", mock.CallCount("StopContainer"))
	}

	status := engine.Status()
	if status.CooldownRemaining <= 0 {
		t.Error("Expected cooldown to be active")
	}
}

func TestChaosEngine_RunCycle_MaxDown(t *testing.T) {
	mock := NewMockRuntime()
	cfg := &config.ChaosConfig{
		Targets:  []string{"service-a", "service-b"},
		Interval: 1,
		Actions:  []config.ActionSpec{{Name: "stop"}},
		Safety:   config.SafetyConfig{DryRun: false, MaxDown: 1, Cooldown: 0},
	}

	engine := NewChaosEngine(cfg, "docker", nil, nil)

	// First cycle injects a fault (stops one container)
	engine.runCycle(context.Background(), mock)
	status := engine.Status()
	if len(status.DownContainers) != 1 {
		t.Fatalf("Expected 1 down container, got %d", len(status.DownContainers))
	}

	// We reset cooldown manually to test max_down logic alone
	engine.mu.Lock()
	engine.lastInjectionTime = time.Time{}
	engine.mu.Unlock()

	// Second cycle should be blocked by max_down limit
	engine.runCycle(context.Background(), mock)
	if mock.CallCount("StopContainer") != 1 {
		t.Errorf("Expected StopContainer call count to remain 1 due to max_down, got %d", mock.CallCount("StopContainer"))
	}
}
