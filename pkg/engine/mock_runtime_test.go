package engine

import (
	"fmt"
	"sync"
)

// MockRuntime is a test double implementing ContainerRuntime.
// It tracks all calls made to it, allowing assertions on behavior.
type MockRuntime struct {
	mu sync.Mutex

	// Containers is the initial set of containers the mock knows about.
	Containers map[string]*ContainerInfo

	// Call tracking
	Calls []MockCall

	// Configurable behavior
	StopErr     error
	RestartErr  error
	PauseErr    error
	UnpauseErr  error
	ExecErr     error
	ExecExit    int
	DelayErr    error
	LossErr     error
	UpdateErr   error
	ListErr     error
}

type MockCall struct {
	Method string
	Args   []interface{}
}

func NewMockRuntime() *MockRuntime {
	return &MockRuntime{
		Containers: map[string]*ContainerInfo{
			"service-a": {ID: "abc123", Name: "service-a", Image: "nginx:latest", Status: "running"},
			"service-b": {ID: "def456", Name: "service-b", Image: "redis:alpine", Status: "running"},
			"service-c": {ID: "ghi789", Name: "service-c", Image: "postgres:15", Status: "running"},
		},
		Calls: []MockCall{},
	}
}

func (m *MockRuntime) record(method string, args ...interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Calls = append(m.Calls, MockCall{Method: method, Args: args})
}

func (m *MockRuntime) CallCount(method string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, c := range m.Calls {
		if c.Method == method {
			count++
		}
	}
	return count
}

func (m *MockRuntime) getContainer(name string) (*ContainerInfo, error) {
	info, ok := m.Containers[name]
	if !ok {
		return nil, fmt.Errorf("container '%s' not found", name)
	}
	return info, nil
}

func (m *MockRuntime) StopContainer(name string, timeout int) (*ContainerInfo, error) {
	m.record("StopContainer", name, timeout)
	if m.StopErr != nil {
		return nil, m.StopErr
	}
	info, err := m.getContainer(name)
	if err != nil {
		return nil, err
	}
	info.Status = "exited"
	return info, nil
}

func (m *MockRuntime) RestartContainer(name string, timeout int) (*ContainerInfo, error) {
	m.record("RestartContainer", name, timeout)
	if m.RestartErr != nil {
		return nil, m.RestartErr
	}
	info, err := m.getContainer(name)
	if err != nil {
		return nil, err
	}
	info.Status = "running"
	return info, nil
}

func (m *MockRuntime) PauseContainer(name string) (*ContainerInfo, error) {
	m.record("PauseContainer", name)
	if m.PauseErr != nil {
		return nil, m.PauseErr
	}
	info, err := m.getContainer(name)
	if err != nil {
		return nil, err
	}
	info.Status = "paused"
	return info, nil
}

func (m *MockRuntime) UnpauseContainer(name string) (*ContainerInfo, error) {
	m.record("UnpauseContainer", name)
	if m.UnpauseErr != nil {
		return nil, m.UnpauseErr
	}
	info, err := m.getContainer(name)
	if err != nil {
		return nil, err
	}
	info.Status = "running"
	return info, nil
}

func (m *MockRuntime) GetContainerPID(name string) (int, error) {
	m.record("GetContainerPID", name)
	return 12345, nil
}

func (m *MockRuntime) UpdateContainerResources(name string, cpuQuota int64, cpuPeriod int64, memLimit int64) (*ContainerInfo, error) {
	m.record("UpdateContainerResources", name, cpuQuota, cpuPeriod, memLimit)
	if m.UpdateErr != nil {
		return nil, m.UpdateErr
	}
	info, err := m.getContainer(name)
	if err != nil {
		return nil, err
	}
	return info, nil
}

func (m *MockRuntime) InjectNetworkDelay(target string, latencyMs int, jitterMs int, duration *int) error {
	m.record("InjectNetworkDelay", target, latencyMs, jitterMs, duration)
	return m.DelayErr
}

func (m *MockRuntime) InjectNetworkLoss(target string, lossPercent int, duration *int) error {
	m.record("InjectNetworkLoss", target, lossPercent, duration)
	return m.LossErr
}

func (m *MockRuntime) ExecCommand(name string, cmd []string) (int, error) {
	m.record("ExecCommand", name, cmd)
	if m.ExecErr != nil {
		return -1, m.ExecErr
	}
	return m.ExecExit, nil
}

func (m *MockRuntime) ListContainers(all bool) ([]ContainerInfo, error) {
	m.record("ListContainers", all)
	if m.ListErr != nil {
		return nil, m.ListErr
	}
	var result []ContainerInfo
	for _, c := range m.Containers {
		result = append(result, *c)
	}
	return result, nil
}

func (m *MockRuntime) Close() {
	m.record("Close")
}
