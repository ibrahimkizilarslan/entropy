package engine

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ibrahimkizilarslan/entropy/pkg/config"
	"github.com/ibrahimkizilarslan/entropy/pkg/registry"
)

func TestNewResourceChaosManager(t *testing.T) {
	manager := NewResourceChaosManager()
	if manager == nil {
		t.Fatalf("NewResourceChaosManager returned nil")
	}
	if manager.timers == nil {
		t.Fatalf("Manager timers map not initialized")
	}
	if len(manager.timers) != 0 {
		t.Errorf("Manager timers should be empty, got %d", len(manager.timers))
	}
}

func TestResourceChaosManagerClearAll(t *testing.T) {
	manager := NewResourceChaosManager()
	mock := NewMockRuntime()
	// Schedule some restores and then clear all
	manager.ScheduleRestore(mock, "target-1", registry.FaultTypeCPULimit, 3600, 50000, 100000, 0)
	manager.ScheduleRestore(mock, "target-2", registry.FaultTypeCPULimit, 3600, 50000, 100000, 0)

	manager.ClearAll()

	manager.mu.Lock()
	count := len(manager.timers)
	manager.mu.Unlock()
	if count != 0 {
		t.Errorf("Expected 0 timers after ClearAll, got %d", count)
	}
}

// TestResourceChaosManager_ConcurrentScheduleRestore_DifferentTargets is a
// regression test for the P1 lock-contention fix: scheduling a restore for
// one target (including its registry.Write call) must not block scheduling
// a restore for a different target. Uses a real on-disk FaultRegistry since
// the registry's own I/O (fsync) is exactly what used to be held under
// ResourceChaosManager.mu.
func TestResourceChaosManager_ConcurrentScheduleRestore_DifferentTargets(t *testing.T) {
	regPath := filepath.Join(t.TempDir(), "registry.json")
	reg, err := registry.Open(regPath)
	if err != nil {
		t.Fatalf("failed to open registry: %v", err)
	}

	manager := NewResourceChaosManager()
	manager.SetRegistry(reg)
	mock := NewMockRuntime()

	var wg sync.WaitGroup
	targets := []string{"target-a", "target-b", "target-c"}
	for _, target := range targets {
		wg.Add(1)
		go func(tgt string) {
			defer wg.Done()
			manager.ScheduleRestore(mock, tgt, registry.FaultTypeCPULimit, 3600, 50000, 100000, 0)
		}(target)
	}
	wg.Wait()

	manager.mu.Lock()
	timerCount := len(manager.timers)
	recordCount := len(manager.activeRecordIDs)
	manager.mu.Unlock()

	if timerCount != len(targets) {
		t.Errorf("expected %d timers after concurrent schedules, got %d", len(targets), timerCount)
	}
	if recordCount != len(targets) {
		t.Errorf("expected %d registry records after concurrent schedules, got %d", len(targets), recordCount)
	}

	active := reg.ListActive()
	if len(active) != len(targets) {
		t.Errorf("expected %d active registry records on disk, got %d", len(targets), len(active))
	}

	manager.ClearAll()
}

func TestActionHandlersMapExists(t *testing.T) {
	expectedActions := []string{
		"stop",
		"restart",
		"pause",
		"delay",
		"loss",
		"limit_cpu",
		"limit_memory",
	}

	for _, action := range expectedActions {
		if _, ok := GetActionHandler(action); !ok {
			t.Errorf("Action %q not found in ActionHandlers", action)
		}
	}
}

func TestDispatch_Stop(t *testing.T) {
	mock := NewMockRuntime()
	spec := config.ActionSpec{Name: "stop"}

	info, err := Dispatch(context.Background(), spec, mock, "service-a")
	if err != nil {
		t.Fatalf("Dispatch stop failed: %v", err)
	}
	if info.Status != "exited" {
		t.Errorf("Expected status 'exited', got '%s'", info.Status)
	}
	if mock.CallCount("StopContainer") != 1 {
		t.Errorf("Expected 1 StopContainer call, got %d", mock.CallCount("StopContainer"))
	}
}

func TestDispatch_Restart(t *testing.T) {
	mock := NewMockRuntime()
	spec := config.ActionSpec{Name: "restart"}

	info, err := Dispatch(context.Background(), spec, mock, "service-a")
	if err != nil {
		t.Fatalf("Dispatch restart failed: %v", err)
	}
	if info.Status != "running" {
		t.Errorf("Expected status 'running', got '%s'", info.Status)
	}
	if mock.CallCount("RestartContainer") != 1 {
		t.Errorf("Expected 1 RestartContainer call, got %d", mock.CallCount("RestartContainer"))
	}
}

