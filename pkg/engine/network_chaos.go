package engine

import (
	"fmt"
	"os"
	"regexp"
	"sync"
	"time"
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
}

func NewNetworkChaosManager() *NetworkChaosManager {
	iface := os.Getenv("ENTROPY_NET_INTERFACE")
	if iface == "" {
		iface = "eth0"
	}
	return &NetworkChaosManager{
		active:   make(map[string]ContainerRuntime),
		timers:   make(map[string]*time.Timer),
		netIface: iface,
	}
}

// execTc runs a tc command inside the target container via the runtime's Exec API.
// This eliminates the need for host-level sudo/nsenter privileges entirely.
func (m *NetworkChaosManager) execTc(runtime ContainerRuntime, name string, args []string) error {
	cmd := append([]string{"tc"}, args...)
	exitCode, err := runtime.ExecCommand(name, cmd)
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

func (m *NetworkChaosManager) applyRule(runtime ContainerRuntime, name string, args []string, duration *int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// If there's an existing rule for this container, remove it first
	if existingRT, exists := m.active[name]; exists {
		m.cancelTimer(name)
		_ = m.execTc(existingRT, name, []string{"qdisc", "del", "dev", m.netIface, "root"})
	}

	addArgs := append([]string{"qdisc", "add", "dev", m.netIface, "root"}, args...)
	if err := m.execTc(runtime, name, addArgs); err != nil {
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
func (m *NetworkChaosManager) InjectDelay(runtime ContainerRuntime, name string, latencyMs int, jitterMs int, duration *int) error {
	if err := validateContainerName(name); err != nil {
		return err
	}
	args := []string{"netem", "delay", fmt.Sprintf("%dms", latencyMs)}
	if jitterMs > 0 {
		args = append(args, fmt.Sprintf("%dms", jitterMs), "distribution", "normal")
	}
	return m.applyRule(runtime, name, args, duration)
}

// InjectLoss injects packet loss into the target container using tc/netem via the container runtime exec API.
func (m *NetworkChaosManager) InjectLoss(runtime ContainerRuntime, name string, percent int, duration *int) error {
	if err := validateContainerName(name); err != nil {
		return err
	}
	args := []string{"netem", "loss", fmt.Sprintf("%d%%", percent)}
	return m.applyRule(runtime, name, args, duration)
}

// Clear removes active network chaos rules from a specific container.
func (m *NetworkChaosManager) Clear(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cancelTimer(name)
	if runtime, exists := m.active[name]; exists {
		_ = m.execTc(runtime, name, []string{"qdisc", "del", "dev", m.netIface, "root"})
		delete(m.active, name)
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
		_ = m.execTc(runtime, name, []string{"qdisc", "del", "dev", m.netIface, "root"})
	}
	m.active = make(map[string]ContainerRuntime)
}
