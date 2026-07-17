package engine

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestValidateContainerName(t *testing.T) {
	tests := []struct {
		name      string
		container string
		wantErr   bool
	}{
		{"valid simple", "my-container", false},
		{"valid with numbers", "app-1", false},
		{"valid with dots", "foo.bar.baz", false},
		{"valid with underscores", "my_db_2", false},
		{"invalid with spaces", "my container", true},
		{"invalid with slash", "my/container", true},
		{"invalid with shell injection", "app-1; rm -rf /", true},
		{"invalid starts with hyphen", "-app", true},
		{"invalid with backticks", "`whoami`", true},
		{"invalid with variables", "$USER", true},
		{"invalid with ampersand", "app&echo", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateContainerName(tt.container)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateContainerName(%q) error = %v, wantErr %v", tt.container, err, tt.wantErr)
			}
		})
	}
}

func TestNewNetworkChaosManager(t *testing.T) {
	m := NewNetworkChaosManager()
	if m == nil {
		t.Fatal("NewNetworkChaosManager returned nil")
	}
	if m.active == nil {
		t.Error("active map should be initialized")
	}
	if m.timers == nil {
		t.Error("timers map should be initialized")
	}
	if m.netIface == "" {
		t.Error("netIface should have a default value")
	}
}

func TestNetworkChaosManager_ClearAll_Empty(t *testing.T) {
	m := NewNetworkChaosManager()
	// Should not panic on empty manager
	m.ClearAll()
	if len(m.active) != 0 {
		t.Error("active should be empty after ClearAll")
	}
	if len(m.timers) != 0 {
		t.Error("timers should be empty after ClearAll")
	}
}

// TestNetworkChaosManager_ConcurrentInjection_DifferentTargets is a
// regression test for the P1 lock-contention fix: injecting into one target
// with a slow (mocked) exec must not block injection into a different
// target. Under the old implementation (a single mutex held across the tc
// exec call), the fast target would have waited for the slow target's exec
// to complete before even starting.
func TestNetworkChaosManager_ConcurrentInjection_DifferentTargets(t *testing.T) {
	m := NewNetworkChaosManager()

	slowRuntime := NewMockRuntime()
	slowRuntime.ExecDelay = 300 * time.Millisecond

	fastRuntime := NewMockRuntime()

	var wg sync.WaitGroup
	fastDone := make(chan time.Duration, 1)
	start := time.Now()

	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = m.InjectDelay(context.Background(), slowRuntime, "target-slow", 100, 0, nil)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		// Give the slow injection a head start so it acquires any
		// contended lock first, if one existed.
		time.Sleep(30 * time.Millisecond)
		_ = m.InjectDelay(context.Background(), fastRuntime, "target-fast", 100, 0, nil)
		fastDone <- time.Since(start)
	}()

	wg.Wait()
	close(fastDone)

	elapsed := <-fastDone
	if elapsed >= 300*time.Millisecond {
		t.Errorf("expected injection into target-fast to complete without waiting on target-slow's exec, took %v", elapsed)
	}
}

// TestNetworkChaosManager_ConcurrentInjection_SameTarget_Serializes verifies
// that per-target locking still serializes operations on the SAME target
// (required so "remove old rule, add new rule" stays atomic). The test
// mainly documents intent; -race is what actually catches corruption.
func TestNetworkChaosManager_ConcurrentInjection_SameTarget_Serializes(t *testing.T) {
	m := NewNetworkChaosManager()
	rt := NewMockRuntime()
	rt.ExecDelay = 20 * time.Millisecond

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(latency int) {
			defer wg.Done()
			_ = m.InjectDelay(context.Background(), rt, "same-target", latency, 0, nil)
		}(100 + i)
	}
	wg.Wait()

	m.mu.Lock()
	_, stillActive := m.active["same-target"]
	m.mu.Unlock()
	if !stillActive {
		t.Error("expected same-target to still have an active rule after all concurrent injections completed")
	}
}

// TestNetworkChaosManager_ClearAll_ClearsConcurrently verifies ClearAll
// reverts multiple targets without one slow target blocking the others.
func TestNetworkChaosManager_ClearAll_ClearsConcurrently(t *testing.T) {
	m := NewNetworkChaosManager()

	slowRuntime := NewMockRuntime()
	slowRuntime.ExecDelay = 200 * time.Millisecond
	fastRuntime := NewMockRuntime()

	if err := m.InjectDelay(context.Background(), slowRuntime, "slow-target", 100, 0, nil); err != nil {
		t.Fatalf("setup: failed to inject into slow-target: %v", err)
	}
	if err := m.InjectDelay(context.Background(), fastRuntime, "fast-target", 100, 0, nil); err != nil {
		t.Fatalf("setup: failed to inject into fast-target: %v", err)
	}

	start := time.Now()
	m.ClearAll()
	elapsed := time.Since(start)

	// If clears were serialized, this would take slow+fast time; if
	// concurrent, it should take roughly max(slow, fast).
	if elapsed >= 350*time.Millisecond {
		t.Errorf("expected ClearAll to revert targets concurrently, took %v", elapsed)
	}

	m.mu.Lock()
	remaining := len(m.active)
	m.mu.Unlock()
	if remaining != 0 {
		t.Errorf("expected no active rules after ClearAll, got %d", remaining)
	}
}
