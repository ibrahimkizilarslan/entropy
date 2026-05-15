package engine

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ibrahimkizilarslan/entropy/pkg/config"
)

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
var allowPrivateNetworks = true

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
	// SSRF protection: validate host:port before connecting
	if err := validateProbeHostPort(spec.HostPort); err != nil {
		return ProbeResult{Success: false, Message: fmt.Sprintf("SSRF protection: %v", err)}
	}

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
