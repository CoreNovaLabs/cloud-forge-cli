package cli

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// bootstrapPollInterval is the interval between service-ready probes after a
// stack reaches CREATE_COMPLETE. It is shared by AWS and Aliyun deploys.
// Tests may shorten it to speed up assertions.
var bootstrapPollInterval = 30 * time.Second

var serviceHTTPClient = &http.Client{
	Timeout: 15 * time.Second,
}

// insecureIPServiceHTTPClient probes IP-address HTTPS endpoints. Domain
// endpoints use normal certificate verification.
var insecureIPServiceHTTPClient = &http.Client{
	Timeout: 15 * time.Second,
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // IP certs may not match hostname
	},
}

type serviceProbe struct {
	display string
	url     string
	tcpAddr string
}

// probeTargets derives readiness probes from stack outputs. HTTP services are
// probed with /health and /, while non-HTTP ServiceURL schemes such as mysql
// or postgresql are treated as TCP endpoints.
func probeTargets(outputs map[string]string) []serviceProbe {
	base := strings.TrimSpace(outputs["ServiceURL"])
	if base == "" {
		if ip := strings.TrimSpace(outputs["PublicIP"]); ip != "" {
			base = "https://" + ip
		}
	}
	if base == "" {
		return nil
	}

	parsed, err := url.Parse(base)
	if err != nil {
		return nil
	}

	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "":
		base = strings.TrimRight(base, "/")
		return []serviceProbe{
			{display: base + "/health", url: base + "/health"},
			{display: base + "/", url: base + "/"},
		}
	default:
		host := strings.TrimSpace(parsed.Hostname())
		port := strings.TrimSpace(parsed.Port())
		if host == "" || port == "" {
			return nil
		}
		return []serviceProbe{
			{
				display: base,
				tcpAddr: net.JoinHostPort(host, port),
			},
		}
	}
}

func probeDisplayStrings(probes []serviceProbe) []string {
	out := make([]string, 0, len(probes))
	for _, probe := range probes {
		out = append(out, probe.display)
	}
	return out
}

// serviceReady reports whether the given URL responds with a 2xx status.
func serviceReady(ctx context.Context, rawURL string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return false
	}
	resp, err := serviceHTTPClientForURL(rawURL).Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func serviceHTTPClientForURL(rawURL string) *http.Client {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return serviceHTTPClient
	}
	if net.ParseIP(parsed.Hostname()) != nil {
		return insecureIPServiceHTTPClient
	}
	return serviceHTTPClient
}

// tcpServiceReady reports whether the given TCP address accepts a connection.
func tcpServiceReady(ctx context.Context, addr string) bool {
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// waitServiceReady polls the deployed endpoint until the HTTP target responds
// 2xx or the TCP target accepts a connection, or the deadline expires. It is
// cloud-agnostic; callers pass the stack outputs and a deadline derived from
// the deploy --timeout.
func waitServiceReady(ctx context.Context, stdout io.Writer, outputs map[string]string, deadline time.Time, showProgress bool) error {
	probes := probeTargets(outputs)
	if len(probes) == 0 {
		return fmt.Errorf("cannot wait for service ready: stack outputs missing ServiceURL and PublicIP")
	}

	fmt.Fprintln(stdout, "\nStack CREATE_COMPLETE. Waiting for app bootstrap...")
	printBootstrapWaitHints(stdout, outputs, probeDisplayStrings(probes))
	attempt := 0
	ticker := time.NewTicker(bootstrapPollInterval)
	defer ticker.Stop()

	for {
		attempt++
		for _, probe := range probes {
			ready := false
			if probe.tcpAddr != "" {
				ready = tcpServiceReady(ctx, probe.tcpAddr)
			} else {
				ready = serviceReady(ctx, probe.url)
			}
			if ready {
				if showProgress {
					fmt.Fprintf(stdout, "[%s] bootstrap ready (%s)\n", time.Now().Local().Format("15:04:05"), probe.display)
				}
				fmt.Fprintln(stdout, "Service is ready.")
				return nil
			}
		}

		if time.Now().After(deadline) {
			publicIP := strings.TrimSpace(outputs["PublicIP"])
			if publicIP != "" {
				return fmt.Errorf("timed out waiting for app bootstrap; stack was created but the service is not ready yet (Public IP: %s). Try again in a few minutes or check the instance bootstrap logs (e.g. /var/log/cloud-init-output.log)", publicIP)
			}
			return fmt.Errorf("timed out waiting for app bootstrap; stack was created but the service is not ready yet. Try again in a few minutes or check the instance bootstrap logs (e.g. /var/log/cloud-init-output.log)")
		}

		if showProgress {
			fmt.Fprintf(stdout, "[%s] bootstrap waiting (attempt %d)\n", time.Now().Local().Format("15:04:05"), attempt)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
