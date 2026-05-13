package engine

import (
	"testing"
)

func TestNewChaosEngine(t *testing.T) {
	// Engine creation requires a real config, which needs proper setup
	// For now, we test that action handlers exist
	if len(ActionHandlers) == 0 {
		t.Error("ActionHandlers should not be empty")
	}
}

func TestActionHandlersExist(t *testing.T) {
	expectedCount := 7 // stop, restart, pause, delay, loss, limit_cpu, limit_memory
	if len(ActionHandlers) < expectedCount {
		t.Errorf("Expected at least %d action handlers, got %d", expectedCount, len(ActionHandlers))
	}
}

func TestNetworkActions(t *testing.T) {
	// Test that network manager exists
	if NetworkManager == nil {
		t.Error("NetworkManager should not be nil")
	}
}

func TestResourceActions(t *testing.T) {
	// Test that resource manager exists
	if ResourceManager == nil {
		t.Error("ResourceManager should not be nil")
	}
}
