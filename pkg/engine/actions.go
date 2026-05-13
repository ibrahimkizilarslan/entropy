package engine

import (
	"fmt"
	"sync"
	"time"

	"github.com/ibrahimkizilarslan/entropy/pkg/config"
)

var (
	NetworkManager  = NewNetworkChaosManager()
	ResourceManager = NewResourceChaosManager()
)

type ResourceChaosManager struct {
	mu     sync.Mutex
	timers map[string]*time.Timer
}

func NewResourceChaosManager() *ResourceChaosManager {
	return &ResourceChaosManager{
		timers: make(map[string]*time.Timer),
	}
}

func (m *ResourceChaosManager) ScheduleRestore(client ContainerRuntime, target string, duration int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if t, ok := m.timers[target]; ok {
		t.Stop()
	}

	m.timers[target] = time.AfterFunc(time.Duration(duration)*time.Second, func() {
		m.mu.Lock()
		delete(m.timers, target)
		m.mu.Unlock()
		_, _ = client.UpdateContainerResources(target, 0, 0, 0)
	})
}

func (m *ResourceChaosManager) ClearAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, t := range m.timers {
		t.Stop()
		delete(m.timers, k)
	}
}

type ActionHandler func(client ContainerRuntime, target string, spec config.ActionSpec) (*ContainerInfo, error)

var ActionHandlers = map[string]ActionHandler{
	"stop":         actionStop,
	"restart":      actionRestart,
	"pause":        actionPause,
	"delay":        actionDelay,
	"loss":         actionLoss,
	"limit_cpu":    actionLimitCPU,
	"limit_memory": actionLimitMemory,
}

func actionStop(client ContainerRuntime, target string, spec config.ActionSpec) (*ContainerInfo, error) {
	return client.StopContainer(target, 10)
}

func actionRestart(client ContainerRuntime, target string, spec config.ActionSpec) (*ContainerInfo, error) {
	return client.RestartContainer(target, 10)
}

func actionPause(client ContainerRuntime, target string, spec config.ActionSpec) (*ContainerInfo, error) {
	return client.PauseContainer(target)
}

func actionDelay(client ContainerRuntime, target string, spec config.ActionSpec) (*ContainerInfo, error) {
	pid, err := client.GetContainerPID(target)
	if err != nil {
		return nil, err
	}
	if err := NetworkManager.InjectDelay(target, pid, spec.LatencyMs, spec.JitterMs, spec.Duration); err != nil {
		return nil, err
	}
	// For network chaos, we just return empty info since ContainerInfo isn't strictly needed 
	// or we can fetch it. For now, returning empty is safe for the CLI output.
	return &ContainerInfo{Name: target, Status: "running (delayed)"}, nil
}

func actionLoss(client ContainerRuntime, target string, spec config.ActionSpec) (*ContainerInfo, error) {
	pid, err := client.GetContainerPID(target)
	if err != nil {
		return nil, err
	}
	if err := NetworkManager.InjectLoss(target, pid, spec.LossPercent, spec.Duration); err != nil {
		return nil, err
	}
	return &ContainerInfo{Name: target, Status: "running (lossy)"}, nil
}

func actionLimitCPU(client ContainerRuntime, target string, spec config.ActionSpec) (*ContainerInfo, error) {
	period := int64(100000)
	quota := int64(spec.CPUs * float64(period))
	info, err := client.UpdateContainerResources(target, quota, period, 0)
	if err != nil {
		return nil, err
	}
	if spec.Duration != nil && *spec.Duration > 0 {
		ResourceManager.ScheduleRestore(client, target, *spec.Duration)
	}
	return info, nil
}

func actionLimitMemory(client ContainerRuntime, target string, spec config.ActionSpec) (*ContainerInfo, error) {
	memBytes := int64(spec.MemoryMB) * 1024 * 1024
	info, err := client.UpdateContainerResources(target, 0, 0, memBytes)
	if err != nil {
		return nil, err
	}
	if spec.Duration != nil && *spec.Duration > 0 {
		ResourceManager.ScheduleRestore(client, target, *spec.Duration)
	}
	return info, nil
}

func Dispatch(action config.ActionSpec, client ContainerRuntime, target string) (*ContainerInfo, error) {
	handler, ok := ActionHandlers[action.Name]
	if !ok {
		return nil, fmt.Errorf("unknown action '%s'", action.Name)
	}
	return handler(client, target, action)
}

func CleanupAll() {
	NetworkManager.ClearAll()
	ResourceManager.ClearAll()
}
