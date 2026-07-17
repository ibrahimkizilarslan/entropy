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
	"syscall"
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

// alwaysBlockedIPs are specific IPs that are blocked unconditionally,
// regardless of the private-network policy below. These are cloud instance
// metadata services that expose credentials/secrets to anything that can
// reach them over HTTP — there is no legitimate chaos-probe use case for
// targeting them.
var alwaysBlockedIPs = []string{
	"169.254.169.254", // AWS / GCP / Azure / DigitalOcean IMDS
	"100.100.100.200", // Alibaba Cloud metadata
	"fd00:ec2::254",   // AWS IMDSv2 (IPv6)
}

// alwaysBlockedHostnames are hostnames that must always be blocked. This is a
// best-effort, defense-in-depth check performed before DNS resolution; the
// authoritative check is isBlockedIP, applied at dial time via dialerControl.
var alwaysBlockedHostnames = []string{
	"metadata.google.internal",
}

// privateNetworksAllowed controls whether probes can reach private/loopback/
// link-local network ranges, via the ENTROPY_ALLOW_PRIVATE_NETWORKS env var.
// Defaults to true: in chaos engineering, probes legitimately target local
// containers and services. Cloud metadata endpoints are blocked regardless
// of this setting.
func privateNetworksAllowed() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("ENTROPY_ALLOW_PRIVATE_NETWORKS")))
	switch v {
	case "false", "0", "no":
		return false
	default:
		return true
	}
}

// isBlockedIP reports whether ip must be blocked for SSRF protection, and why.
func isBlockedIP(ip net.IP) (blocked bool, reason string) {
	for _, blockedStr := range alwaysBlockedIPs {
		if b := net.ParseIP(blockedStr); b != nil && b.Equal(ip) {
			return true, fmt.Sprintf("cloud metadata IP %s", ip)
		}
	}

	if !privateNetworksAllowed() {
		switch {
		case ip.IsLoopback():
			return true, fmt.Sprintf("loopback IP %s (set ENTROPY_ALLOW_PRIVATE_NETWORKS=true to allow)", ip)
		case ip.IsPrivate():
			return true, fmt.Sprintf("private IP %s (set ENTROPY_ALLOW_PRIVATE_NETWORKS=true to allow)", ip)
		case ip.IsLinkLocalUnicast():
			return true, fmt.Sprintf("link-local IP %s (set ENTROPY_ALLOW_PRIVATE_NETWORKS=true to allow)", ip)
		}
	}

	return false, ""
}

// dialerControl is installed as net.Dialer.Control on every probe dialer
// (HTTP and TCP). Unlike a pre-check against the request hostname, Control
// runs after DNS resolution, against the exact IP address the connection is
// about to be made to. This closes the DNS-rebinding / TOCTOU gap: a hostname
// that resolves to a safe IP when first validated but a blocked IP at actual
// connection time (e.g. a second DNS lookup returning a different address)
// is still caught here, because this is the address the OS is about to
// connect to, not a separately-resolved one.
func dialerControl(network, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid dial address %q: %w", address, err)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("could not parse dial target %q as an IP", host)
	}
	if blocked, reason := isBlockedIP(ip); blocked {
		return fmt.Errorf("SSRF protection: connection to %s blocked (%s)", ip, reason)
	}
	return nil
}

// newSafeDialer returns a net.Dialer with dialerControl installed, for use by
// both HTTP and TCP probes.
func newSafeDialer(timeout time.Duration) *net.Dialer {
	return &net.Dialer{
		Timeout: timeout,
		Control: dialerControl,
	}
}

// validateProbeHost performs a best-effort, early SSRF check against a
// hostname or IP literal, before any connection is attempted. This exists to
// produce a clear, fast error message; it is NOT the authoritative check —
// dialerControl (run at actual dial time, against the resolved IP) is — so a
// DNS failure or race here is not a security hole, only a missed early exit.
func validateProbeHost(hostname string) error {
	for _, blocked := range alwaysBlockedHostnames {
		if strings.EqualFold(hostname, blocked) {
			return fmt.Errorf("probe target is a cloud metadata endpoint (%s), which is blocked for security", hostname)
		}
	}

	if ip := net.ParseIP(hostname); ip != nil {
		if blocked, reason := isBlockedIP(ip); blocked {
			return fmt.Errorf("probe target IP is blocked (%s)", reason)
		}
		return nil
	}

	ips, err := net.LookupHost(hostname)
	if err != nil {
		// If DNS fails here, let the probe attempt (and fail naturally) rather
		// than blocking on a lookup error unrelated to SSRF.
		return nil
	}
	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		if blocked, reason := isBlockedIP(ip); blocked {
			return fmt.Errorf("probe target resolves to a blocked IP (%s)", reason)
		}
	}
	return nil
}

// validateProbeURL checks the target URL's hostname against SSRF protections.
func validateProbeURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid probe URL: %w", err)
	}

	hostname := parsed.Hostname()
	if hostname == "" {
		return fmt.Errorf("probe URL has no hostname: %s", rawURL)
	}

	return validateProbeHost(hostname)
}

// validateProbeHostPort checks the target host:port against SSRF protections.
func validateProbeHostPort(hostPort string) error {
	host, _, err := net.SplitHostPort(hostPort)
	if err != nil {
		return fmt.Errorf("invalid host:port format: %s", hostPort)
	}

	return validateProbeHost(host)
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
	// SSRF protection, layer 1: fast pre-check against the request hostname.
	if err := validateProbeURL(spec.URL); err != nil {
		return ProbeResult{Success: false, Message: fmt.Sprintf("SSRF protection: %v", err)}
	}

	timeout := time.Duration(spec.Timeout) * time.Second

	// Clone the default transport (preserving proxy support, HTTP/2, and
	// connection-pooling defaults) and only override DialContext.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// SSRF protection, layer 2 (authoritative): dialerControl runs at dial
	// time against the resolved IP, closing the DNS-rebinding gap that a
	// hostname-only pre-check cannot.
	transport.DialContext = newSafeDialer(timeout).DialContext

	client := &http.Client{
		Timeout:   timeout,
		Transport: transport,
		// Prevent open redirect-based SSRF by limiting redirects
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("stopped after 3 redirects")
			}
			// Validate each redirect target's hostname; the dialer's Control
			// hook enforces the authoritative IP-level check regardless.
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
	// SSRF protection, layer 1: fast pre-check against the request host.
	if err := validateProbeHostPort(spec.HostPort); err != nil {
		return ProbeResult{Success: false, Message: fmt.Sprintf("SSRF protection: %v", err)}
	}

	timeout := time.Duration(spec.Timeout) * time.Second

	// SSRF protection, layer 2 (authoritative): dialerControl runs at dial
	// time against the resolved IP, closing the DNS-rebinding gap.
	d := newSafeDialer(timeout)

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