func TestDispatch_Pause(t *testing.T) {
	mock := NewMockRuntime()
	spec := config.ActionSpec{Name: "pause"}

	info, err := Dispatch(context.Background(), spec, mock, "service-b")
	if err != nil {
		t.Fatalf("Dispatch pause failed: %v", err)
	}
	if info.Status != "paused" {
		t.Errorf("Expected status 'paused', got '%s'", info.Status)
	}
	if mock.CallCount("PauseContainer") != 1 {
		t.Errorf("Expected 1 PauseContainer call, got %d", mock.CallCount("PauseContainer"))
	}
}

func TestDispatch_Delay(t *testing.T) {
	mock := NewMockRuntime()
	dur := 10
	spec := config.ActionSpec{Name: "delay", LatencyMs: 300, JitterMs: 50, Duration: &dur}

	info, err := Dispatch(context.Background(), spec, mock, "service-a")
	if err != nil {
		t.Fatalf("Dispatch delay failed: %v", err)
	}
	if info.Status != "running (delayed)" {
		t.Errorf("Expected status 'running (delayed)', got '%s'", info.Status)
	}
	if mock.CallCount("InjectNetworkDelay") != 1 {
		t.Errorf("Expected 1 InjectNetworkDelay call, got %d", mock.CallCount("InjectNetworkDelay"))
	}
}

func TestDispatch_Loss(t *testing.T) {
	mock := NewMockRuntime()
	dur := 5
	spec := config.ActionSpec{Name: "loss", LossPercent: 20, Duration: &dur}

	info, err := Dispatch(context.Background(), spec, mock, "service-a")
	if err != nil {
		t.Fatalf("Dispatch loss failed: %v", err)
	}
	if info.Status != "running (lossy)" {
		t.Errorf("Expected status 'running (lossy)', got '%s'", info.Status)
	}
	if mock.CallCount("InjectNetworkLoss") != 1 {
		t.Errorf("Expected 1 InjectNetworkLoss call, got %d", mock.CallCount("InjectNetworkLoss"))
	}
}

func TestDispatch_LimitCPU(t *testing.T) {
	mock := NewMockRuntime()
	dur := 10
	spec := config.ActionSpec{Name: "limit_cpu", CPUs: 0.5, Duration: &dur}

	info, err := Dispatch(context.Background(), spec, mock, "service-a")
	if err != nil {
		t.Fatalf("Dispatch limit_cpu failed: %v", err)
	}
	if info.Name != "service-a" {
		t.Errorf("Expected name 'service-a', got '%s'", info.Name)
	}
	if mock.CallCount("UpdateContainerResources") != 1 {
		t.Errorf("Expected 1 UpdateContainerResources call, got %d", mock.CallCount("UpdateContainerResources"))
	}
}

func TestDispatch_LimitMemory(t *testing.T) {
	mock := NewMockRuntime()
	dur := 10
	spec := config.ActionSpec{Name: "limit_memory", MemoryMB: 128, Duration: &dur}

	info, err := Dispatch(context.Background(), spec, mock, "service-a")
	if err != nil {
		t.Fatalf("Dispatch limit_memory failed: %v", err)
	}
	if info.Name != "service-a" {
		t.Errorf("Expected name 'service-a', got '%s'", info.Name)
	}
	if mock.CallCount("UpdateContainerResources") != 1 {
		t.Errorf("Expected 1 UpdateContainerResources call, got %d", mock.CallCount("UpdateContainerResources"))
	}
}

func TestDispatch_UnknownAction(t *testing.T) {
	mock := NewMockRuntime()
	spec := config.ActionSpec{Name: "explode"}

	_, err := Dispatch(context.Background(), spec, mock, "service-a")
	if err == nil {
		t.Fatal("Expected error for unknown action, got nil")
	}
}

func TestDispatch_StopError(t *testing.T) {
	mock := NewMockRuntime()
	mock.StopErr = fmt.Errorf("docker daemon unavailable")
	spec := config.ActionSpec{Name: "stop"}

	_, err := Dispatch(context.Background(), spec, mock, "service-a")
	if err == nil {
		t.Fatal("Expected error when runtime fails, got nil")
	}
}

func TestDispatch_ContainerNotFound(t *testing.T) {
	mock := NewMockRuntime()
	spec := config.ActionSpec{Name: "stop"}

	_, err := Dispatch(context.Background(), spec, mock, "nonexistent-service")
	if err == nil {
		t.Fatal("Expected error for nonexistent container, got nil")
	}
}

// End of file
