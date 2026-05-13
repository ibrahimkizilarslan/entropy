package engine

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
type ContainerRuntime interface {
	StopContainer(name string, timeout int) (*ContainerInfo, error)
	RestartContainer(name string, timeout int) (*ContainerInfo, error)
	PauseContainer(name string) (*ContainerInfo, error)
	UnpauseContainer(name string) (*ContainerInfo, error)
	GetContainerPID(name string) (int, error)
	UpdateContainerResources(name string, cpuQuota int64, cpuPeriod int64, memLimit int64) (*ContainerInfo, error)
	InjectNetworkDelay(target string, latencyMs int, jitterMs int, duration *int) error
	InjectNetworkLoss(target string, lossPercent int, duration *int) error
	ExecCommand(name string, cmd []string) (int, error)
	ListContainers(all bool) ([]ContainerInfo, error)
	Close()
}
