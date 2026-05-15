package engine

import "context"

// ContainerInfo represents the standardized output of a container's state,
// regardless of the underlying runtime (Docker, Kubernetes, etc.)
type ContainerInfo struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Image  string            `json:"image"`
	Status string            `json:"status"`
	Ports  map[string]string `json:"ports"`
}

// ContainerRuntime defines the interface for interacting with container orchestration platforms.
// By abstracting the runtime, Entropy can support Docker, Kubernetes, and other platforms seamlessly.
//
// All methods accept a context.Context as the first argument. Callers should pass a context
// with an appropriate deadline or cancellation signal to ensure operations don't block indefinitely.
// Use context.Background() only at the top-level entry points (CLI commands, daemon loop).
type ContainerRuntime interface {
	StopContainer(ctx context.Context, name string, timeout int) (*ContainerInfo, error)
	RestartContainer(ctx context.Context, name string, timeout int) (*ContainerInfo, error)
	PauseContainer(ctx context.Context, name string) (*ContainerInfo, error)
	UnpauseContainer(ctx context.Context, name string) (*ContainerInfo, error)
	GetContainerPID(ctx context.Context, name string) (int, error)
	UpdateContainerResources(ctx context.Context, name string, cpuQuota int64, cpuPeriod int64, memLimit int64) (*ContainerInfo, error)
	InjectNetworkDelay(ctx context.Context, target string, latencyMs int, jitterMs int, duration *int) error
	InjectNetworkLoss(ctx context.Context, target string, lossPercent int, duration *int) error
	ExecCommand(ctx context.Context, name string, cmd []string) (int, error)
	ListContainers(ctx context.Context, all bool) ([]ContainerInfo, error)
	Close()
}
