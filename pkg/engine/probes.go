package engine

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ibrahimkizilarslan/entropy/pkg/config"
)

// defaultAllowedExecCommands are the only executables permitted in exec probes
// by default. This is a deliberately narrow allowlist of read-only diagnostic
// commands: nothing here can write files, spawn a shell, or reach the network.
//
// A blocklist was used previously, but blocklists are inherently incomplete:
// commands like `env sh -c '...'`, `busybox sh`, `awk 'BEGIN{system("...")}'`,
// or `find . -exec ...` are not shells or interpreters themselves, yet can be
// used to obtain arbitrary code execution. An allowlist closes that class of
// bypass by construction — anything not explicitly trusted is rejected.
var defaultAllowedExecCommands = []string{
	"cat", "ls", "stat", "test", "true", "false",
	"echo", "pgrep", "ps", "head", "tail", "wc", "grep",
}

// execShellMetacharacters are characters that, if present in any argument,
// could allow a nominally-safe command to be abused (e.g. as a wrapper that
// still reaches a shell or writes output to an unexpected place). This is a
// defense-in-depth check layered on top of the allowlist, not a replacement
// for it.
const execShellMetacharacters = ";|&`$()<>"

// allowedExecCommands returns the effective allowlist: the built-in defaults
// plus any executables added via ENTROPY_EXEC_ALLOWLIST (comma-separated).
// This lets operators extend the allowlist for their own scenarios without
// forking Entropy, while keeping the out-of-the-box default restrictive.
func allowedExecCommands() map[string]bool {
	allowed := make(map[string]bool, len(defaultAllowedExecCommands))
	for _, c := range defaultAllowedExecCommands {
		allowed[strings.ToLower(c)] = true
	}
	if extra := os.Getenv("ENTROPY_EXEC_ALLOWLIST"); extra != "" {
		for _, c := range strings.Split(extra, ",") {
			c = strings.ToLower(strings.TrimSpace(c))
			if c != "" {
				allowed[c] = true
			}
		}
	}
	return allowed
}

// validateExecCommand checks that the command's base executable is in the
// allowlist and that no argument contains shell metacharacters. This prevents
// using exec probes as an RCE vector through scenario YAML files.
func validateExecCommand(cmdParts []string) error {
	if len(cmdParts) == 0 {
		return fmt.Errorf("empty exec command")
	}

	// Extract the base name of the executable (handles absolute paths like /bin/cat)
	executable := strings.ToLower(filepath.Base(cmdParts[0]))

	if !allowedExecCommands()[executable] {
		return fmt.Errorf("exec probe command '%s' is not in the allowlist. "+
			"Allowed executables: %s. "+
			"Add more via the ENTROPY_EXEC_ALLOWLIST environment variable (comma-separated)",
			executable, strings.Join(defaultAllowedExecCommands, ", "))
	}

	for _, arg := range cmdParts[1:] {
		if strings.ContainsAny(arg, execShellMetacharacters) {
			return fmt.Errorf("exec probe argument '%s' contains a shell metacharacter and is blocked for security", arg)
		}
	}

	return nil
}

type ProbeResult struct {
	Success bool
	Message string
}

// denyListCIDRs contains CIDR ranges that probes should not be allowed to reach
// unless explicitly opted-in. This prevents SSRF attacks via malicious scenario files.
var denyListCIDRs = []string{
	"169.254.169.254/32", // AWS/GCP/Azure metadata service
	"169.254.0.0/16",     // Link-local addresses
	"127.0.0.0/8",        // Loopback (allowed by default, but metadata is blocked)
	"10.0.0.0/8",         // RFC1918 private
	"172.16.0.0/12",      // RFC1918 private
	"192.168.0.0/16",     // RFC1918 private
}

// metadataDenyList contains specific IP addresses that must always be blocked (cloud metadata).
var metadataDenyList = []string{
	"169.254.169.254",
	"metadata.google.internal",
}

// allowPrivateNetworks controls whether probes can reach private network ranges.
// In chaos engineering, probes legitimately target local containers, so private
// networks are allowed by default. Only cloud metadata endpoints are always blocked.
const allowPrivateNetworks = true

// validateProbeURL checks the target URL against the deny-list to prevent SSRF attacks.
// Cloud metadata endpoints (169.254.169.254) are always blocked.
func validateProbeURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid probe URL: %w", err)
	}

	hostname := parsed.Hostname()
	if hostname == "" {
		return fmt.Errorf("probe URL has no hostname: %s", rawURL)
	}

	// Always block known cloud metadata endpoints
	for _, blocked := range metadataDenyList {
		if strings.EqualFold(hostname, blocked) {
			return fmt.Errorf("probe URL targets a cloud metadata endpoint (%s), which is blocked for security", hostname)
		}
	}

	// Resolve hostname to IP for CIDR checks
	ips, err := net.LookupHost(hostname)
	if err != nil {
		// If DNS fails, allow the probe to proceed (it will fail naturally)
		return nil
	}

	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}

		// Always block link-local metadata range
		_, metadataCIDR, _ := net.ParseCIDR("169.254.169.254/32")
		if metadataCIDR.Contains(ip) {
			return fmt.Errorf("probe URL resolves to cloud metadata IP (%s), which is blocked for security", ipStr)
		}

		// If private networks are not allowed, check against all deny-listed CIDRs
		if !allowPrivateNetworks {
			for _, cidrStr := range denyListCIDRs {
				_, cidr, _ := net.ParseCIDR(cidrStr)
				if cidr != nil && cidr.Contains(ip) {
					return fmt.Errorf("probe URL resolves to a private/restricted IP (%s in %s), which is blocked", ipStr, cidrStr)
				}
			}
		}
	}

	return nil
}

