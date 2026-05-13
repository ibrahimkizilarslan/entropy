package engine

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/ibrahimkizilarslan/entropy/pkg/config"
)

type ProbeResult struct {
	Success bool
	Message string
}

func RunProbe(spec *config.ProbeSpec, runtime ContainerRuntime) ProbeResult {
	switch spec.Type {
	case "http":
		return runHTTPProbe(spec)
	case "tcp":
		return runTCPProbe(spec)
	case "exec":
		return runExecProbe(spec, runtime)
	default:
		return ProbeResult{Success: false, Message: fmt.Sprintf("unsupported probe type: %s", spec.Type)}
	}
}

func runHTTPProbe(spec *config.ProbeSpec) ProbeResult {
	client := &http.Client{
		Timeout: time.Duration(spec.Timeout) * time.Second,
	}

	resp, err := client.Get(spec.URL)
	if err != nil {
		if spec.ExpectStatus != nil {
			return ProbeResult{Success: false, Message: fmt.Sprintf("HTTP GET failed: %v", err)}
		}
		if spec.ExpectNotStatus != nil {
			status := 0
			if status != *spec.ExpectNotStatus {
				return ProbeResult{Success: true, Message: fmt.Sprintf("HTTP GET failed (status 0), which is not %d", *spec.ExpectNotStatus)}
			}
		}
		return ProbeResult{Success: false, Message: fmt.Sprintf("HTTP GET failed: %v", err)}
	}
	defer resp.Body.Close()

	status := resp.StatusCode

	if spec.ExpectStatus != nil {
		if status == *spec.ExpectStatus {
			return ProbeResult{Success: true, Message: fmt.Sprintf("got expected status %d", status)}
		}
		return ProbeResult{Success: false, Message: fmt.Sprintf("expected status %d, got %d", *spec.ExpectStatus, status)}
	}

	if spec.ExpectNotStatus != nil {
		if status != *spec.ExpectNotStatus {
			return ProbeResult{Success: true, Message: fmt.Sprintf("got status %d, which is not %d", status, *spec.ExpectNotStatus)}
		}
		return ProbeResult{Success: false, Message: fmt.Sprintf("did not expect status %d, but got it", status)}
	}

	return ProbeResult{Success: true, Message: fmt.Sprintf("HTTP GET succeeded with status %d", status)}
}

func runTCPProbe(spec *config.ProbeSpec) ProbeResult {
	timeout := time.Duration(spec.Timeout) * time.Second
	conn, err := net.DialTimeout("tcp", spec.HostPort, timeout)
	if err != nil {
		return ProbeResult{Success: false, Message: fmt.Sprintf("TCP connect failed to %s: %v", spec.HostPort, err)}
	}
	conn.Close()
	return ProbeResult{Success: true, Message: fmt.Sprintf("TCP connected successfully to %s", spec.HostPort)}
}

func runExecProbe(spec *config.ProbeSpec, runtime ContainerRuntime) ProbeResult {
	if runtime == nil {
		return ProbeResult{Success: false, Message: "Container runtime not initialized"}
	}

	cmdParts := strings.Fields(spec.Command)
	if len(cmdParts) == 0 {
		return ProbeResult{Success: false, Message: "empty exec command"}
	}

	exitCode, err := runtime.ExecCommand(spec.Target, cmdParts)
	if err != nil {
		return ProbeResult{Success: false, Message: fmt.Sprintf("Exec failed: %v", err)}
	}

	if exitCode == 0 {
		return ProbeResult{Success: true, Message: fmt.Sprintf("Exec command '%s' succeeded", spec.Command)}
	}
	return ProbeResult{Success: false, Message: fmt.Sprintf("Exec command '%s' failed with exit code %d", spec.Command, exitCode)}
}
