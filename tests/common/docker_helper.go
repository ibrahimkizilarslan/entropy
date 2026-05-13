package common

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

// DockerHelper provides utility methods for interacting with Docker in tests
type DockerHelper struct {
	cli *client.Client
}

// NewDockerHelper creates a new DockerHelper
func NewDockerHelper() (*DockerHelper, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &DockerHelper{cli: cli}, nil
}

// IsContainerRunning checks if a container with the given name is running
func (h *DockerHelper) IsContainerRunning(name string) (bool, error) {
	containers, err := h.cli.ContainerList(context.Background(), types.ContainerListOptions{All: true})
	if err != nil {
		return false, err
	}

	for _, c := range containers {
		for _, n := range c.Names {
			if strings.TrimPrefix(n, "/") == name {
				return c.State == "running", nil
			}
		}
	}
	return false, fmt.Errorf("container %s not found", name)
}

// GetContainerStatus returns the status of a container
func (h *DockerHelper) GetContainerStatus(name string) (string, error) {
	containers, err := h.cli.ContainerList(context.Background(), types.ContainerListOptions{All: true})
	if err != nil {
		return "", err
	}

	for _, c := range containers {
		for _, n := range c.Names {
			if strings.TrimPrefix(n, "/") == name {
				return c.State, nil
			}
		}
	}
	return "", fmt.Errorf("container %s not found", name)
}

// Close closes the docker client
func (h *DockerHelper) Close() {
	if h.cli != nil {
		h.cli.Close()
	}
}
