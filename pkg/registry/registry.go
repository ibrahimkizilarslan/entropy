package registry

import (
	"sync"
	"time"
)

// FaultType identifies the class of chaos injection.
// Each type maps directly to a specific revert operation.
type FaultType string

const (
	FaultTypeNetworkDelay FaultType = "network_delay"
	FaultTypeNetworkLoss  FaultType = "network_loss"
	FaultTypeCPULimit     FaultType = "cpu_limit"
	FaultTypeMemoryLimit  FaultType = "memory_limit"
)

// FaultRecord represents a single persisted chaos injection.
// It contains every field needed to REVERT the fault autonomously on crash-recovery,
// without re-running the original scenario file.
type FaultRecord struct {
	// ID is the globally unique identifier for this fault injection (UUID v4).
	ID string `json:"id"`

	// FaultType identifies the class of chaos for dispatch during recovery.
	FaultType FaultType `json:"fault_type"`

	// Target is the container name (Docker) or pod name (Kubernetes).
	Target string `json:"target"`

	// Runtime is either "docker" or "kubernetes".
	Runtime string `json:"runtime"`

	// Namespace is the Kubernetes namespace. Empty string for Docker runtimes.
	Namespace string `json:"namespace,omitempty"`

	// InjectedAt is the UTC timestamp of when this fault was first applied.
	InjectedAt time.Time `json:"injected_at"`

	// ExpiresAt is the optional UTC timestamp when the fault was scheduled to auto-revert.
	// A nil value means the fault has no automatic expiry (manual revert only).
	ExpiresAt *time.Time `json:"expires_at,omitempty"`

	// Params holds fault-specific parameters needed to reconstruct revert commands.
	// Examples:
	//   FaultTypeNetworkDelay: {"latency_ms": 300, "jitter_ms": 50, "iface": "eth0"}
	//   FaultTypeCPULimit:     {"cpu_quota": 50000, "cpu_period": 100000}
	Params map[string]any `json:"params"`

	// Reverted is true once the fault has been successfully cleaned up.
	// Records with Reverted=true are eligible for garbage collection.
	Reverted bool `json:"reverted"`

	// RevertedAt is the UTC timestamp of successful revert. Nil if not yet reverted.
	RevertedAt *time.Time `json:"reverted_at,omitempty"`
}

// IsExpired reports whether this fault's scheduled auto-revert time has passed.
func (r *FaultRecord) IsExpired() bool {
	if r.ExpiresAt == nil {
		return false
	}
	return time.Now().UTC().After(*r.ExpiresAt)
}

// FaultRegistry is the in-memory state store backed by a persistent JSON file.
// All public methods are safe for concurrent use.
type FaultRegistry struct {
	mu      sync.RWMutex
	path    string
	records map[string]*FaultRecord // ID → *FaultRecord
}
