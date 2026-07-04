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

var aliyunBootstrapPollInterval = 30 * time.Second

var aliyunServiceHTTPClient = &http.Client{
	Timeout: 15 * time.Second,
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // IP certs may not match hostname
	},
}

func aliyunProbeURLs(outputs map[string]string) []string {
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

func aliyunServiceReady(ctx context.Context, url string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := aliyunServiceHTTPClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func waitAliyunServiceReady(ctx context.Context, stdout io.Writer, outputs map[string]string, deadline time.Time, showProgress bool) error {
	urls := aliyunProbeURLs(outputs)
	if len(urls) == 0 {
		return fmt.Errorf("cannot wait for service ready: stack outputs missing ServiceURL and PublicIP")
	}

	fmt.Fprintln(stdout, "\nStack CREATE_COMPLETE. Waiting for app bootstrap (may take 8-15 minutes)...")
	attempt := 0
	ticker := time.NewTicker(aliyunBootstrapPollInterval)
	defer ticker.Stop()

	for {
		attempt++
		for _, url := range urls {
			if aliyunServiceReady(ctx, url) {
				if showProgress {
					fmt.Fprintf(stdout, "[%s] bootstrap ready (%s)\n", time.Now().Local().Format("15:04:05"), url)
				}
				fmt.Fprintln(stdout, "Service is ready.")
				return nil
			}
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for app bootstrap; stack was created but ServiceURL is not responding yet. Try again in a few minutes or check /var/log/cloud-init-output.log on the instance")
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
