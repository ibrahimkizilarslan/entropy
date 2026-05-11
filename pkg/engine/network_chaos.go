package engine

import (
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
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
	mu     sync.Mutex
	active map[string]int // container name → PID
	timers map[string]*time.Timer
}

func NewNetworkChaosManager() *NetworkChaosManager {
	return &NetworkChaosManager{
		active: make(map[string]int),
		timers: make(map[string]*time.Timer),
	}
}

func (m *NetworkChaosManager) checkPlatform() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("network chaos (delay/loss) requires a Linux host")
	}
	return nil
}

func (m *NetworkChaosManager) runTc(pid int, args []string) error {
	cmdArgs := append([]string{"nsenter", "-t", fmt.Sprintf("%d", pid), "-n", "--", "tc"}, args...)
	cmd := exec.Command("sudo", cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tc failed: %s", string(out))
	}
	return nil
}

func (m *NetworkChaosManager) cancelTimer(containerName string) {
	if t, ok := m.timers[containerName]; ok {
		t.Stop()
		delete(m.timers, containerName)
	}
}

func (m *NetworkChaosManager) applyRule(name string, pid int, args []string, duration *int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.active[name]; exists {
		m.cancelTimer(name)
		_ = m.runTc(pid, []string{"qdisc", "del", "dev", "eth0", "root"})
	}

	addArgs := append([]string{"qdisc", "add", "dev", "eth0", "root"}, args...)
	if err := m.runTc(pid, addArgs); err != nil {
		return fmt.Errorf("tc failed for container '%s': %w", name, err)
	}

	m.active[name] = pid

	if duration != nil && *duration > 0 {
		m.timers[name] = time.AfterFunc(time.Duration(*duration)*time.Second, func() {
			m.Clear(name, &pid)
		})
	}
	return nil
}

func (m *NetworkChaosManager) InjectDelay(name string, pid int, latencyMs int, jitterMs int, duration *int) error {
	if err := m.checkPlatform(); err != nil {
		return err
	}
	if err := validateContainerName(name); err != nil {
		return err
	}
	args := []string{"netem", "delay", fmt.Sprintf("%dms", latencyMs)}
	if jitterMs > 0 {
		args = append(args, fmt.Sprintf("%dms", jitterMs), "distribution", "normal")
	}
	return m.applyRule(name, pid, args, duration)
}

func (m *NetworkChaosManager) InjectLoss(name string, pid int, percent int, duration *int) error {
	if err := m.checkPlatform(); err != nil {
		return err
	}
	if err := validateContainerName(name); err != nil {
		return err
	}
	args := []string{"netem", "loss", fmt.Sprintf("%d%%", percent)}
	return m.applyRule(name, pid, args, duration)
}

func (m *NetworkChaosManager) Clear(name string, pid *int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cancelTimer(name)
	if storedPID, exists := m.active[name]; exists {
		cleanupPID := storedPID
		if pid != nil {
			cleanupPID = *pid
		}
		_ = m.runTc(cleanupPID, []string{"qdisc", "del", "dev", "eth0", "root"})
		delete(m.active, name)
	}
}

func (m *NetworkChaosManager) ClearAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name := range m.timers {
		m.cancelTimer(name)
	}
	for name, pid := range m.active {
		_ = m.runTc(pid, []string{"qdisc", "del", "dev", "eth0", "root"})
		delete(m.active, name)
	}
}
