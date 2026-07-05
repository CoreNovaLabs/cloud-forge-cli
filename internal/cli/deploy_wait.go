package cli

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// bootstrapPollInterval is the interval between service-ready probes after a
// stack reaches CREATE_COMPLETE. It is shared by AWS and Aliyun deploys.
// Tests may shorten it to speed up assertions.
var bootstrapPollInterval = 30 * time.Second

// serviceHTTPClient probes the deployed endpoint. TLS verification is skipped
// because the endpoint is typically an IP address carrying an IP-based
// certificate that does not match a hostname.
var serviceHTTPClient = &http.Client{
	Timeout: 15 * time.Second,
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // IP certs may not match hostname
	},
}

// probeURLs derives candidate health-check URLs from stack outputs. It prefers
// ServiceURL and falls back to https://<PublicIP>. Both /health and the root
// path are probed so apps without a dedicated health endpoint still report
// readiness. The logic is cloud-agnostic: any deployer that exposes a
// ServiceURL or PublicIP output can use it.
func probeURLs(outputs map[string]string) []string {
	base := strings.TrimSpace(outputs["ServiceURL"])
	if base == "" {
		if ip := strings.TrimSpace(outputs["PublicIP"]); ip != "" {
			base = "https://" + ip
		}
	}
	if base == "" {
		return nil
	}
	base = strings.TrimRight(base, "/")
	return []string{base + "/health", base + "/"}
}

// serviceReady reports whether the given URL responds with a 2xx status.
func serviceReady(ctx context.Context, url string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := serviceHTTPClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

// waitServiceReady polls the deployed endpoint until it responds 2xx or the
// deadline expires. It is cloud-agnostic; callers pass the stack outputs and
// a deadline derived from the deploy --timeout.
func waitServiceReady(ctx context.Context, stdout io.Writer, outputs map[string]string, deadline time.Time, showProgress bool) error {
	urls := probeURLs(outputs)
	if len(urls) == 0 {
		return fmt.Errorf("cannot wait for service ready: stack outputs missing ServiceURL and PublicIP")
	}

	fmt.Fprintln(stdout, "\nStack CREATE_COMPLETE. Waiting for app bootstrap...")
	printBootstrapWaitHints(stdout, outputs, urls)
	attempt := 0
	ticker := time.NewTicker(bootstrapPollInterval)
	defer ticker.Stop()

	for {
		attempt++
		for _, url := range urls {
			if serviceReady(ctx, url) {
				if showProgress {
					fmt.Fprintf(stdout, "[%s] bootstrap ready (%s)\n", time.Now().Local().Format("15:04:05"), url)
				}
				fmt.Fprintln(stdout, "Service is ready.")
				return nil
			}
		}

		if time.Now().After(deadline) {
			publicIP := strings.TrimSpace(outputs["PublicIP"])
			if publicIP != "" {
				return fmt.Errorf("timed out waiting for app bootstrap; stack was created but ServiceURL is not responding yet (Public IP: %s). Try again in a few minutes or check the instance bootstrap logs (e.g. /var/log/cloud-init-output.log)", publicIP)
			}
			return fmt.Errorf("timed out waiting for app bootstrap; stack was created but ServiceURL is not responding yet. Try again in a few minutes or check the instance bootstrap logs (e.g. /var/log/cloud-init-output.log)")
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
