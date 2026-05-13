package engine_test

import (
	"testing"

	"github.com/ibrahimkizilarslan/entropy/pkg/config"
	"github.com/ibrahimkizilarslan/entropy/pkg/engine"
)

func TestNewResourceChaosManager(t *testing.T) {
	manager := engine.NewResourceChaosManager()

	if manager == nil {
		t.Fatalf("NewResourceChaosManager returned nil")
	}
}

func TestResourceChaosManagerClearAll(t *testing.T) {
	manager := engine.NewResourceChaosManager()

	// In real code we can't easily add nil timers because of private fields
	// But we can check if ClearAll works on an empty manager without crashing
	manager.ClearAll()

	// Verify structure works
	if manager == nil {
		t.Error("Manager should not be nil")
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
		if _, ok := engine.ActionHandlers[action]; !ok {
			t.Errorf("Action %q not found in ActionHandlers", action)
		}
	}
}

func TestDispatchWithValidAction(t *testing.T) {
	tests := []struct {
		name        string
		actionName  string
		shouldExist bool
	}{
		{
			name:        "stop action",
			actionName:  "stop",
			shouldExist: true,
		},
		{
			name:        "restart action",
			actionName:  "restart",
			shouldExist: true,
		},
		{
			name:        "pause action",
			actionName:  "pause",
			shouldExist: true,
		},
		{
			name:        "delay action",
			actionName:  "delay",
			shouldExist: true,
		},
		{
			name:        "loss action",
			actionName:  "loss",
			shouldExist: true,
		},
		{
			name:        "limit_cpu action",
			actionName:  "limit_cpu",
			shouldExist: true,
		},
		{
			name:        "limit_memory action",
			actionName:  "limit_memory",
			shouldExist: true,
		},
		{
			name:        "invalid action",
			actionName:  "nonexistent",
			shouldExist: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, ok := engine.ActionHandlers[tt.actionName]

			if tt.shouldExist && !ok {
				t.Errorf("Action %q should exist in ActionHandlers", tt.actionName)
			}

			if !tt.shouldExist && ok {
				t.Errorf("Action %q should not exist in ActionHandlers", tt.actionName)
			}

			if tt.shouldExist && handler == nil {
				t.Errorf("Handler for action %q is nil", tt.actionName)
			}
		})
	}
}

func TestActionSpecBasics(t *testing.T) {
	spec := config.ActionSpec{
		Name:      "delay",
		LatencyMs: 300,
		JitterMs:  50,
	}

	if spec.Name != "delay" {
		t.Errorf("Action spec name mismatch: got %q, want %q", spec.Name, "delay")
	}

	if spec.LatencyMs != 300 {
		t.Errorf("Latency mismatch: got %d, want 300", spec.LatencyMs)
	}

	if spec.JitterMs != 50 {
		t.Errorf("Jitter mismatch: got %d, want 50", spec.JitterMs)
	}
}

func TestActionSpecIsNetwork(t *testing.T) {
	tests := []struct {
		name       string
		actionName string
		isNetwork  bool
	}{
		{"delay is network", "delay", true},
		{"loss is network", "loss", true},
		{"stop is not network", "stop", false},
		{"restart is not network", "restart", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := config.ActionSpec{Name: tt.actionName}
			if spec.IsNetwork() != tt.isNetwork {
				t.Errorf("IsNetwork for %q: got %v, want %v", tt.actionName, spec.IsNetwork(), tt.isNetwork)
			}
		})
	}
}

func TestActionSpecIsResource(t *testing.T) {
	tests := []struct {
		name       string
		actionName string
		isResource bool
	}{
		{"limit_cpu is resource", "limit_cpu", true},
		{"limit_memory is resource", "limit_memory", true},
		{"stop is not resource", "stop", false},
		{"delay is not resource", "delay", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := config.ActionSpec{Name: tt.actionName}
			if spec.IsResource() != tt.isResource {
				t.Errorf("IsResource for %q: got %v, want %v", tt.actionName, spec.IsResource(), tt.isResource)
			}
		})
	}
}
