package engine

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ibrahimkizilarslan/entropy/pkg/config"
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

func TestValidateProbeURL_BlocksMetadata(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"AWS metadata", "http://169.254.169.254/latest/meta-data/", true},
		{"GCP metadata", "http://metadata.google.internal/computeMetadata/v1/", true},
		{"normal URL", "http://example.com/health", false},
		{"localhost allowed", "http://localhost:8080/health", false},
		{"empty hostname", "http:///path", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProbeURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateProbeURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestValidateProbeHostPort_BlocksMetadata(t *testing.T) {
	tests := []struct {
		name     string
		hostPort string
		wantErr  bool
	}{
		{"metadata IP", "169.254.169.254:80", true},
		{"normal host", "localhost:6379", false},
		{"private IP allowed", "192.168.1.1:8080", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProbeHostPort(tt.hostPort)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateProbeHostPort(%q) error = %v, wantErr %v", tt.hostPort, err, tt.wantErr)
			}
		})
	}
}

func TestHTTPProbe_SSRFBlocked(t *testing.T) {
	probe := &config.ProbeSpec{
		Type:    "http",
		URL:     "http://169.254.169.254/latest/meta-data/",
		Timeout: 2,
	}
	result := RunProbe(probe, nil)
	if result.Success {
		t.Error("Expected SSRF protection to block metadata URL")
	}
	if result.Message == "" {
		t.Error("Expected error message for blocked URL")
	}
}

func TestTCPProbe_SSRFBlocked(t *testing.T) {
	probe := &config.ProbeSpec{
		Type:     "tcp",
		HostPort: "169.254.169.254:80",
		Timeout:  2,
	}
	result := RunProbe(probe, nil)
	if result.Success {
		t.Error("Expected SSRF protection to block metadata host")
	}
}
