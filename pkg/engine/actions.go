package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ibrahimkizilarslan/entropy/pkg/config"
	"github.com/ibrahimkizilarslan/entropy/pkg/registry"
)

// runtimeTypeName returns a stable string identifier for a ContainerRuntime instance.
// Used when persisting FaultRecords to the registry.
func runtimeTypeName(rt ContainerRuntime) string {
	switch rt.(type) {
	case *KubernetesClient:
		return "kubernetes"
	default:
		return "docker"
	}
}

type ResourceChaosManager struct {
	mu     sync.Mutex
	timers map[string]*time.Timer

	// registry is the persistent fault store. May be nil if not configured.
	registry        *registry.FaultRegistry
	activeRecordIDs map[string]string // target name → registry record ID
}

func NewResourceChaosManager() *ResourceChaosManager {
	return &ResourceChaosManager{
		timers:          make(map[string]*time.Timer),
		activeRecordIDs: make(map[string]string),
	}
}

// SetRegistry attaches a FaultRegistry to this manager.
// Must be called before any chaos injection if crash-recovery is desired.
func (m *ResourceChaosManager) SetRegistry(r *registry.FaultRegistry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.registry = r
}

func (m *ResourceChaosManager) ScheduleRestore(client ContainerRuntime, target string, faultType registry.FaultType, duration int, cpuQuota, cpuPeriod, memLimit int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if t, ok := m.timers[target]; ok {
		t.Stop()
	}

	// Persist the fault to the registry BEFORE scheduling the timer
	if m.registry != nil {
		expiresAt := time.Now().UTC().Add(time.Duration(duration) * time.Second)
		params := map[string]any{
			"cpu_quota":  cpuQuota,
			"cpu_period": cpuPeriod,
			"mem_limit":  memLimit,
		}
		recordID, err := m.registry.Write(registry.FaultRecord{
			FaultType: faultType,
			Target:    target,
			Runtime:   runtimeTypeName(client),
			ExpiresAt: &expiresAt,
			Params:    params,
		})
		if err == nil {
			m.activeRecordIDs[target] = recordID
		}
	}

	m.timers[target] = time.AfterFunc(time.Duration(duration)*time.Second, func() {
		m.mu.Lock()
		delete(m.timers, target)
		m.mu.Unlock()
		// Use a background context for timer-triggered restores since no caller context exists
		_, _ = client.UpdateContainerResources(context.Background(), target, 0, 0, 0)
		// Mark the registry record as reverted after successful restore
		if m.registry != nil {
			m.mu.Lock()
			id, ok := m.activeRecordIDs[target]
			if ok {
				delete(m.activeRecordIDs, target)
			}
			m.mu.Unlock()
			if ok {
				_ = m.registry.MarkReverted(id)
			}
		}
	})
}

func (m *ResourceChaosManager) ClearAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, t := range m.timers {
		t.Stop()
		delete(m.timers, k)
	}
	// Mark any remaining resource records as reverted on graceful shutdown
	if m.registry != nil {
		for target, id := range m.activeRecordIDs {
			_ = m.registry.MarkReverted(id)
			delete(m.activeRecordIDs, target)
		}
	}
}

type ActionHandler func(ctx context.Context, client ContainerRuntime, target string, spec config.ActionSpec) (*ContainerInfo, error)

var actionHandlers = map[string]ActionHandler{
	"stop":         actionStop,
	"restart":      actionRestart,
	"pause":        actionPause,
	"delay":        actionDelay,
	"loss":         actionLoss,
	"limit_cpu":    actionLimitCPU,
	"limit_memory": actionLimitMemory,
}

// GetSupportedActions returns a list of all action names supported by the engine.
func GetSupportedActions() []string {
	actions := make([]string, 0, len(actionHandlers))
	for k := range actionHandlers {
		actions = append(actions, k)
	}
	return actions
}

// GetActionHandler returns the handler for a given action name.
func GetActionHandler(name string) (ActionHandler, bool) {
	handler, ok := actionHandlers[name]
	return handler, ok
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
		client.ScheduleResourceRestore(ctx, target, registry.FaultTypeCPULimit, *spec.Duration, quota, period, 0)
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
		client.ScheduleResourceRestore(ctx, target, registry.FaultTypeMemoryLimit, *spec.Duration, 0, 0, memBytes)
	}
	return info, nil
}

func Dispatch(ctx context.Context, action config.ActionSpec, client ContainerRuntime, target string) (*ContainerInfo, error) {
	handler, ok := actionHandlers[action.Name]
	if !ok {
		return nil, fmt.Errorf("unknown action '%s'", action.Name)
	}
	return handler(ctx, client, target, action)
}
