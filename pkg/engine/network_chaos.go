package engine

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/ibrahimkizilarslan/entropy/pkg/registry"
)

// safeNamePattern allows only safe characters in container names to prevent command injection
var safeNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

func validateContainerName(name string) error {
	if !safeNamePattern.MatchString(name) {
		return fmt.Errorf("invalid container name '%s': only alphanumeric, hyphens, underscores, and dots are allowed", name)
	}
	return nil
}

type NetworkChaosManager struct {
	// mu protects the maps below (active, timers, activeRecordIDs, targetLocks)
	// and the registry field. It is only ever held for short, in-memory
	// operations — never across an I/O call (tc exec, registry read/write).
	mu       sync.Mutex
	active   map[string]ContainerRuntime // container name → runtime used for injection
	timers   map[string]*time.Timer
	netIface string // configurable network interface (default: eth0)

	// registry is the persistent fault store. May be nil if not configured.
	registry        *registry.FaultRegistry
	activeRecordIDs map[string]string // container name → registry record ID

	// targetLocks holds one mutex per container name. Holding a target's lock
	// serializes injection/clear operations for THAT target only, so that a
	// slow tc exec or registry write for one container never blocks
	// operations on any other container. The map itself is protected by mu;
	// entries are never removed (the target set is bounded by chaos.yaml).
	targetLocks map[string]*sync.Mutex
}

func NewNetworkChaosManager() *NetworkChaosManager {
	iface := os.Getenv("ENTROPY_NET_INTERFACE")
	if iface == "" {
		iface = "eth0"
	}
	return &NetworkChaosManager{
		active:          make(map[string]ContainerRuntime),
		timers:          make(map[string]*time.Timer),
		netIface:        iface,
		activeRecordIDs: make(map[string]string),
		targetLocks:     make(map[string]*sync.Mutex),
	}
}

// SetRegistry attaches a FaultRegistry to this manager.
// Must be called before any chaos injection if crash-recovery is desired.
func (m *NetworkChaosManager) SetRegistry(r *registry.FaultRegistry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.registry = r
}

func (m *NetworkChaosManager) getRegistry() *registry.FaultRegistry {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.registry
}

// lockFor returns the per-target mutex for name, creating it on first use.
// Callers must hold the returned lock for the full duration of an
// injection/clear operation on that target, including any I/O — but must
// NOT hold mu while doing so.
func (m *NetworkChaosManager) lockFor(name string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.targetLocks[name]
	if !ok {
		l = &sync.Mutex{}
		m.targetLocks[name] = l
	}
	return l
}

// execTc runs a tc command inside the target container via the runtime's Exec API.
// This eliminates the need for host-level sudo/nsenter privileges entirely.
func (m *NetworkChaosManager) execTc(ctx context.Context, runtime ContainerRuntime, name string, args []string) error {
	cmd := append([]string{"tc"}, args...)
	exitCode, err := runtime.ExecCommand(ctx, name, cmd)
	if err != nil {
		return fmt.Errorf("tc command failed for '%s': %w\n  → Hint: ensure the target container has 'iproute2' installed and NET_ADMIN capability", name, err)
	}
	if exitCode != 0 {
		return fmt.Errorf("tc command returned exit code %d for '%s'", exitCode, name)
	}
	return nil
}

