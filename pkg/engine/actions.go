package engine

import (
	"context"
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
		// Use a background context for timer-triggered restores since no caller context exists
		_, _ = client.UpdateContainerResources(context.Background(), target, 0, 0, 0)
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

type ActionHandler func(ctx context.Context, client ContainerRuntime, target string, spec config.ActionSpec) (*ContainerInfo, error)

var ActionHandlers = map[string]ActionHandler{
	"stop":         actionStop,
	"restart":      actionRestart,
	"pause":        actionPause,
	"delay":        actionDelay,
	"loss":         actionLoss,
	"limit_cpu":    actionLimitCPU,
	"limit_memory": actionLimitMemory,
}

func actionStop(ctx context.Context, client ContainerRuntime, target string, spec config.ActionSpec) (*ContainerInfo, error) {
	return client.StopContainer(ctx, target, 10)
}

func actionRestart(ctx context.Context, client ContainerRuntime, target string, spec config.ActionSpec) (*ContainerInfo, error) {
	return client.RestartContainer(ctx, target, 10)
}

func actionPause(ctx context.Context, client ContainerRuntime, target string, spec config.ActionSpec) (*ContainerInfo, error) {
	return client.PauseContainer(ctx, target)
}

func actionDelay(ctx context.Context, client ContainerRuntime, target string, spec config.ActionSpec) (*ContainerInfo, error) {
	if err := client.InjectNetworkDelay(ctx, target, spec.LatencyMs, spec.JitterMs, spec.Duration); err != nil {
		return nil, err
	}
	return &ContainerInfo{Name: target, Status: "running (delayed)"}, nil
}

func actionLoss(ctx context.Context, client ContainerRuntime, target string, spec config.ActionSpec) (*ContainerInfo, error) {
	if err := client.InjectNetworkLoss(ctx, target, spec.LossPercent, spec.Duration); err != nil {
		return nil, err
	}
	return &ContainerInfo{Name: target, Status: "running (lossy)"}, nil
}

func actionLimitCPU(ctx context.Context, client ContainerRuntime, target string, spec config.ActionSpec) (*ContainerInfo, error) {
	period := int64(100000)
	quota := int64(spec.CPUs * float64(period))
	info, err := client.UpdateContainerResources(ctx, target, quota, period, 0)
	if err != nil {
		return nil, err
	}
	if spec.Duration != nil && *spec.Duration > 0 {
		ResourceManager.ScheduleRestore(client, target, *spec.Duration)
	}
	return info, nil
}

func actionLimitMemory(ctx context.Context, client ContainerRuntime, target string, spec config.ActionSpec) (*ContainerInfo, error) {
	memBytes := int64(spec.MemoryMB) * 1024 * 1024
	info, err := client.UpdateContainerResources(ctx, target, 0, 0, memBytes)
	if err != nil {
		return nil, err
	}
	if spec.Duration != nil && *spec.Duration > 0 {
		ResourceManager.ScheduleRestore(client, target, *spec.Duration)
	}
	return info, nil
}

func Dispatch(ctx context.Context, action config.ActionSpec, client ContainerRuntime, target string) (*ContainerInfo, error) {
	handler, ok := ActionHandlers[action.Name]
	if !ok {
		return nil, fmt.Errorf("unknown action '%s'", action.Name)
	}
	return handler(ctx, client, target, action)
}

func CleanupAll() {
	NetworkManager.ClearAll()
	ResourceManager.ClearAll()
}
