package cli

import (
	"fmt"
	"io"
	"strings"
)

func printBootstrapWaitHints(stdout io.Writer, outputs map[string]string, probeURLs []string) {
	publicIP := strings.TrimSpace(outputs["PublicIP"])
	serviceURL := strings.TrimSpace(outputs["ServiceURL"])
	if publicIP != "" {
		fmt.Fprintf(stdout, "Public IP:      %s\n", publicIP)
	}
	if serviceURL != "" {
		fmt.Fprintf(stdout, "Service URL:    %s\n", serviceURL)
	}
	if len(probeURLs) > 0 {
		fmt.Fprintf(stdout, "Probing:        %s\n", strings.Join(probeURLs, ", "))
	}
	if publicIP != "" && serviceURL != "" && !strings.Contains(serviceURL, publicIP) {
		fmt.Fprintf(stdout, "Note: Domain TLS may lag; try https://%s/health meanwhile.\n", publicIP)
	}
}