// applyRule installs a tc/netem rule on the target container. Concurrent
// calls for DIFFERENT targets proceed fully in parallel; concurrent calls for
// the SAME target are serialized via the per-target lock, since replacing a
// rule requires an atomic "remove old, then add new" sequence.
func (m *NetworkChaosManager) applyRule(ctx context.Context, runtime ContainerRuntime, name string, faultType registry.FaultType, faultParams map[string]any, tcArgs []string, duration *int) error {
	targetLock := m.lockFor(name)
	targetLock.Lock()
	defer targetLock.Unlock()

	reg := m.getRegistry()

	// If there's an existing rule for this container, remove it first.
	m.mu.Lock()
	existingRT, hadExisting := m.active[name]
	if t, ok := m.timers[name]; ok {
		t.Stop()
		delete(m.timers, name)
	}
	prevRecordID, hadPrevRecord := m.activeRecordIDs[name]
	m.mu.Unlock()

	if hadExisting {
		_ = m.execTc(ctx, existingRT, name, []string{"qdisc", "del", "dev", m.netIface, "root"})
		if reg != nil && hadPrevRecord {
			_ = reg.MarkReverted(prevRecordID)
			m.mu.Lock()
			delete(m.activeRecordIDs, name)
			m.mu.Unlock()
		}
	}

	// Persist the fault BEFORE injection so a crash during injection is recoverable.
	var recordID string
	if reg != nil {
		var expiresAt *time.Time
		if duration != nil && *duration > 0 {
			t := time.Now().UTC().Add(time.Duration(*duration) * time.Second)
			expiresAt = &t
		}
		params := make(map[string]any, len(faultParams)+1)
		for k, v := range faultParams {
			params[k] = v
		}
		params["iface"] = m.netIface
		var err error
		recordID, err = reg.Write(registry.FaultRecord{
			FaultType: faultType,
			Target:    name,
			Runtime:   runtimeTypeName(runtime),
			ExpiresAt: expiresAt,
			Params:    params,
		})
		if err == nil {
			m.mu.Lock()
			m.activeRecordIDs[name] = recordID
			m.mu.Unlock()
		}
	}

	addArgs := append([]string{"qdisc", "add", "dev", m.netIface, "root"}, tcArgs...)
	if err := m.execTc(ctx, runtime, name, addArgs); err != nil {
		// Injection failed — mark the registry record as reverted (cleanup)
		if reg != nil && recordID != "" {
			_ = reg.MarkReverted(recordID)
			m.mu.Lock()
			delete(m.activeRecordIDs, name)
			m.mu.Unlock()
		}
		return fmt.Errorf("network chaos injection failed for '%s': %w", name, err)
	}

	m.mu.Lock()
	m.active[name] = runtime
	m.mu.Unlock()

	if duration != nil && *duration > 0 {
		clearName := name
		timer := time.AfterFunc(time.Duration(*duration)*time.Second, func() {
			m.Clear(clearName)
		})
		m.mu.Lock()
		m.timers[name] = timer
		m.mu.Unlock()
	}
	return nil
}

// InjectDelay injects network latency into the target container using tc/netem via the container runtime exec API.
func (m *NetworkChaosManager) InjectDelay(ctx context.Context, runtime ContainerRuntime, name string, latencyMs int, jitterMs int, duration *int) error {
	if err := validateContainerName(name); err != nil {
		return err
	}
	tcArgs := []string{"netem", "delay", fmt.Sprintf("%dms", latencyMs)}
	if jitterMs > 0 {
		tcArgs = append(tcArgs, fmt.Sprintf("%dms", jitterMs), "distribution", "normal")
	}
	params := map[string]any{"latency_ms": latencyMs, "jitter_ms": jitterMs}
	return m.applyRule(ctx, runtime, name, registry.FaultTypeNetworkDelay, params, tcArgs, duration)
}

// InjectLoss injects packet loss into the target container using tc/netem via the container runtime exec API.
func (m *NetworkChaosManager) InjectLoss(ctx context.Context, runtime ContainerRuntime, name string, percent int, duration *int) error {
	if err := validateContainerName(name); err != nil {
		return err
	}
	tcArgs := []string{"netem", "loss", fmt.Sprintf("%d%%", percent)}
	params := map[string]any{"loss_pct": percent}
	return m.applyRule(ctx, runtime, name, registry.FaultTypeNetworkLoss, params, tcArgs, duration)
}

// Clear removes active network chaos rules from a specific container.
// Concurrent Clear/applyRule calls for other targets are unaffected.
func (m *NetworkChaosManager) Clear(name string) {
	targetLock := m.lockFor(name)
	targetLock.Lock()
	defer targetLock.Unlock()

	reg := m.getRegistry()

	m.mu.Lock()
	if t, ok := m.timers[name]; ok {
		t.Stop()
		delete(m.timers, name)
	}
	runtime, exists := m.active[name]
	m.mu.Unlock()

	if exists {
		_ = m.execTc(context.Background(), runtime, name, []string{"qdisc", "del", "dev", m.netIface, "root"})
		m.mu.Lock()
		delete(m.active, name)
		m.mu.Unlock()
	}

	if reg != nil {
		m.mu.Lock()
		id, ok := m.activeRecordIDs[name]
		if ok {
			delete(m.activeRecordIDs, name)
		}
		m.mu.Unlock()
		if ok {
			_ = reg.MarkReverted(id)
		}
	}
}

// ClearAll removes all active network chaos rules across all containers.
// Each target is cleared concurrently via Clear, so a slow revert for one
// container does not delay reverting the others.
func (m *NetworkChaosManager) ClearAll() {
	m.mu.Lock()
	names := make(map[string]struct{}, len(m.active))
	for name := range m.active {
		names[name] = struct{}{}
	}
	for name := range m.timers {
		names[name] = struct{}{}
	}
	m.mu.Unlock()

	var wg sync.WaitGroup
	for name := range names {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			m.Clear(n)
		}(name)
	}
	wg.Wait()
}
