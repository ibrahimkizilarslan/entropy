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
	// mu protects timers, activeRecordIDs, registry, and targetLocks. It is
	// only ever held for short, in-memory operations — never across an I/O
	// call (registry.Write, registry.MarkReverted, UpdateContainerResources).
	mu     sync.Mutex
	timers map[string]*time.Timer

	// registry is the persistent fault store. May be nil if not configured.
	registry        *registry.FaultRegistry
	activeRecordIDs map[string]string // target name → registry record ID

	// targetLocks holds one mutex per target. Holding a target's lock
	// serializes ScheduleRestore calls for THAT target only, so a slow
	// registry write for one target never blocks operations on another.
	targetLocks map[string]*sync.Mutex
}

func NewResourceChaosManager() *ResourceChaosManager {
	return &ResourceChaosManager{
		timers:          make(map[string]*time.Timer),
		activeRecordIDs: make(map[string]string),
		targetLocks:     make(map[string]*sync.Mutex),
	}
}

// SetRegistry attaches a FaultRegistry to this manager.
// Must be called before any chaos injection if crash-recovery is desired.
func (m *ResourceChaosManager) SetRegistry(r *registry.FaultRegistry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.registry = r
}

func (m *ResourceChaosManager) getRegistry() *registry.FaultRegistry {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.registry
}

// lockFor returns the per-target mutex for target, creating it on first use.
func (m *ResourceChaosManager) lockFor(target string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.targetLocks[target]
	if !ok {
		l = &sync.Mutex{}
		m.targetLocks[target] = l
	}
	return l
}

func (m *ResourceChaosManager) ScheduleRestore(client ContainerRuntime, target string, faultType registry.FaultType, duration int, cpuQuota, cpuPeriod, memLimit int64) {
	targetLock := m.lockFor(target)
	targetLock.Lock()
	defer targetLock.Unlock()

	m.mu.Lock()
	if t, ok := m.timers[target]; ok {
		t.Stop()
		delete(m.timers, target)
	}
	m.mu.Unlock()

	reg := m.getRegistry()

	// Persist the fault to the registry BEFORE scheduling the timer
	if reg != nil {
		expiresAt := time.Now().UTC().Add(time.Duration(duration) * time.Second)
		params := map[string]any{
			"cpu_quota":  cpuQuota,
			"cpu_period": cpuPeriod,
			"mem_limit":  memLimit,
		}
		recordID, err := reg.Write(registry.FaultRecord{
			FaultType: faultType,
			Target:    target,
			Runtime:   runtimeTypeName(client),
			ExpiresAt: &expiresAt,
			Params:    params,
		})
		if err == nil {
			m.mu.Lock()
			m.activeRecordIDs[target] = recordID
			m.mu.Unlock()
		}
	}

	timer := time.AfterFunc(time.Duration(duration)*time.Second, func() {
		m.mu.Lock()
		delete(m.timers, target)
		m.mu.Unlock()
		// Use a background context for timer-triggered restores since no caller context exists
		_, _ = client.UpdateContainerResources(context.Background(), target, 0, 0, 0)
		// Mark the registry record as reverted after successful restore
		m.mu.Lock()
		id, ok := m.activeRecordIDs[target]
		if ok {
			delete(m.activeRecordIDs, target)
		}
		m.mu.Unlock()
		if ok {
			if reg := m.getRegistry(); reg != nil {
				_ = reg.MarkReverted(id)
			}
		}
	})

	m.mu.Lock()
	m.timers[target] = timer
	m.mu.Unlock()
}

// ClearAll stops all pending restore timers and reverts any remaining
// registry records. Registry writes for different targets happen
// concurrently so a slow revert for one target doesn't delay the others.
func (m *ResourceChaosManager) ClearAll() {
	m.mu.Lock()
	for k, t := range m.timers {
		t.Stop()
		delete(m.timers, k)
	}
	reg := m.registry
	pending := make(map[string]string, len(m.activeRecordIDs))
	for target, id := range m.activeRecordIDs {
		pending[target] = id
	}
	m.activeRecordIDs = make(map[string]string)
	m.mu.Unlock()

	if reg == nil || len(pending) == 0 {
		return
	}

	var wg sync.WaitGroup
	for _, id := range pending {
		wg.Add(1)
		go func(recordID string) {
			defer wg.Done()
			_ = reg.MarkReverted(recordID)
		}(id)
	}
	wg.Wait()
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
