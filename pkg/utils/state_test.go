package utils

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStateManager_WriteAndRead(t *testing.T) {
	tempDir := t.TempDir()
	sm := NewStateManager(tempDir)

	now := time.Now().UTC().Truncate(time.Second) // Truncate for JSON comparison
	originalState := &EngineState{
		PID:               1234,
		StartedAt:         now,
		ConfigPath:        "test.yaml",
		DryRun:            true,
		CycleCount:        5,
		DownContainers:    []string{"svc1", "svc2"},
		CooldownRemaining: 10.5,
		CooldownTotal:     30,
		LastEvent: &EventRecord{
			Timestamp:    now,
			Action:       "stop",
			Target:       "svc1",
			ResultStatus: "stopped",
		},
	}

	err := sm.Write(originalState)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Verify file exists and has correct permissions
	info, err := os.Stat(sm.StateFile())
	if err != nil {
		t.Fatalf("Failed to stat state file: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("Expected file permissions 0600, got %v", info.Mode().Perm())
	}

	readState, err := sm.Read()
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if readState == nil {
		t.Fatal("Read returned nil state")
	}

	if readState.PID != originalState.PID {
		t.Errorf("Expected PID %d, got %d", originalState.PID, readState.PID)
	}
	if !readState.StartedAt.Equal(originalState.StartedAt) {
		t.Errorf("Expected StartedAt %v, got %v", originalState.StartedAt, readState.StartedAt)
	}
	if readState.CycleCount != originalState.CycleCount {
		t.Errorf("Expected CycleCount %d, got %d", originalState.CycleCount, readState.CycleCount)
	}
	if len(readState.DownContainers) != 2 || readState.DownContainers[0] != "svc1" {
		t.Errorf("Expected DownContainers to match, got %v", readState.DownContainers)
	}
	if readState.LastEvent == nil || readState.LastEvent.Action != "stop" {
		t.Errorf("Expected LastEvent action 'stop', got %v", readState.LastEvent)
	}
}

func TestStateManager_Read_NotExist(t *testing.T) {
	tempDir := t.TempDir()
	sm := NewStateManager(tempDir)

	state, err := sm.Read()
	if err != nil {
		t.Fatalf("Expected nil error for non-existent file, got %v", err)
	}
	if state != nil {
		t.Fatalf("Expected nil state for non-existent file, got %v", state)
	}
}

func TestStateManager_Clear(t *testing.T) {
	tempDir := t.TempDir()
	sm := NewStateManager(tempDir)

	// Create a dummy file
	err := os.MkdirAll(filepath.Dir(sm.StateFile()), 0755)
	if err != nil {
		t.Fatalf("Failed to create dir: %v", err)
	}
	err = os.WriteFile(sm.StateFile(), []byte("{}"), 0600)
	if err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	err = sm.Clear()
	if err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	_, err = os.Stat(sm.StateFile())
	if !os.IsNotExist(err) {
		t.Fatalf("Expected file to be deleted, stat returned error: %v", err)
	}

	// Calling Clear again should not fail
	err = sm.Clear()
	if err != nil {
		t.Fatalf("Second Clear failed: %v", err)
	}
}

func TestStateManager_IsAlive(t *testing.T) {
	sm := NewStateManager("")

	// Our own PID is definitely alive
	myPID := os.Getpid()
	if !sm.IsAlive(myPID) {
		t.Errorf("IsAlive returned false for our own PID %d", myPID)
	}

	// Unlikely to have a process with PID 999999
	if sm.IsAlive(999999) {
		t.Errorf("IsAlive returned true for non-existent PID 999999")
	}
}

func TestStateManager_RunningPID(t *testing.T) {
	tempDir := t.TempDir()
	sm := NewStateManager(tempDir)

	// No state file
	if pid := sm.RunningPID(); pid != nil {
		t.Errorf("Expected nil RunningPID, got %d", *pid)
	}

	// Write state with our own PID
	myPID := os.Getpid()
	sm.Write(&EngineState{PID: myPID})

	pid := sm.RunningPID()
	if pid == nil || *pid != myPID {
		t.Errorf("Expected RunningPID %d, got %v", myPID, pid)
	}

	// Write state with dead PID
	sm.Write(&EngineState{PID: 999999})
	if pid := sm.RunningPID(); pid != nil {
		t.Errorf("Expected nil RunningPID for dead process, got %d", *pid)
	}
}

func TestStateManager_CorruptJson(t *testing.T) {
	tempDir := t.TempDir()
	sm := NewStateManager(tempDir)

	err := os.MkdirAll(filepath.Dir(sm.StateFile()), 0755)
	if err != nil {
		t.Fatalf("Failed to create dir: %v", err)
	}
	err = os.WriteFile(sm.StateFile(), []byte("{invalid json}"), 0600)
	if err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	_, err = sm.Read()
	if err == nil {
		t.Fatal("Expected error reading corrupt JSON, got nil")
	}
	if _, ok := err.(*json.SyntaxError); !ok && err.Error() != "invalid character 'i' looking for beginning of object key string" {
		t.Errorf("Expected JSON syntax error, got %v", err)
	}
}
