package cli

import (
	"fmt"
	"io"
	"net"
	"strings"
)

type domainConfig struct {
	domainName    string
	hostedZoneID  string
	dnsDomainName string
	dnsRR         string
	caddyTlsMode  string
}

func validateDomainConfig(cloud string, deploy *deployFlags, stderr io.Writer) error {
	return validateDomainParameters(cloud, domainConfigFromDeploy(deploy), stderr)
}

func validateDomainParameters(cloud string, params map[string]string, stderr io.Writer) error {
	return validateDomain(cloud, domainConfigFromParams(params), stderr)
}

func validateDomain(cloud string, cfg domainConfig, stderr io.Writer) error {
	domain := strings.TrimSpace(cfg.domainName)
	hostedZone := strings.TrimSpace(cfg.hostedZoneID)
	dnsDomain := strings.TrimSpace(cfg.dnsDomainName)

	switch cloud {
	case "aws":
		if dnsDomain != "" {
			return fmt.Errorf("--dns-domain is only supported with --cloud aliyun")
		}
	case "aliyun":
		if hostedZone != "" {
			return fmt.Errorf("--hosted-zone-id is only supported with --cloud aws")
		}
	}

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

	if tls := strings.TrimSpace(cfg.caddyTlsMode); domain != "" && tls == "internal" {
		fmt.Fprintln(stderr, "Warning: --caddy-tls-mode=internal is not suitable for a public custom domain.")
	}

	return nil
}

func domainConfigFromDeploy(deploy *deployFlags) map[string]string {
	params := map[string]string{
		"DomainName":    deploy.domainName,
		"HostedZoneId":  deploy.hostedZoneID,
		"DnsDomainName": deploy.dnsDomainName,
		"DnsRR":         "",
		"CaddyTlsMode":  deploy.caddyTlsMode,
	}
	if deploy.parameters != nil {
		for _, name := range []string{"DomainName", "HostedZoneId", "DnsDomainName", "DnsRR", "CaddyTlsMode"} {
			if value, ok := deploy.parameters[name]; ok {
				params[name] = value
			}
		}
	}
	return params
}

func domainConfigFromParams(params map[string]string) domainConfig {
	if params == nil {
		return domainConfig{}
	}
	return domainConfig{
		domainName:    params["DomainName"],
		hostedZoneID:  params["HostedZoneId"],
		dnsDomainName: params["DnsDomainName"],
		dnsRR:         params["DnsRR"],
		caddyTlsMode:  params["CaddyTlsMode"],
	}
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
	if deploy.parameters != nil {
		if value, ok := deploy.parameters["DomainName"]; ok {
			domain = strings.TrimSpace(value)
		}
	}
	if domain == "" {
		return
	}
	fmt.Fprintf(stdout, "Domain:         %s\n", domain)

	switch cloud {
	case "aws":
		hostedZoneID := strings.TrimSpace(deploy.hostedZoneID)
		if deploy.parameters != nil {
			if value, ok := deploy.parameters["HostedZoneId"]; ok {
				hostedZoneID = strings.TrimSpace(value)
			}
		}
		if hostedZoneID != "" {
			fmt.Fprintln(stdout, "Route53:        will create an A record when the stack deploys")
		}
	case "aliyun":
		dnsDomain := strings.TrimSpace(deploy.dnsDomainName)
		if deploy.parameters != nil {
			if value, ok := deploy.parameters["DnsDomainName"]; ok {
				dnsDomain = strings.TrimSpace(value)
			}
		}
		if dnsDomain != "" {
			if rr, err := resolveDnsRR(domain, dnsDomain); err == nil {
				fmt.Fprintf(stdout, "Alidns:         will create A record RR=%s in %s\n", rr, dnsDomain)
			}
		} else {
			fmt.Fprintln(stdout, "DNS:            add an A record pointing to the instance EIP after deploy")
		}
	}
}

func manualDomainDNS(cloud string, params map[string]string) bool {
	cfg := domainConfigFromParams(params)
	if strings.TrimSpace(cfg.domainName) == "" {
		return false
	}
	switch cloud {
	case "aws":
		return strings.TrimSpace(cfg.hostedZoneID) == ""
	case "aliyun":
		return strings.TrimSpace(cfg.dnsDomainName) == ""
	default:
		return false
	}
}

func shouldWaitForServiceReady(cloud string, deploy *deployFlags, waitReadyExplicit bool, params map[string]string) bool {
	if !deploy.waitReady || deploy.noWaitReady {
		return false
	}
	if manualDomainDNS(cloud, params) && !waitReadyExplicit {
		return false
	}
	return true
}

func printManualDomainDNSWaitSkipped(cloud string, params, outputs map[string]string, stdout io.Writer) {
	cfg := domainConfigFromParams(params)
	domain := strings.TrimSpace(cfg.domainName)
	publicIP := strings.TrimSpace(outputs["PublicIP"])

	var dnsFlag string
	switch cloud {
	case "aws":
		dnsFlag = "--hosted-zone-id"
	case "aliyun":
		dnsFlag = "--dns-domain"
	default:
		dnsFlag = "automatic DNS"
	}

	fmt.Fprintf(stdout, "\nNote: --domain was provided without %s, so Cloud Forge skipped the default endpoint wait.\n", dnsFlag)
	if domain != "" && publicIP != "" {
		fmt.Fprintf(stdout, "Add an A record for %s pointing to %s, then open the ServiceURL. Pass --wait-ready explicitly to wait anyway.\n", domain, publicIP)
		return
	}
	fmt.Fprintln(stdout, "Configure the DNS A record after the stack outputs are available. Pass --wait-ready explicitly to wait anyway.")
}
