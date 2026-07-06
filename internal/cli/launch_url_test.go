package cli

import (
	"reflect"
	"strings"
	"testing"
)

func TestDeployArgsFromLaunchURL(t *testing.T) {
	args, err := deployArgsFromLaunchURL("cloud-forge://deploy?app=actual-budget&cloud=aws&region=us-east-1&stackName=cloud-forge-actual-budget&param.InstanceType=t3.small&param.AllowedIP=1.2.3.4%2F32")
	if err != nil {
		t.Fatalf("deployArgsFromLaunchURL() error = %v", err)
	}
	want := []string{
		"deploy", "actual-budget",
		"--cloud", "aws",
		"--region", "us-east-1",
		"--stack-name", "cloud-forge-actual-budget",
		"--param", "AllowedIP=1.2.3.4/32",
		"--param", "InstanceType=t3.small",
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("deployArgsFromLaunchURL() = %#v, want %#v", args, want)
	}
}

func TestDeployArgsFromLaunchURLSupportsPathAppAndBooleans(t *testing.T) {
	args, err := deployArgsFromLaunchURL("cloud-forge://deploy/bazarr?cloud=aliyun&region=cn-hongkong&dryRun=true&noWaitReady=1&param.InstanceType=ecs.t6-c1m1.large")
	if err != nil {
		t.Fatalf("deployArgsFromLaunchURL() error = %v", err)
	}
	want := []string{
		"deploy", "bazarr",
		"--cloud", "aliyun",
		"--region", "cn-hongkong",
		"--dry-run",
		"--no-wait-ready",
		"--param", "InstanceType=ecs.t6-c1m1.large",
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("deployArgsFromLaunchURL() = %#v, want %#v", args, want)
	}
}

func TestDeployArgsFromLaunchURLSupportsCacheTTL(t *testing.T) {
	args, err := deployArgsFromLaunchURL("cloud-forge://deploy?app=hello-nginx&cloud=aws&cacheTTL=0s")
	if err != nil {
		t.Fatalf("deployArgsFromLaunchURL() error = %v", err)
	}
	want := []string{
		"deploy", "hello-nginx",
		"--cloud", "aws",
		"--cache-ttl", "0s",
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("deployArgsFromLaunchURL() = %#v, want %#v", args, want)
	}
}

func TestDeployArgsFromLaunchURLRejectsInvalidInput(t *testing.T) {
	tests := []string{
		"https://example.com/deploy?app=actual-budget",
		"cloud-forge://delete?app=actual-budget",
		"cloud-forge://deploy?cloud=gcp&app=actual-budget",
		"cloud-forge://deploy?cloud=aws",
	}
	for _, raw := range tests {
		t.Run(raw, func(t *testing.T) {
			if _, err := deployArgsFromLaunchURL(raw); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestRunLaunchURLPrintCommand(t *testing.T) {
	var stdout strings.Builder
	var stderr strings.Builder
	code := runLaunchURL(
		t.Context(),
		[]string{"--print-command", "cloud-forge://deploy?app=actual-budget&cloud=aws&param.DomainName=budget.example.com"},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	if code != 0 {
		t.Fatalf("exit code %d, stderr=%s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "cloud-forge deploy actual-budget --cloud aws") || !strings.Contains(got, "--param DomainName=budget.example.com") {
		t.Fatalf("unexpected command output: %s", got)
	}
}
