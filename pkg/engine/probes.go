package engine

import (
	"fmt"
	"net/http"
	"time"

	"github.com/ibrahimkizilarslan/entropy-cli/pkg/config"
)

type ProbeResult struct {
	Success bool
	Message string
}

func RunProbe(spec *config.ProbeSpec) ProbeResult {
	if spec.Type != "http" {
		return ProbeResult{Success: false, Message: fmt.Sprintf("unsupported probe type: %s", spec.Type)}
	}

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
