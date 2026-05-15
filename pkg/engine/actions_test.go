package engine

import (
	"fmt"
	"testing"

	"github.com/ibrahimkizilarslan/entropy/pkg/config"
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
	manager.ScheduleRestore(mock, "target-1", 3600)
	manager.ScheduleRestore(mock, "target-2", 3600)

	manager.ClearAll()

	manager.mu.Lock()
	count := len(manager.timers)
	manager.mu.Unlock()
	if count != 0 {
		t.Errorf("Expected 0 timers after ClearAll, got %d", count)
	}
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
		if _, ok := ActionHandlers[action]; !ok {
			t.Errorf("Action %q not found in ActionHandlers", action)
		}
	}
}

func TestDispatch_Stop(t *testing.T) {
	mock := NewMockRuntime()
	spec := config.ActionSpec{Name: "stop"}

	info, err := Dispatch(spec, mock, "service-a")
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

	info, err := Dispatch(spec, mock, "service-a")
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

	info, err := Dispatch(spec, mock, "service-b")
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

	info, err := Dispatch(spec, mock, "service-a")
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

	info, err := Dispatch(spec, mock, "service-a")
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

	info, err := Dispatch(spec, mock, "service-a")
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

	info, err := Dispatch(spec, mock, "service-a")
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

	_, err := Dispatch(spec, mock, "service-a")
	if err == nil {
		t.Fatal("Expected error for unknown action, got nil")
	}
}

func TestDispatch_StopError(t *testing.T) {
	mock := NewMockRuntime()
	mock.StopErr = fmt.Errorf("docker daemon unavailable")
	spec := config.ActionSpec{Name: "stop"}

	_, err := Dispatch(spec, mock, "service-a")
	if err == nil {
		t.Fatal("Expected error when runtime fails, got nil")
	}
}

func TestDispatch_ContainerNotFound(t *testing.T) {
	mock := NewMockRuntime()
	spec := config.ActionSpec{Name: "stop"}

	_, err := Dispatch(spec, mock, "nonexistent-service")
	if err == nil {
		t.Fatal("Expected error for nonexistent container, got nil")
	}
}

func TestCleanupAll(t *testing.T) {
	// Should not panic when called with no active chaos
	CleanupAll()
}
