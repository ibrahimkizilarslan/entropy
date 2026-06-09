package registry

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --- Test helpers ---

func newTempRegistry(t *testing.T) (*FaultRegistry, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")
	reg, err := Open(path)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	return reg, path
}

func makeRecord(target string, ft FaultType) FaultRecord {
	return FaultRecord{
		Target:    target,
		FaultType: ft,
		Runtime:   "docker",
		Params:    map[string]any{"iface": "eth0"},
	}
}

// --- Open / Load tests ---

func TestOpen_NewFile(t *testing.T) {
	reg, path := newTempRegistry(t)
	if reg == nil {
		t.Fatal("expected non-nil registry")
	}
	if reg.Path() != path {
		t.Errorf("path mismatch: got %s, want %s", reg.Path(), path)
	}
	if len(reg.ListAll()) != 0 {
		t.Error("expected empty registry on new file")
	}
}

func TestOpen_ExistingFile(t *testing.T) {
	reg, path := newTempRegistry(t)
	_, err := reg.Write(makeRecord("svc-a", FaultTypeNetworkDelay))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Reload from disk
	reg2, err := Open(path)
	if err != nil {
		t.Fatalf("second Open failed: %v", err)
	}
	if len(reg2.ListAll()) != 1 {
		t.Errorf("expected 1 record after reload, got %d", len(reg2.ListAll()))
	}
}

func TestOpen_CorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")

	// Write garbage
	if err := os.WriteFile(path, []byte("{not valid json{{{{"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Should not return an error — starts fresh and renames corrupt file
	reg, err := Open(path)
	if err != nil {
		t.Fatalf("Open with corrupt file should not error, got: %v", err)
	}
	if len(reg.ListAll()) != 0 {
		t.Error("expected empty registry after corrupt file recovery")
	}

	// Corrupt file should have been renamed
	if _, err := os.Stat(path + ".corrupt"); os.IsNotExist(err) {
		t.Error("expected corrupt file to be renamed with .corrupt suffix")
	}
}

// --- Write tests ---

func TestWrite_AssignsID(t *testing.T) {
	reg, _ := newTempRegistry(t)
	rec := makeRecord("svc-a", FaultTypeNetworkDelay)
	rec.ID = "" // ensure ID is auto-assigned

	id, err := reg.Write(rec)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if id == "" {
		t.Error("expected non-empty ID to be assigned")
	}
}

func TestWrite_SetInjectedAt(t *testing.T) {
	reg, _ := newTempRegistry(t)
	before := time.Now().UTC().Add(-time.Second)

	id, err := reg.Write(makeRecord("svc-a", FaultTypeNetworkDelay))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	all := reg.ListAll()
	if len(all) != 1 {
		t.Fatalf("expected 1 record, got %d", len(all))
	}
	if all[0].ID != id {
		t.Errorf("ID mismatch")
	}
	if all[0].InjectedAt.Before(before) {
		t.Error("InjectedAt should be set to approximately now")
	}
}

func TestWrite_Persists(t *testing.T) {
	reg, path := newTempRegistry(t)
	_, err := reg.Write(makeRecord("svc-a", FaultTypeCPULimit))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Verify the file exists and is valid JSON
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	var f registryFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("persisted file is not valid JSON: %v", err)
	}
	if len(f.Records) != 1 {
		t.Errorf("expected 1 persisted record, got %d", len(f.Records))
	}
}

// --- MarkReverted tests ---

func TestMarkReverted_Basic(t *testing.T) {
	reg, _ := newTempRegistry(t)
	id, _ := reg.Write(makeRecord("svc-a", FaultTypeNetworkDelay))

	if err := reg.MarkReverted(id); err != nil {
		t.Fatalf("MarkReverted failed: %v", err)
	}

	all := reg.ListAll()
	if !all[0].Reverted {
		t.Error("expected record to be marked as reverted")
	}
	if all[0].RevertedAt == nil {
		t.Error("expected RevertedAt to be set")
	}
}

func TestMarkReverted_Idempotent(t *testing.T) {
	reg, _ := newTempRegistry(t)
	id, _ := reg.Write(makeRecord("svc-a", FaultTypeNetworkDelay))
	_ = reg.MarkReverted(id)

	// Second call must not return an error
	if err := reg.MarkReverted(id); err != nil {
		t.Errorf("second MarkReverted should be idempotent, got: %v", err)
	}
}

func TestMarkReverted_NotFound(t *testing.T) {
	reg, _ := newTempRegistry(t)
	err := reg.MarkReverted("non-existent-id")
	if err == nil {
		t.Error("expected error for non-existent ID")
	}
}

// --- ListActive tests ---

func TestListActive_FiltersReverted(t *testing.T) {
	reg, _ := newTempRegistry(t)

	id1, _ := reg.Write(makeRecord("svc-a", FaultTypeNetworkDelay))
	_, _ = reg.Write(makeRecord("svc-b", FaultTypeNetworkLoss))
	_ = reg.MarkReverted(id1)

	active := reg.ListActive()
	if len(active) != 1 {
		t.Errorf("expected 1 active record, got %d", len(active))
	}
	if active[0].Target != "svc-b" {
		t.Errorf("expected active target to be svc-b, got %s", active[0].Target)
	}
}

func TestListActive_ReturnsCopy(t *testing.T) {
	reg, _ := newTempRegistry(t)
	_, _ = reg.Write(makeRecord("svc-a", FaultTypeNetworkDelay))

	active := reg.ListActive()
	active[0].Target = "mutated"

	// Original should be unchanged
	active2 := reg.ListActive()
	if active2[0].Target == "mutated" {
		t.Error("ListActive should return a copy, not a reference")
	}
}