// validateProbeHostPort checks the target host:port against the deny-list.
func validateProbeHostPort(hostPort string) error {
	host, _, err := net.SplitHostPort(hostPort)
	if err != nil {
		return fmt.Errorf("invalid host:port format: %s", hostPort)
	}

	// Check metadata endpoints
	for _, blocked := range metadataDenyList {
		if strings.EqualFold(host, blocked) {
			return fmt.Errorf("probe target is a cloud metadata endpoint (%s), which is blocked for security", host)
		}
	}

	return nil
}

func RunProbe(spec *config.ProbeSpec, runtime ContainerRuntime) ProbeResult {
	return RunProbeWithContext(context.Background(), spec, runtime)
}

func RunProbeWithContext(ctx context.Context, spec *config.ProbeSpec, runtime ContainerRuntime) ProbeResult {
	switch spec.Type {
	case "http":
		return runHTTPProbe(ctx, spec)
	case "tcp":
		return runTCPProbe(ctx, spec)
	case "exec":
		return runExecProbe(ctx, spec, runtime)
	default:
		return ProbeResult{Success: false, Message: fmt.Sprintf("unsupported probe type: %s", spec.Type)}
	}
}

func runHTTPProbe(ctx context.Context, spec *config.ProbeSpec) ProbeResult {
	// SSRF protection: validate URL before making the request
	if err := validateProbeURL(spec.URL); err != nil {
		return ProbeResult{Success: false, Message: fmt.Sprintf("SSRF protection: %v", err)}
	}

	client := &http.Client{
		Timeout: time.Duration(spec.Timeout) * time.Second,
		// Prevent open redirect-based SSRF by limiting redirects
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("stopped after 3 redirects")
			}
			// Validate each redirect target
			if err := validateProbeURL(req.URL.String()); err != nil {
				return fmt.Errorf("redirect blocked by SSRF protection: %w", err)
			}
			return nil
		},
	}

	req, err := http.NewRequestWithContext(ctx, "GET", spec.URL, nil)
	if err != nil {
		return ProbeResult{Success: false, Message: fmt.Sprintf("Failed to create request: %v", err)}
	}

	resp, err := client.Do(req)
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

func runTCPProbe(ctx context.Context, spec *config.ProbeSpec) ProbeResult {
	// SSRF protection: validate host:port before connecting
	if err := validateProbeHostPort(spec.HostPort); err != nil {
		return ProbeResult{Success: false, Message: fmt.Sprintf("SSRF protection: %v", err)}
	}

	timeout := time.Duration(spec.Timeout) * time.Second

	var d net.Dialer
	d.Timeout = timeout

	conn, err := d.DialContext(ctx, "tcp", spec.HostPort)
	if err != nil {
		return ProbeResult{Success: false, Message: fmt.Sprintf("TCP connect failed to %s: %v", spec.HostPort, err)}
	}
	conn.Close()
	return ProbeResult{Success: true, Message: fmt.Sprintf("TCP connected successfully to %s", spec.HostPort)}
}

func runExecProbe(ctx context.Context, spec *config.ProbeSpec, runtime ContainerRuntime) ProbeResult {
	if runtime == nil {
		return ProbeResult{Success: false, Message: "Container runtime not initialized"}
	}

	cmdParts := strings.Fields(spec.Command)
	if len(cmdParts) == 0 {
		return ProbeResult{Success: false, Message: "empty exec command"}
	}

	// Security: block dangerous executables to prevent RCE via scenario YAML
	if err := validateExecCommand(cmdParts); err != nil {
		return ProbeResult{Success: false, Message: fmt.Sprintf("security: %v", err)}
	}

	// Audit logging: log every exec probe invocation for security monitoring
	slog.Warn("EXEC_PROBE",
		slog.String("target", spec.Target),
		slog.String("command", spec.Command),
		slog.String("executable", filepath.Base(cmdParts[0])),
	)

	exitCode, err := runtime.ExecCommand(ctx, spec.Target, cmdParts)
	if err != nil {
		return ProbeResult{Success: false, Message: fmt.Sprintf("Exec failed: %v", err)}
	}

	if exitCode == 0 {
		return ProbeResult{Success: true, Message: fmt.Sprintf("Exec command '%s' succeeded", spec.Command)}
	}
	return ProbeResult{Success: false, Message: fmt.Sprintf("Exec command '%s' failed with exit code %d", spec.Command, exitCode)}
}
