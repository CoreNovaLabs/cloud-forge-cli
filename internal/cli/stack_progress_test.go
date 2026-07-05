package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestPrintStackProgressLinePublicIP(t *testing.T) {
	var stdout bytes.Buffer
	printStackProgressLine(&stdout, stackProgressLine{
		Timestamp:         time.Date(2026, 7, 5, 13, 11, 43, 0, time.UTC),
		ResourceType:      "ALIYUN::VPC::EIP",
		LogicalResourceID: "HelloNginxEIP",
		ResourceStatus:    "CREATE_COMPLETE",
		PublicIP:          "203.0.113.10",
	})
	got := stdout.String()
	if !strings.Contains(got, "Public IP: 203.0.113.10") {
		t.Fatalf("expected public IP in progress line, got: %q", got)
	}
}

func TestPrintBootstrapWaitHints(t *testing.T) {
	var stdout bytes.Buffer
	printBootstrapWaitHints(&stdout, map[string]string{
		"PublicIP":   "203.0.113.10",
		"ServiceURL": "https://git.example.com",
	}, []string{"https://git.example.com/health", "https://git.example.com/"})
	got := stdout.String()
	if !strings.Contains(got, "Public IP:      203.0.113.10") {
		t.Fatalf("missing public IP hint: %q", got)
	}
	if !strings.Contains(got, "Service URL:    https://git.example.com") {
		t.Fatalf("missing service URL hint: %q", got)
	}
	if !strings.Contains(got, "https://203.0.113.10/health") {
		t.Fatalf("missing domain lag note: %q", got)
	}
}

func TestPrintBootstrapWaitHintsIPOnly(t *testing.T) {
	var stdout bytes.Buffer
	printBootstrapWaitHints(&stdout, map[string]string{
		"PublicIP":   "203.0.113.10",
		"ServiceURL": "https://203.0.113.10",
	}, []string{"https://203.0.113.10/health", "https://203.0.113.10/"})
	got := stdout.String()
	if !strings.Contains(got, "Public IP:      203.0.113.10") {
		t.Fatalf("missing public IP hint: %q", got)
	}
	if !strings.Contains(got, "Service URL:    https://203.0.113.10") {
		t.Fatalf("missing service URL hint: %q", got)
	}
	if strings.Contains(got, "Domain TLS may lag") {
		t.Fatalf("IP-only deploy should not show domain lag note: %q", got)
	}
}

func TestPrintStackProgressLineAWS(t *testing.T) {
	var stdout bytes.Buffer
	printStackProgressLine(&stdout, stackProgressLine{
		Timestamp:         time.Date(2026, 7, 5, 13, 11, 43, 0, time.UTC),
		ResourceType:      "AWS::EC2::EIP",
		LogicalResourceID: "HelloNginxEIP",
		ResourceStatus:    "CREATE_COMPLETE",
		PublicIP:          "203.0.113.10",
	})
	got := stdout.String()
	if !strings.Contains(got, "Public IP: 203.0.113.10") {
		t.Fatalf("expected public IP in AWS progress line, got: %q", got)
	}
}