// --- GarbageCollect tests ---

func TestGarbageCollect(t *testing.T) {
	reg, _ := newTempRegistry(t)
	id1, _ := reg.Write(makeRecord("svc-a", FaultTypeNetworkDelay))
	_, _ = reg.Write(makeRecord("svc-b", FaultTypeNetworkLoss))
	_ = reg.MarkReverted(id1)

	n, err := reg.GarbageCollect()
	if err != nil {
		t.Fatalf("GarbageCollect failed: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 record removed, got %d", n)
	}
	if len(reg.ListAll()) != 1 {
		t.Errorf("expected 1 remaining record, got %d", len(reg.ListAll()))
	}
}

func TestGarbageCollect_NoOp(t *testing.T) {
	reg, _ := newTempRegistry(t)
	_, _ = reg.Write(makeRecord("svc-a", FaultTypeNetworkDelay))

	n, err := reg.GarbageCollect()
	if err != nil {
		t.Fatalf("GarbageCollect failed: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 removed from GC with no reverted records, got %d", n)
	}
}

// --- IsExpired tests ---

func TestIsExpired_NoExpiry(t *testing.T) {
	rec := FaultRecord{}
	if rec.IsExpired() {
		t.Error("record with no ExpiresAt should not be expired")
	}
}

func TestIsExpired_Future(t *testing.T) {
	future := time.Now().UTC().Add(time.Hour)
	rec := FaultRecord{ExpiresAt: &future}
	if rec.IsExpired() {
		t.Error("record with future ExpiresAt should not be expired")
	}
}

func TestIsExpired_Past(t *testing.T) {
	past := time.Now().UTC().Add(-time.Hour)
	rec := FaultRecord{ExpiresAt: &past}
	if !rec.IsExpired() {
		t.Error("record with past ExpiresAt should be expired")
	}
}

// --- RecoverOrphans tests ---

// mockReverter implements the Reverter interface for testing recovery logic.
type mockReverter struct {
	execCalls   []string // captures ExecCommand target calls
	updateCalls []string // captures UpdateContainerResources target calls
	execErr     error
	updateErr   error
}

func (m *mockReverter) ExecCommand(_ context.Context, target string, _ []string) (int, error) {
	m.execCalls = append(m.execCalls, target)
	if m.execErr != nil {
		return 1, m.execErr
	}
	return 0, nil
}

func (m *mockReverter) UpdateContainerResources(_ context.Context, target string, _, _, _ int64) (any, error) {
	m.updateCalls = append(m.updateCalls, target)
	if m.updateErr != nil {
		return nil, m.updateErr
	}
	return nil, nil
}

func TestRecoverOrphans_RevertsNetworkFault(t *testing.T) {
	reg, _ := newTempRegistry(t)
	_, _ = reg.Write(makeRecord("svc-a", FaultTypeNetworkDelay))

	mock := &mockReverter{}
	results := reg.RecoverOrphans(context.Background(), []string{"svc-a"}, mock, "eth0")

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Reverted {
		t.Error("expected fault to be marked as reverted")
	}
	if len(mock.execCalls) != 1 || mock.execCalls[0] != "svc-a" {
		t.Errorf("expected 1 ExecCommand call on svc-a, got %v", mock.execCalls)
	}

	// Verify registry is updated
	if len(reg.ListActive()) != 0 {
		t.Error("expected no active faults after recovery")
	}
}

func TestRecoverOrphans_SkipsTargetsNotInConfig(t *testing.T) {
	reg, _ := newTempRegistry(t)
	_, _ = reg.Write(makeRecord("svc-old", FaultTypeNetworkDelay)) // NOT in allowed list

	mock := &mockReverter{}
	results := reg.RecoverOrphans(context.Background(), []string{"svc-a"}, mock, "eth0")

	if len(results) != 0 {
		t.Errorf("expected 0 results (target not in config), got %d", len(results))
	}
	if len(mock.execCalls) != 0 {
		t.Error("expected no ExecCommand calls for excluded targets")
	}
	// Record should still be active (we didn't revert it)
	if len(reg.ListActive()) != 1 {
		t.Error("out-of-scope orphan should remain active in registry")
	}
}

func TestRecoverOrphans_RevertsCPUFault(t *testing.T) {
	reg, _ := newTempRegistry(t)
	_, _ = reg.Write(makeRecord("svc-a", FaultTypeCPULimit))

	mock := &mockReverter{}
	results := reg.RecoverOrphans(context.Background(), []string{"svc-a"}, mock, "eth0")

	if len(results) != 1 || !results[0].Reverted {
		t.Errorf("expected 1 reverted result, got %+v", results)
	}
	if len(mock.updateCalls) != 1 || mock.updateCalls[0] != "svc-a" {
		t.Errorf("expected 1 UpdateContainerResources call on svc-a, got %v", mock.updateCalls)
	}
}

func TestRecoverOrphans_NoOrphans(t *testing.T) {
	reg, _ := newTempRegistry(t)

	mock := &mockReverter{}
	results := reg.RecoverOrphans(context.Background(), []string{"svc-a"}, mock, "eth0")

	if results != nil {
		t.Errorf("expected nil results for empty registry, got %v", results)
	}
}

func TestRecoverOrphans_SkipsAlreadyReverted(t *testing.T) {
	reg, _ := newTempRegistry(t)
	id, _ := reg.Write(makeRecord("svc-a", FaultTypeNetworkDelay))
	_ = reg.MarkReverted(id)

	mock := &mockReverter{}
	results := reg.RecoverOrphans(context.Background(), []string{"svc-a"}, mock, "eth0")

	if len(results) != 0 {
		t.Errorf("expected 0 results for already-reverted record, got %d", len(results))
	}
}
