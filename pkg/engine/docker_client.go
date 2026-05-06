package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

type ContainerInfo struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Image  string            `json:"image"`
	Status string            `json:"status"`
	Ports  map[string]string `json:"ports"`
}

type DockerClient struct {
	cli            *client.Client
	allowedTargets map[string]bool
}

func tryConnectWithOpts(allowedTargets []string, opt client.Opt) (*DockerClient, error) {
	cli, err := client.NewClientWithOpts(opt, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	_, err = cli.Ping(context.Background())
	if err != nil {
		return nil, err
	}

	var allowed map[string]bool
	if allowedTargets != nil {
		allowed = make(map[string]bool)
		for _, t := range allowedTargets {
			allowed[t] = true
		}
	}

	return &DockerClient{
		cli:            cli,
		allowedTargets: allowed,
	}, nil
}

func NewDockerClient(allowedTargets []string) (*DockerClient, error) {
	// 1. Explicit DOCKER_HOST bypasses everything
	if os.Getenv("DOCKER_HOST") != "" {
		return tryConnectWithOpts(allowedTargets, client.FromEnv)
	}

	homeDir, _ := os.UserHomeDir()
	currentContext := ""

	// 2. Try to read active context from ~/.docker/config.json
	if homeDir != "" {
		data, err := os.ReadFile(filepath.Join(homeDir, ".docker", "config.json"))
		if err == nil {
			var cfg struct {
				CurrentContext string `json:"currentContext"`
			}
			if json.Unmarshal(data, &cfg) == nil {
				currentContext = cfg.CurrentContext
			}
		}
	}

	var endpoints []string

	// 3. Build prioritized endpoint list
	if currentContext == "desktop-linux" && runtime.GOOS == "linux" && homeDir != "" {
		endpoints = append(endpoints, "unix://"+filepath.Join(homeDir, ".docker", "desktop", "docker.sock"))
	}

	if runtime.GOOS == "linux" {
		endpoints = append(endpoints, "unix:///var/run/docker.sock")
		if homeDir != "" {
			endpoints = append(endpoints, "unix://"+filepath.Join(homeDir, ".docker", "desktop", "docker.sock"))
		}
	} else if runtime.GOOS == "darwin" {
		if homeDir != "" {
			endpoints = append(endpoints, "unix://"+filepath.Join(homeDir, ".docker", "run", "docker.sock"))
		}
		endpoints = append(endpoints, "unix:///var/run/docker.sock")
	} else if runtime.GOOS == "windows" {
		endpoints = append(endpoints, "npipe:////./pipe/docker_engine")
	}

	// 4. Try endpoints sequentially
	var lastErr error
	for _, ep := range endpoints {
		dc, err := tryConnectWithOpts(allowedTargets, client.WithHost(ep))
		if err == nil {
			return dc, nil
		}
		lastErr = err
	}

	// 5. Final fallback to FromEnv (default docker behavior)
	dc, err := tryConnectWithOpts(allowedTargets, client.FromEnv)
	if err == nil {
		return dc, nil
	}

	return nil, fmt.Errorf("cannot connect to Docker daemon. Checked multiple endpoints. Last error: %w", lastErr)
}

func (d *DockerClient) assertAllowed(name string) error {
	if d.allowedTargets != nil && !d.allowedTargets[name] {
		return fmt.Errorf("'%s' is not in the configured chaos targets", name)
	}
	return nil
}

func (d *DockerClient) getContainerID(name string) (string, error) {
	containers, err := d.cli.ContainerList(context.Background(), types.ContainerListOptions{
		All: true,
	})
	if err != nil {
		return "", err
	}
	for _, c := range containers {
		for _, n := range c.Names {
			if strings.TrimPrefix(n, "/") == name {
				return c.ID, nil
			}
		}
		if c.Labels["com.docker.compose.service"] == name {
			return c.ID, nil
		}
	}
	return "", fmt.Errorf("container not found: %s", name)
}

func (d *DockerClient) getContainerInfo(id string) (*ContainerInfo, error) {
	c, err := d.cli.ContainerInspect(context.Background(), id)
	if err != nil {
		return nil, err
	}
	
	ports := make(map[string]string)
	for k, v := range c.NetworkSettings.Ports {
		if len(v) > 0 {
			ports[string(k)] = v[0].HostPort
		}
	}
	
	imageName := c.Config.Image
	
	return &ContainerInfo{
		ID:     c.ID[:12],
		Name:   strings.TrimPrefix(c.Name, "/"),
		Image:  imageName,
		Status: c.State.Status,
		Ports:  ports,
	}, nil
}

func (d *DockerClient) ListContainers(all bool) ([]ContainerInfo, error) {
	containers, err := d.cli.ContainerList(context.Background(), types.ContainerListOptions{All: all})
	if err != nil {
		return nil, err
	}
	
	var res []ContainerInfo
	for _, c := range containers {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		
		ports := make(map[string]string)
		for _, p := range c.Ports {
			if p.PublicPort != 0 {
				ports[fmt.Sprintf("%d/%s", p.PrivatePort, p.Type)] = fmt.Sprintf("%d", p.PublicPort)
			}
		}
		
		res = append(res, ContainerInfo{
			ID:     c.ID[:12],
			Name:   name,
			Image:  c.Image,
			Status: c.State,
			Ports:  ports,
		})
	}
	return res, nil
}

func (d *DockerClient) StopContainer(name string, timeout int) (*ContainerInfo, error) {
	if err := d.assertAllowed(name); err != nil {
		return nil, err
	}
	id, err := d.getContainerID(name)
	if err != nil {
		return nil, err
	}
	
	t := timeout
	err = d.cli.ContainerStop(context.Background(), id, container.StopOptions{Timeout: &t})
	if err != nil {
		return nil, fmt.Errorf("failed to stop container %s: %w", name, err)
	}
	
	return d.getContainerInfo(id)
}

func (d *DockerClient) RestartContainer(name string, timeout int) (*ContainerInfo, error) {
	if err := d.assertAllowed(name); err != nil {
		return nil, err
	}
	id, err := d.getContainerID(name)
	if err != nil {
		return nil, err
	}
	
	t := timeout
	err = d.cli.ContainerRestart(context.Background(), id, container.StopOptions{Timeout: &t})
	if err != nil {
		return nil, fmt.Errorf("failed to restart container %s: %w", name, err)
	}
	
	return d.getContainerInfo(id)
}

func (d *DockerClient) PauseContainer(name string) (*ContainerInfo, error) {
	if err := d.assertAllowed(name); err != nil {
		return nil, err
	}
	id, err := d.getContainerID(name)
	if err != nil {
		return nil, err
	}
	
	err = d.cli.ContainerPause(context.Background(), id)
	if err != nil {
		return nil, fmt.Errorf("failed to pause container %s: %w", name, err)
	}
	
	return d.getContainerInfo(id)
}

func (d *DockerClient) UnpauseContainer(name string) (*ContainerInfo, error) {
	if err := d.assertAllowed(name); err != nil {
		return nil, err
	}
	id, err := d.getContainerID(name)
	if err != nil {
		return nil, err
	}
	
	err = d.cli.ContainerUnpause(context.Background(), id)
	if err != nil {
		return nil, fmt.Errorf("failed to unpause container %s: %w", name, err)
	}
	
	return d.getContainerInfo(id)
}

func (d *DockerClient) GetContainerPID(name string) (int, error) {
	if err := d.assertAllowed(name); err != nil {
		return 0, err
	}
	id, err := d.getContainerID(name)
	if err != nil {
		return 0, err
	}
	
	c, err := d.cli.ContainerInspect(context.Background(), id)
	if err != nil {
		return 0, err
	}
	
	if !c.State.Running {
		return 0, fmt.Errorf("container is %s, not running", c.State.Status)
	}
	
	if c.State.Pid == 0 {
		return 0, fmt.Errorf("PID is 0 or missing")
	}
	
	return c.State.Pid, nil
}

func (d *DockerClient) UpdateContainerResources(name string, cpuQuota int64, cpuPeriod int64, memLimit int64) (*ContainerInfo, error) {
	if err := d.assertAllowed(name); err != nil {
		return nil, err
	}
	id, err := d.getContainerID(name)
	if err != nil {
		return nil, err
	}
	
	res := container.Resources{
		CPUQuota:  cpuQuota,
		CPUPeriod: cpuPeriod,
		Memory:    memLimit,
	}
	
	_, err = d.cli.ContainerUpdate(context.Background(), id, container.UpdateConfig{Resources: res})
	if err != nil {
		return nil, fmt.Errorf("failed to update resources for container %s: %w", name, err)
	}
	
	return d.getContainerInfo(id)
}

func (d *DockerClient) Close() {
	d.cli.Close()
}
