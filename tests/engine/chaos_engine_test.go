package engine_test

import (
	"testing"

	"github.com/ibrahimkizilarslan/entropy/pkg/engine"
)

func TestNewChaosEngine(t *testing.T) {
	// Engine creation requires a real config, which needs proper setup
	// For now, we test that action handlers exist
	if len(engine.GetSupportedActions()) == 0 {
		t.Error("ActionHandlers should not be empty")
	}
}

func TestActionHandlersExist(t *testing.T) {
	expectedCount := 7 // stop, restart, pause, delay, loss, limit_cpu, limit_memory
	if len(engine.GetSupportedActions()) < expectedCount {
		t.Errorf("Expected at least %d action handlers, got %d", expectedCount, len(engine.GetSupportedActions()))
	}
}

// Removed singleton tests
