package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/cloud-forge/cli/internal/awsdeploy"
	"github.com/cloud-forge/cli/pkg/store"
)

func TestResolveDnsRR(t *testing.T) {
	t.Parallel()

	cases := []struct {
		domain    string
		dnsDomain string
		want      string
		wantErr   bool
	}{
		{"git.example.com", "example.com", "git", false},
		{"example.com", "example.com", "@", false},
		{"app.staging.example.com", "example.com", "app.staging", false},
		{"git.other.com", "example.com", "", true},
	}

	for _, tc := range cases {
		got, err := resolveDnsRR(tc.domain, tc.dnsDomain)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("resolveDnsRR(%q, %q) expected error", tc.domain, tc.dnsDomain)
			}
			continue
		}
		if err != nil {
			t.Fatalf("resolveDnsRR(%q, %q): %v", tc.domain, tc.dnsDomain, err)
		}
		if got != tc.want {
			t.Fatalf("resolveDnsRR(%q, %q) = %q, want %q", tc.domain, tc.dnsDomain, got, tc.want)
		}
	}
}

func TestValidateDomainConfigAWS(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	deploy := &deployFlags{
		hostedZoneID: "Z123",
	}
	if err := validateDomainConfig("aws", deploy, &stderr); err == nil {
		t.Fatal("expected error when hosted zone without domain")
	}

	stderr.Reset()
	deploy = &deployFlags{domainName: "git.example.com"}
	if err := validateDomainConfig("aws", deploy, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "Route53") {
		t.Fatalf("expected Route53 warning, got %q", stderr.String())
	}
}

func TestValidateDomainConfigAliyun(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	deploy := &deployFlags{dnsDomainName: "example.com"}
	if err := validateDomainConfig("aliyun", deploy, &stderr); err == nil {
		t.Fatal("expected error when dns-domain without domain")
	}

	stderr.Reset()
	deploy = &deployFlags{
		domainName:    "git.example.com",
		dnsDomainName: "example.com",
	}
	if err := validateDomainConfig("aliyun", deploy, &stderr); err != nil {
		t.Fatal(err)
	}
}

func TestValidateDomainConfigRejectsCrossCloudDNSFlags(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	if err := validateDomainConfig("aws", &deployFlags{
		domainName:    "git.example.com",
		dnsDomainName: "example.com",
	}, &stderr); err == nil || !strings.Contains(err.Error(), "--dns-domain") {
		t.Fatalf("expected aws to reject --dns-domain, got %v", err)
	}

	if err := validateDomainConfig("aliyun", &deployFlags{
		domainName:   "git.example.com",
		hostedZoneID: "Z123",
	}, &stderr); err == nil || !strings.Contains(err.Error(), "--hosted-zone-id") {
		t.Fatalf("expected aliyun to reject --hosted-zone-id, got %v", err)
	}
}

func TestValidateDomainConfigUsesParamOverrides(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	deploy := &deployFlags{
		parameters: keyValueFlag{
			"DomainName": "https://git.example.com",
		},
	}
	if err := validateDomainConfig("aws", deploy, &stderr); err == nil || !strings.Contains(err.Error(), "URL scheme") {
		t.Fatalf("expected invalid DomainName override, got %v", err)
	}

	deploy = &deployFlags{
		parameters: keyValueFlag{
			"DomainName":    "git.example.com",
			"DnsDomainName": "example.com",
		},
	}
	if err := validateDomainConfig("aws", deploy, &stderr); err == nil || !strings.Contains(err.Error(), "--dns-domain") {
		t.Fatalf("expected aws to reject DnsDomainName override, got %v", err)
	}
}

