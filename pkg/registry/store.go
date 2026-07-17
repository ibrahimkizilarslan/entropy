package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// defaultRegistryPath returns the default path for the fault registry file.
// It resolves to $HOME/.entropy/registry.json, or falls back to
// ./.entropy/registry.json if the home directory is not available.
func defaultRegistryPath() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".entropy", "registry.json")
	}
	return filepath.Join(".entropy", "registry.json")
}

// registryFile is the on-disk serialization format.
type registryFile struct {
	Version int            `json:"version"`
	Records []*FaultRecord `json:"records"`
}

const registryVersion = 1

// Open loads (or creates) a FaultRegistry at the given path.
// If path is empty, the default path ($HOME/.entropy/registry.json) is used.
// If the file does not exist yet, an empty registry is returned.
func Open(path string) (*FaultRegistry, error) {
	if path == "" {
		path = defaultRegistryPath()
	}

	// Ensure the parent directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("registry: cannot create directory for registry file: %w", err)
	}

	r := &FaultRegistry{
		path:    path,
		records: make(map[string]*FaultRecord),
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// First run — start with an empty registry
		return r, nil
	}
	if err != nil {
		return nil, fmt.Errorf("registry: failed to read registry file at %s: %w", path, err)
	}

	var f registryFile
	if err := json.Unmarshal(data, &f); err != nil {
		// Corrupted file — log and start fresh to avoid blocking the engine.
		// The corrupted file is renamed for forensic inspection.
		corruptPath := path + ".corrupt"
		_ = os.Rename(path, corruptPath)
		fmt.Fprintf(os.Stderr, "[registry] WARNING: corrupt registry file, starting fresh. Original saved to %s\n", corruptPath)
		return r, nil
	}

	for _, rec := range f.Records {
		r.records[rec.ID] = rec
	}

	return r, nil
}

// Write adds or updates a FaultRecord in the registry and persists the change atomically.
// If record.ID is empty, a new UUID is generated and assigned.
// Returns the final record ID.
func (r *FaultRegistry) Write(record FaultRecord) (string, error) {
	if record.ID == "" {
		record.ID = uuid.NewString()
	}
	if record.InjectedAt.IsZero() {
		record.InjectedAt = time.Now().UTC()
	}

	r.mu.Lock()
	r.records[record.ID] = &record
	r.mu.Unlock()

	if err := r.persist(); err != nil {
		// Roll back the in-memory write on persistence failure so state stays consistent.
		r.mu.Lock()
		delete(r.records, record.ID)
		r.mu.Unlock()
		return "", fmt.Errorf("registry: failed to persist after Write: %w", err)
	}

	return record.ID, nil
}

// MarkReverted marks a fault record as successfully reverted and persists the change.
// It is idempotent: calling it on an already-reverted record is a no-op.
func (r *FaultRegistry) MarkReverted(id string) error {
	r.mu.Lock()
	rec, ok := r.records[id]
	if !ok {
		r.mu.Unlock()
		return fmt.Errorf("registry: record %q not found", id)
	}
	if rec.Reverted {
		r.mu.Unlock()
		return nil // idempotent
	}
	now := time.Now().UTC()
	rec.Reverted = true
	rec.RevertedAt = &now
	r.mu.Unlock()

	return r.persist()
}

// ListActive returns all FaultRecords that have not yet been reverted.
// The returned slice is a deep copy; mutations do not affect the registry state.
func (r *FaultRegistry) ListActive() []FaultRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var active []FaultRecord
	for _, rec := range r.records {
		if !rec.Reverted {
			cp := *rec
			active = append(active, cp)
		}
	}
	return active
}

// ListAll returns every FaultRecord, including already-reverted ones.
// Intended for the `entropy registry list` CLI command.
func (r *FaultRegistry) ListAll() []FaultRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()

	all := make([]FaultRecord, 0, len(r.records))
	for _, rec := range r.records {
		all = append(all, *rec)
	}
	return all
}

// GarbageCollect removes all records that have been reverted, freeing disk space.
// Returns the number of records removed.
func (r *FaultRegistry) GarbageCollect() (int, error) {
	r.mu.Lock()
	removed := 0
	for id, rec := range r.records {
		if rec.Reverted {
			delete(r.records, id)
			removed++
		}
	}
	r.mu.Unlock()

	if removed == 0 {
		return 0, nil
	}

	if err := r.persist(); err != nil {
		return 0, fmt.Errorf("registry: GarbageCollect persist failed: %w", err)
	}
	return removed, nil
}

// Path returns the absolute path of the registry file on disk.
func (r *FaultRegistry) Path() string {
	return r.path
}

// persist writes the in-memory state to disk atomically using the
// write-temp → fsync → rename pattern. This guarantees that the registry
// file is never left in a partially-written (corrupt) state after a crash.
//
// persistMu serializes the whole write-temp/fsync/rename sequence across
// concurrent callers (see the FaultRegistry.persistMu doc comment): without
// it, two concurrent persist() calls racing on the same temp file path could
// let one call's rename win with a snapshot that is missing the other
// call's already-committed in-memory record.
func (r *FaultRegistry) persist() error {
	r.persistMu.Lock()
	defer r.persistMu.Unlock()

	r.mu.RLock()
	records := make([]*FaultRecord, 0, len(r.records))
	for _, rec := range r.records {
		cp := *rec
		records = append(records, &cp)
	}
	r.mu.RUnlock()

	f := registryFile{
		Version: registryVersion,
		Records: records,
	}

	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("registry: JSON marshal failed: %w", err)
	}

	// Write to a temp file alongside the real file
	tmpPath := r.path + ".tmp"
	tmpFile, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("registry: failed to open temp file for write: %w", err)
	}

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("registry: failed to write data to temp file: %w", err)
	}

	// fsync before rename: guarantees data hits disk before the atomic rename
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("registry: fsync failed: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("registry: failed to close temp file: %w", err)
	}

	// Atomic rename: on Linux/macOS this is guaranteed to be atomic
	if err := os.Rename(tmpPath, r.path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("registry: atomic rename failed: %w", err)
	}

	return nil
}
