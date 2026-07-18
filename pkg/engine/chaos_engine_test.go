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

// ---- startCycleAsync: async execution + single-flight guard (P2-1) ----

// TestChaosEngine_StartCycleAsync_WaitGroupBlocksUntilCycleFinishes is a
// regression test for the Ctrl+C-responsiveness fix: runLoop's stopEvent
// case calls cycleWG.Wait() before cleanup, to avoid racing cleanup against
// an in-flight cycle. This verifies that mechanism directly: cycleWG.Wait()
// must block for (at least) as long as the in-flight cycle takes, and
// cycleInFlight must be cleared once it finishes.
func TestChaosEngine_StartCycleAsync_WaitGroupBlocksUntilCycleFinishes(t *testing.T) {
	mock := NewMockRuntime()
	mock.StopDelay = 150 * time.Millisecond

	cfg := &config.ChaosConfig{
		Targets:  []string{"service-a"},
		Interval: 1,
		Actions:  []config.ActionSpec{{Name: "stop"}},
		Safety:   config.SafetyConfig{DryRun: false, MaxDown: 1, Cooldown: 0},
	}
	e := NewChaosEngine(cfg, "docker", nil, nil)

	start := time.Now()
	e.startCycleAsync(context.Background(), mock)

	if !e.cycleInFlight.Load() {
		t.Error("expected cycleInFlight to be true immediately after startCycleAsync returns")
	}

	e.cycleWG.Wait()
	elapsed := time.Since(start)

	if elapsed < mock.StopDelay {
		t.Errorf("expected cycleWG.Wait() to block for at least the cycle duration (%v), only waited %v", mock.StopDelay, elapsed)
	}
	if e.cycleInFlight.Load() {
		t.Error("expected cycleInFlight to be false once the cycle has finished")
	}
	if mock.CallCount("StopContainer") != 1 {
		t.Errorf("expected exactly 1 StopContainer call, got %d", mock.CallCount("StopContainer"))
	}
}

// TestChaosEngine_StartCycleAsync_SingleFlight verifies that a tick arriving
// while a previous cycle is still in flight is skipped rather than starting
// an overlapping cycle. Overlapping cycles could race on the
// cooldown/max_down safety checks in runCycle.
func TestChaosEngine_StartCycleAsync_SingleFlight(t *testing.T) {
	mock := NewMockRuntime()
	mock.StopDelay = 200 * time.Millisecond

	cfg := &config.ChaosConfig{
		Targets:  []string{"service-a"},
		Interval: 1,
		Actions:  []config.ActionSpec{{Name: "stop"}},
		Safety:   config.SafetyConfig{DryRun: false, MaxDown: 1, Cooldown: 0},
	}
	e := NewChaosEngine(cfg, "docker", nil, nil)
	ctx := context.Background()

	e.startCycleAsync(ctx, mock)

	// Give the first cycle time to actually start and set cycleInFlight,
	// then simulate a second tick arriving while it's still running.
	time.Sleep(30 * time.Millisecond)
	e.startCycleAsync(ctx, mock)

	e.cycleWG.Wait()

	if got := mock.CallCount("StopContainer"); got != 1 {
		t.Errorf("expected exactly 1 StopContainer call (overlapping tick should be skipped), got %d", got)
	}
}

// TestChaosEngine_StartCycleAsync_AllowsNextCycleAfterPreviousFinishes
// verifies the single-flight guard only blocks concurrent/overlapping
// cycles — a cycle started after the previous one has fully finished must
// run normally.
func TestChaosEngine_StartCycleAsync_AllowsNextCycleAfterPreviousFinishes(t *testing.T) {
	mock := NewMockRuntime()

	cfg := &config.ChaosConfig{
		Targets:  []string{"service-a"},
		Interval: 1,
		Actions:  []config.ActionSpec{{Name: "stop"}},
		Safety:   config.SafetyConfig{DryRun: false, MaxDown: 1, Cooldown: 0},
	}
	e := NewChaosEngine(cfg, "docker", nil, nil)
	ctx := context.Background()

	e.startCycleAsync(ctx, mock)
	e.cycleWG.Wait()

	// Reset state that would otherwise make the second cycle a no-op for
	// unrelated reasons (cooldown/max_down), so this test isolates the
	// single-flight guard specifically.
	e.mu.Lock()
	e.lastInjectionTime = time.Time{}
	e.downSet = map[string]bool{}
	e.mu.Unlock()

	e.startCycleAsync(ctx, mock)
	e.cycleWG.Wait()

	if got := mock.CallCount("StopContainer"); got != 2 {
		t.Errorf("expected 2 StopContainer calls across two sequential (non-overlapping) cycles, got %d", got)
	}
}

// TestChaosEngine_StartCycleAsync_RecoversFromPanic is a regression test:
// runLoop's own recover() only guards its own goroutine, so a panic inside
// the goroutine started by startCycleAsync would previously crash the whole
// process undetected. This verifies startCycleAsync has its own recover and
// that cycleWG/cycleInFlight are still cleaned up correctly afterwards —
// if the panic weren't recovered, this test process itself would crash
// rather than reporting a test failure.
func TestChaosEngine_StartCycleAsync_RecoversFromPanic(t *testing.T) {
	mock := NewMockRuntime()
	mock.StopPanic = true

	cfg := &config.ChaosConfig{
		Targets:  []string{"service-a"},
		Interval: 1,
		Actions:  []config.ActionSpec{{Name: "stop"}},
		Safety:   config.SafetyConfig{DryRun: false, MaxDown: 1, Cooldown: 0},
	}
	e := NewChaosEngine(cfg, "docker", nil, nil)

	e.startCycleAsync(context.Background(), mock)
	e.cycleWG.Wait()

	if e.cycleInFlight.Load() {
		t.Error("expected cycleInFlight to be false after a panicking cycle finishes recovering")
	}

	// The engine must still be usable afterwards — a subsequent cycle
	// (against a non-panicking mock) should run normally.
	mock2 := NewMockRuntime()
	e.startCycleAsync(context.Background(), mock2)
	e.cycleWG.Wait()

	if mock2.CallCount("StopContainer") != 1 {
		t.Errorf("expected the engine to remain usable after a recovered panic, got %d StopContainer calls", mock2.CallCount("StopContainer"))
	}
}
