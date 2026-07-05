package cli

import (
	"fmt"
	"io"
	"net"
	"strings"
)

func validateDomainConfig(cloud string, deploy *deployFlags, stderr io.Writer) error {
	domain := strings.TrimSpace(deploy.domainName)
	hostedZone := strings.TrimSpace(deploy.hostedZoneID)
	dnsDomain := strings.TrimSpace(deploy.dnsDomainName)

	if hostedZone != "" && domain == "" {
		return fmt.Errorf("--hosted-zone-id requires --domain")
	}
	if dnsDomain != "" && domain == "" {
		return fmt.Errorf("--dns-domain requires --domain")
	}
	if domain == "" {
		return nil
	}

	if err := validateHostname(domain); err != nil {
		return fmt.Errorf("invalid --domain: %w", err)
	}

	switch cloud {
	case "aws":
		if hostedZone == "" {
			fmt.Fprintln(stderr, "Warning: --domain without --hosted-zone-id will not create a Route53 record; configure DNS manually.")
		}
	case "aliyun":
		if dnsDomain == "" {
			fmt.Fprintln(stderr, "Warning: --domain without --dns-domain will not create an Alidns record; configure DNS manually.")
		} else {
			if err := validateHostname(dnsDomain); err != nil {
				return fmt.Errorf("invalid --dns-domain: %w", err)
			}
			if _, err := resolveDnsRR(domain, dnsDomain); err != nil {
				return err
			}
		}
	}

	if tls := strings.TrimSpace(deploy.caddyTlsMode); domain != "" && tls == "internal" {
		fmt.Fprintln(stderr, "Warning: --caddy-tls-mode=internal is not suitable for a public custom domain.")
	}

	return nil
}

func resolveDnsRR(domain, dnsDomain string) (string, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	dnsDomain = strings.ToLower(strings.TrimSpace(dnsDomain))
	if domain == dnsDomain {
		return "@", nil
	}
	suffix := "." + dnsDomain
	if !strings.HasSuffix(domain, suffix) {
		return "", fmt.Errorf("--domain %q must end with --dns-domain %q", domain, dnsDomain)
	}
	rr := strings.TrimSuffix(domain, suffix)
	if rr == "" {
		return "@", nil
	}
	if strings.Contains(rr, ".") {
		// multi-label RR such as app.staging.example.com under example.com
		for _, label := range strings.Split(rr, ".") {
			if err := validateDNSLabel(label); err != nil {
				return "", fmt.Errorf("invalid DNS host record %q: %w", rr, err)
			}
		}
		return rr, nil
	}
	if err := validateDNSLabel(rr); err != nil {
		return "", fmt.Errorf("invalid DNS host record %q: %w", rr, err)
	}
	return rr, nil
}

func validateHostname(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("hostname is required")
	}
	if strings.Contains(value, "://") || strings.ContainsAny(value, "/:?#") {
		return fmt.Errorf("hostname must not include a URL scheme or path")
	}
	if len(value) > 253 {
		return fmt.Errorf("hostname is too long")
	}
	if ip := net.ParseIP(value); ip != nil {
		return fmt.Errorf("expected a DNS name, not an IP address")
	}
	labels := strings.Split(value, ".")
	for _, label := range labels {
		if err := validateDNSLabel(label); err != nil {
			return err
		}
	}
	return nil
}

func validateDNSLabel(label string) error {
	if label == "" {
		return fmt.Errorf("empty label")
	}
	if len(label) > 63 {
		return fmt.Errorf("label %q is too long", label)
	}
	if label[0] == '-' || label[len(label)-1] == '-' {
		return fmt.Errorf("label %q must not start or end with '-'", label)
	}
	for _, r := range label {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return fmt.Errorf("label %q contains invalid characters", label)
	}
	return nil
}

func printDomainDeployHints(cloud string, deploy *deployFlags, stdout io.Writer) {
	domain := strings.TrimSpace(deploy.domainName)
	if domain == "" {
		return
	}
	fmt.Fprintf(stdout, "Domain:         %s\n", domain)

	switch cloud {
	case "aws":
		if strings.TrimSpace(deploy.hostedZoneID) != "" {
			fmt.Fprintln(stdout, "Route53:        will create an A record when the stack deploys")
		}
	case "aliyun":
		if dnsDomain := strings.TrimSpace(deploy.dnsDomainName); dnsDomain != "" {
			if rr, err := resolveDnsRR(domain, dnsDomain); err == nil {
				fmt.Fprintf(stdout, "Alidns:         will create A record RR=%s in %s\n", rr, dnsDomain)
			}
		} else {
			fmt.Fprintln(stdout, "DNS:            add an A record pointing to the instance EIP after deploy")
		}
	}
}
