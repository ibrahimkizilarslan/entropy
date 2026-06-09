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
	mu       sync.Mutex
	active   map[string]ContainerRuntime // container name → runtime used for injection
	timers   map[string]*time.Timer
	netIface string // configurable network interface (default: eth0)

	// registry is the persistent fault store. May be nil if not configured.
	registry         *registry.FaultRegistry
	activeRecordIDs  map[string]string // container name → registry record ID
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
	}
}

// SetRegistry attaches a FaultRegistry to this manager.
// Must be called before any chaos injection if crash-recovery is desired.
func (m *NetworkChaosManager) SetRegistry(r *registry.FaultRegistry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.registry = r
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

func (m *NetworkChaosManager) cancelTimer(containerName string) {
	if t, ok := m.timers[containerName]; ok {
		t.Stop()
		delete(m.timers, containerName)
	}
}

func (m *NetworkChaosManager) applyRule(ctx context.Context, runtime ContainerRuntime, name string, faultType registry.FaultType, faultParams map[string]any, tcArgs []string, duration *int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// If there's an existing rule for this container, remove it first
	if existingRT, exists := m.active[name]; exists {
		m.cancelTimer(name)
		_ = m.execTc(ctx, existingRT, name, []string{"qdisc", "del", "dev", m.netIface, "root"})
		// Mark previous registry record as reverted (replaced by new injection)
		if m.registry != nil {
			if prevID, ok := m.activeRecordIDs[name]; ok {
				_ = m.registry.MarkReverted(prevID)
				delete(m.activeRecordIDs, name)
			}
		}
	}

	// Persist the fault BEFORE injection so a crash during injection is recoverable
	if m.registry != nil {
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
		recordID, err := m.registry.Write(registry.FaultRecord{
			FaultType: faultType,
			Target:    name,
			Runtime:   runtimeTypeName(runtime),
			ExpiresAt: expiresAt,
			Params:    params,
		})
		if err == nil {
			m.activeRecordIDs[name] = recordID
		}
	}

	addArgs := append([]string{"qdisc", "add", "dev", m.netIface, "root"}, tcArgs...)
	if err := m.execTc(ctx, runtime, name, addArgs); err != nil {
		// Injection failed — mark the registry record as reverted (cleanup)
		if m.registry != nil {
			if id, ok := m.activeRecordIDs[name]; ok {
				_ = m.registry.MarkReverted(id)
				delete(m.activeRecordIDs, name)
			}
		}
		return fmt.Errorf("network chaos injection failed for '%s': %w", name, err)
	}

	m.active[name] = runtime

	if duration != nil && *duration > 0 {
		clearName := name
		m.timers[name] = time.AfterFunc(time.Duration(*duration)*time.Second, func() {
			m.Clear(clearName)
		})
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
func (m *NetworkChaosManager) Clear(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cancelTimer(name)
	if runtime, exists := m.active[name]; exists {
		_ = m.execTc(context.Background(), runtime, name, []string{"qdisc", "del", "dev", m.netIface, "root"})
		delete(m.active, name)
	}
	// Mark registry record as reverted after successful clear
	if m.registry != nil {
		if id, ok := m.activeRecordIDs[name]; ok {
			_ = m.registry.MarkReverted(id)
			delete(m.activeRecordIDs, name)
		}
	}
}

// ClearAll removes all active network chaos rules across all containers.
func (m *NetworkChaosManager) ClearAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name := range m.timers {
		if t, ok := m.timers[name]; ok {
			t.Stop()
		}
	}
	m.timers = make(map[string]*time.Timer)

	for name, runtime := range m.active {
		_ = m.execTc(context.Background(), runtime, name, []string{"qdisc", "del", "dev", m.netIface, "root"})
	}
	m.active = make(map[string]ContainerRuntime)

	// Mark all network records as reverted
	if m.registry != nil {
		for name, id := range m.activeRecordIDs {
			_ = m.registry.MarkReverted(id)
			delete(m.activeRecordIDs, name)
		}
	}
}
