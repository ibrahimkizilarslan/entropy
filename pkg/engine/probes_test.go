package engine

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ibrahimkizilarslan/entropy-cli/pkg/config"
)

func TestRunProbe_HTTP(t *testing.T) {
	// Start a test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	probeHTTP := &config.ProbeSpec{
		Type:    "http",
		URL:     server.URL + "/health",
		Timeout: 2,
	}

	t.Run("success", func(t *testing.T) {
		result := RunProbe(probeHTTP, nil)
		if !result.Success {
			t.Errorf("Expected success for successful probe, got %v", result.Message)
		}
	})

	t.Run("expect_status success", func(t *testing.T) {
		status := 200
		probe := *probeHTTP
		probe.ExpectStatus = &status
		result := RunProbe(&probe, nil)
		if !result.Success {
			t.Errorf("Expected success for expected status 200, got %v", result.Message)
		}
	})

	t.Run("expect_status failure", func(t *testing.T) {
		status := 201
		probe := *probeHTTP
		probe.ExpectStatus = &status
		result := RunProbe(&probe, nil)
		if result.Success {
			t.Error("Expected failure for mismatched status, got success")
		}
	})

	t.Run("404 fallback to success if not specified", func(t *testing.T) {
		// In the current implementation, any response without an error means success if no expectations
		probe := *probeHTTP
		probe.URL = server.URL + "/notfound"
		result := RunProbe(&probe, nil)
		if !result.Success {
			t.Errorf("Expected success for 404 status without expect_status, got %v", result.Message)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		timeoutServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(2 * time.Second)
			w.WriteHeader(http.StatusOK)
		}))
		defer timeoutServer.Close()

		probe := *probeHTTP
		probe.URL = timeoutServer.URL
		probe.Timeout = 1

		result := RunProbe(&probe, nil)
		if result.Success {
			t.Error("Expected failure for timeout, got success")
		}
	})
}

func TestRunProbe_UnknownType(t *testing.T) {
	probe := &config.ProbeSpec{
		Type: "unknown",
	}
	result := RunProbe(probe, nil)
	if result.Success {
		t.Error("Expected failure for unknown probe type, got success")
	}
}