func TestBuildAliyunDeployParametersDomain(t *testing.T) {
	t.Parallel()

	app := &store.App{
		Params: map[string]store.ParamDefinition{
			"DomainName":    {Type: "string", Default: ""},
			"DnsDomainName": {Type: "string", Aliyun: &store.CloudParam{Default: ""}},
			"DnsRR":         {Type: "string", Aliyun: &store.CloudParam{Default: ""}},
			"CaddyEmail":    {Type: "string", Default: ""},
			"CaddyTlsMode": {
				Type:   "string",
				Aliyun: &store.CloudParam{Default: "ip-letsencrypt", Options: []string{"auto", "ip-letsencrypt", "http", "internal"}},
			},
			"KeyPairName": {Type: "string", Aliyun: &store.CloudParam{Required: true}},
			"VpcId":       {Type: "string", Aliyun: &store.CloudParam{Required: true}},
			"VSwitchId":   {Type: "string", Aliyun: &store.CloudParam{Required: true}},
		},
	}
	deploy := &deployFlags{
		domainName:    "git.example.com",
		dnsDomainName: "example.com",
		caddyEmail:    "ops@example.com",
		keyName:       "test-key",
		vpcID:         "vpc-test",
		vswitchID:     "vsw-test",
	}
	params, err := buildAliyunDeployParameters(app, deploy)
	if err != nil {
		t.Fatal(err)
	}
	if params["DomainName"] != "git.example.com" {
		t.Fatalf("DomainName = %q", params["DomainName"])
	}
	if params["DnsDomainName"] != "example.com" {
		t.Fatalf("DnsDomainName = %q", params["DnsDomainName"])
	}
	if params["DnsRR"] != "git" {
		t.Fatalf("DnsRR = %q", params["DnsRR"])
	}
	if params["CaddyEmail"] != "ops@example.com" {
		t.Fatalf("CaddyEmail = %q", params["CaddyEmail"])
	}
}

func TestBuildAliyunDeployParametersDomainParamOverrides(t *testing.T) {
	t.Parallel()

	app := &store.App{
		Params: map[string]store.ParamDefinition{
			"DomainName":    {Type: "string", Default: ""},
			"DnsDomainName": {Type: "string", Aliyun: &store.CloudParam{Default: ""}},
			"DnsRR":         {Type: "string", Aliyun: &store.CloudParam{Default: ""}},
			"KeyPairName":   {Type: "string", Aliyun: &store.CloudParam{Required: true}},
			"VpcId":         {Type: "string", Aliyun: &store.CloudParam{Required: true}},
			"VSwitchId":     {Type: "string", Aliyun: &store.CloudParam{Required: true}},
		},
	}
	deploy := &deployFlags{
		keyName:   "test-key",
		vpcID:     "vpc-test",
		vswitchID: "vsw-test",
		parameters: keyValueFlag{
			"DomainName":    "git.example.com",
			"DnsDomainName": "example.com",
		},
	}
	params, err := buildAliyunDeployParameters(app, deploy)
	if err != nil {
		t.Fatal(err)
	}
	if params["DnsRR"] != "git" {
		t.Fatalf("DnsRR = %q", params["DnsRR"])
	}
}

func TestBuildAWSDeployParametersDomain(t *testing.T) {
	t.Parallel()

	app := &store.App{
		Params: map[string]store.ParamDefinition{
			"DomainName":   {Type: "string", Default: ""},
			"HostedZoneId": {Type: "string", AWS: &store.CloudParam{Default: ""}},
			"CaddyEmail":   {Type: "string", Default: ""},
			"CaddyTlsMode": {
				Type: "string",
				AWS:  &store.CloudParam{Default: "ip-letsencrypt", Options: []string{"auto", "ip-letsencrypt", "http", "internal"}},
			},
		},
	}
	deploy := &deployFlags{
		domainName:   "git.example.com",
		hostedZoneID: "Z123",
		caddyEmail:   "ops@example.com",
	}
	params, err := buildAWSDeployParameters(context.Background(), app, deploy, awsdeploy.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if params["HostedZoneId"] != "Z123" {
		t.Fatalf("HostedZoneId = %q", params["HostedZoneId"])
	}
	if params["CaddyEmail"] != "ops@example.com" {
		t.Fatalf("CaddyEmail = %q", params["CaddyEmail"])
	}
}
