package cli

import (
	"fmt"
	"io"
	"strings"
	"time"
)

type stackProgressLine struct {
	Timestamp            time.Time
	ResourceType         string
	LogicalResourceID    string
	ResourceStatus       string
	ResourceStatusReason string
	PublicIP             string
}

func printStackProgressLine(stdout io.Writer, event stackProgressLine) {
	timestamp := event.Timestamp.Local().Format("15:04:05")
	resourceType := stringsTrimOr(event.ResourceType, "-")
	resourceID := stringsTrimOr(event.LogicalResourceID, "-")
	status := stringsTrimOr(event.ResourceStatus, "-")
	fmt.Fprintf(stdout, "[%s] %s %s %s", timestamp, resourceType, resourceID, status)
	if reason := strings.TrimSpace(event.ResourceStatusReason); reason != "" {
		fmt.Fprintf(stdout, " - %s", reason)
	}
	if ip := strings.TrimSpace(event.PublicIP); ip != "" {
		fmt.Fprintf(stdout, " - Public IP: %s", ip)
	}
	fmt.Fprintln(stdout)
}
