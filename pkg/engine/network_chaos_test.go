package engine

import (
	"testing"
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
