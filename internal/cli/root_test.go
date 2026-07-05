package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cloud-forge/cli/internal/aliyundeploy"
	"github.com/cloud-forge/cli/internal/awsdeploy"
)

func TestSearchUsesLocalCatalog(t *testing.T) {
	t.Setenv("CLOUD_FORGE_TELEMETRY", "0")
	indexPath := writeTestCatalog(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"search",
		"gitea",
		"--store-url",
		fileURL(indexPath),
		"--cache-dir",
		t.TempDir(),
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "gitea") {
		t.Fatalf("expected search output to include gitea, got: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "free") {
		t.Fatalf("expected search output to include free price, got: %s", stdout.String())
	}
}

func TestSearchRefreshesCatalogOnMiss(t *testing.T) {
	t.Setenv("CLOUD_FORGE_TELEMETRY", "0")

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		appID := "gitea"
		if requests > 1 {
			appID = "freshrss"
		}
		fmt.Fprintf(w, `{
  "catalog_version": "1.0.0",
  "generated_at": "2026-06-30T00:00:00Z",
  "store": {"name": "Test Store", "description": "test"},
  "apps": [{
    "id": %q,
    "name": %q,
    "desc": "test app",
    "category": "devtools",
    "clouds": ["aws"],
    "version": "1.0.0",
    "price": "free",
    "images": {"aws": "ami-0123456789abcdef0"},
    "templates": {"aws": {"path": "apps/%s/templates/aws.yaml"}}
  }]
}`, appID, appID, appID)
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	indexURL := server.URL + "/index/apps.json"

	warm := Run(context.Background(), []string{
		"search", "gitea",
		"--store-url", indexURL,
		"--cache-dir", cacheDir,
	}, io.Discard, io.Discard)
	if warm != 0 {
		t.Fatalf("warmup search exit code %d", warm)
	}
	if requests != 1 {
		t.Fatalf("expected 1 index request after warmup, got %d", requests)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"search", "freshrss",
		"--store-url", indexURL,
		"--cache-dir", cacheDir,
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "freshrss") {
		t.Fatalf("expected search output to include freshrss, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if requests != 2 {
		t.Fatalf("expected 2 index requests after refresh-on-miss, got %d", requests)
	}
}

func TestTemplateReadsLocalTemplateBeforeRemoteURL(t *testing.T) {
	t.Setenv("CLOUD_FORGE_TELEMETRY", "0")
	indexPath := writeTestCatalog(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"template",
		"gitea",
		"--cloud",
		"aws",
		"--store-url",
		fileURL(indexPath),
		"--cache-dir",
		t.TempDir(),
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); got != "Resources: {}\n" {
		t.Fatalf("unexpected template body: %q", got)
	}
}

func TestTemplateTracksTelemetry(t *testing.T) {
	t.Setenv("CLOUD_FORGE_TELEMETRY", "1")
	indexPath := writeTestCatalog(t)

	events := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var event map[string]any
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			t.Fatal(err)
		}
		events <- event
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"template",
		"gitea",
		"--cloud",
		"aws",
		"--store-url",
		fileURL(indexPath),
		"--cache-dir",
		t.TempDir(),
		"--telemetry-endpoint",
		server.URL,
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
	}

	select {
	case event := <-events:
		if event["event"] != "template_fetch" {
			t.Fatalf("unexpected event %v", event["event"])
		}
		if event["app_id"] != "gitea" {
			t.Fatalf("unexpected app id %v", event["app_id"])
		}
		if event["cloud"] != "aws" {
			t.Fatalf("unexpected cloud %v", event["cloud"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for telemetry")
	}
}

func TestShowPrintsCostNotice(t *testing.T) {
	t.Setenv("CLOUD_FORGE_TELEMETRY", "0")
	indexPath := writeTestCatalog(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"show",
		"gitea",
		"--store-url",
		fileURL(indexPath),
		"--cache-dir",
		t.TempDir(),
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"Price:       free",
		"Cost notice:",
		"This deploy has base costs.",
		"Cloud Forge runtime AMI fee applies.",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected show output to include %q, got: %s", want, out)
		}
	}
}

func TestDeployAWSUsesCatalogTemplateAndParameters(t *testing.T) {
	t.Setenv("CLOUD_FORGE_TELEMETRY", "0")
	indexPath := writeTestCatalog(t)

	var gotConfig awsdeploy.Config
	var gotInput awsdeploy.DeployInput
	oldNewAWSDeployer := newAWSDeployer
	newAWSDeployer = func(ctx context.Context, cfg awsdeploy.Config) (awsStackDeployer, error) {
		gotConfig = cfg
		return fakeAWSDeployer{input: &gotInput}, nil
	}
	t.Cleanup(func() {
		newAWSDeployer = oldNewAWSDeployer
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"deploy",
		"--dry-run",
		"gitea",
		"--cloud",
		"aws",
		"--region",
		"us-east-1",
		"--stack-name",
		"test-gitea",
		"--store-url",
		fileURL(indexPath),
		"--cache-dir",
		t.TempDir(),
		"--instance-type",
		"t3.small",
		"--param",
		"KeyName=my-key",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
	}
	if gotConfig.Region != "us-east-1" {
		t.Fatalf("unexpected region %q", gotConfig.Region)
	}
	if gotInput.StackName != "test-gitea" {
		t.Fatalf("unexpected stack name %q", gotInput.StackName)
	}
	if gotInput.TemplateBody != "Resources: {}\n" {
		t.Fatalf("unexpected template body %q", gotInput.TemplateBody)
	}
	if !gotInput.DryRun {
		t.Fatal("expected dry-run deploy input")
	}
	if !gotInput.Wait {
		t.Fatal("expected deploy to wait by default")
	}
	if gotInput.Parameters["InstanceType"] != "t3.small" {
		t.Fatalf("unexpected InstanceType %q", gotInput.Parameters["InstanceType"])
	}
	if gotInput.Parameters["KeyName"] != "my-key" {
		t.Fatalf("unexpected KeyName %q", gotInput.Parameters["KeyName"])
	}
	if gotInput.Parameters["AllowedIP"] != "0.0.0.0/0" {
		t.Fatalf("unexpected AllowedIP default %q", gotInput.Parameters["AllowedIP"])
	}
	if !strings.Contains(stdout.String(), "Action:      validated") {
		t.Fatalf("expected deploy output to include action, got: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Stack name:  test-gitea") {
		t.Fatalf("expected deploy output to include stack name, got: %s", stdout.String())
	}
}

func TestDeployAWSImageIDAliasesLatestAMI(t *testing.T) {
	t.Setenv("CLOUD_FORGE_TELEMETRY", "0")
	indexPath := writeTestCatalog(t)

	var gotInput awsdeploy.DeployInput
	oldNewAWSDeployer := newAWSDeployer
	newAWSDeployer = func(ctx context.Context, cfg awsdeploy.Config) (awsStackDeployer, error) {
		return fakeAWSDeployer{input: &gotInput}, nil
	}
	t.Cleanup(func() {
		newAWSDeployer = oldNewAWSDeployer
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"deploy",
		"--dry-run",
		"gitea",
		"--cloud",
		"aws",
		"--store-url",
		fileURL(indexPath),
		"--cache-dir",
		t.TempDir(),
		"--image-id",
		"ami-0abc123def4567890",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
	}
	if gotInput.Parameters["LatestAmiId"] != "ami-0abc123def4567890" {
		t.Fatalf("LatestAmiId = %q", gotInput.Parameters["LatestAmiId"])
	}
	if _, ok := gotInput.Parameters["ImageId"]; ok {
		t.Fatalf("did not expect AWS ImageId parameter, got %#v", gotInput.Parameters)
	}
}

func TestDeployAWSManualDomainDNSSkipsReadyWaitByDefault(t *testing.T) {
	t.Setenv("CLOUD_FORGE_TELEMETRY", "0")
	indexPath := writeTestCatalog(t)

	var gotInput awsdeploy.DeployInput
	oldNewAWSDeployer := newAWSDeployer
	newAWSDeployer = func(ctx context.Context, cfg awsdeploy.Config) (awsStackDeployer, error) {
		return fakeAWSDeployer{
			input: &gotInput,
			output: &awsdeploy.DeployOutput{
				Action:    "created",
				Region:    "us-east-1",
				AccountID: "123456789012",
				StackName: "cloud-forge-gitea",
				Status:    "CREATE_COMPLETE",
				Outputs: map[string]string{
					"DomainName": "git.example.com",
					"PublicIP":   "203.0.113.10",
					"ServiceURL": "http://127.0.0.1:1",
				},
			},
		}, nil
	}
	t.Cleanup(func() {
		newAWSDeployer = oldNewAWSDeployer
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"deploy",
		"gitea",
		"--cloud",
		"aws",
		"--store-url",
		fileURL(indexPath),
		"--cache-dir",
		t.TempDir(),
		"--ssh-key",
		"none",
		"--domain",
		"git.example.com",
		"--timeout",
		"50ms",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s stdout: %s", code, stderr.String(), stdout.String())
	}
	if !gotInput.Wait {
		t.Fatal("expected stack wait to remain enabled")
	}
	if !strings.Contains(stdout.String(), "skipped the default endpoint wait") {
		t.Fatalf("expected manual DNS wait skip note, got: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "Service is ready.") {
		t.Fatalf("did not expect service ready wait, got: %s", stdout.String())
	}
}

func TestDeployAWSAutoSSHKeyEnsuresDefaultKeyPair(t *testing.T) {
	t.Setenv("CLOUD_FORGE_TELEMETRY", "0")
	indexPath := writeTestCatalog(t)

	var gotInput awsdeploy.DeployInput
	var gotDeployerConfig awsdeploy.Config
	oldNewAWSDeployer := newAWSDeployer
	newAWSDeployer = func(ctx context.Context, cfg awsdeploy.Config) (awsStackDeployer, error) {
		gotDeployerConfig = cfg
		return fakeAWSDeployer{input: &gotInput}, nil
	}
	t.Cleanup(func() {
		newAWSDeployer = oldNewAWSDeployer
	})

	var gotConfig awsdeploy.Config
	var gotKeyInput awsdeploy.EnsureKeyPairInput
	oldEnsureAWSKeyPair := ensureAWSKeyPair
	ensureAWSKeyPair = func(ctx context.Context, cfg awsdeploy.Config, input awsdeploy.EnsureKeyPairInput) (*awsdeploy.EnsureKeyPairOutput, error) {
		gotConfig = cfg
		gotKeyInput = input
		return &awsdeploy.EnsureKeyPairOutput{
			KeyName:        input.KeyName,
			PrivateKeyPath: "/tmp/cloud-forge-default.pem",
		}, nil
	}
	t.Cleanup(func() {
		ensureAWSKeyPair = oldEnsureAWSKeyPair
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"deploy",
		"gitea",
		"--cloud",
		"aws",
		"--stack-name",
		"test-gitea",
		"--store-url",
		fileURL(indexPath),
		"--cache-dir",
		t.TempDir(),
		"--no-wait-ready",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
	}
	if gotDeployerConfig.Region != "us-east-1" {
		t.Fatalf("unexpected deployer region %q", gotDeployerConfig.Region)
	}
	if gotConfig.Region != "us-east-1" {
		t.Fatalf("unexpected key manager region %q", gotConfig.Region)
	}
	if gotKeyInput.KeyName != awsdeploy.DefaultKeyPairName {
		t.Fatalf("unexpected ensured key name %q", gotKeyInput.KeyName)
	}
	if gotInput.Parameters["KeyName"] != awsdeploy.DefaultKeyPairName {
		t.Fatalf("unexpected KeyName %q", gotInput.Parameters["KeyName"])
	}
	if !strings.Contains(stdout.String(), "SSH key pair: cloud-forge-default") {
		t.Fatalf("expected deploy output to include SSH key pair, got: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "SSH private key: /tmp/cloud-forge-default.pem") {
		t.Fatalf("expected deploy output to include SSH private key, got: %s", stdout.String())
	}
}

func TestDeployAWSPrintsCloudFormationProgress(t *testing.T) {
	t.Setenv("CLOUD_FORGE_TELEMETRY", "0")
	indexPath := writeTestCatalog(t)

	var gotInput awsdeploy.DeployInput
	oldNewAWSDeployer := newAWSDeployer
	newAWSDeployer = func(ctx context.Context, cfg awsdeploy.Config) (awsStackDeployer, error) {
		return fakeAWSDeployer{
			input: &gotInput,
			progressEvents: []awsdeploy.StackProgressEvent{
				{
					Timestamp:         time.Date(2026, 7, 3, 12, 1, 8, 0, time.UTC),
					ResourceType:      "AWS::EC2::SecurityGroup",
					LogicalResourceID: "HelloSecurityGroup",
					ResourceStatus:    "CREATE_COMPLETE",
				},
			},
		}, nil
	}
	t.Cleanup(func() {
		newAWSDeployer = oldNewAWSDeployer
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"deploy",
		"gitea",
		"--cloud",
		"aws",
		"--region",
		"us-east-1",
		"--stack-name",
		"test-gitea",
		"--store-url",
		fileURL(indexPath),
		"--cache-dir",
		t.TempDir(),
		"--param",
		"KeyName=my-key",
		"--no-wait-ready",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
	}
	if gotInput.Progress == nil {
		t.Fatal("expected progress callback")
	}
	if !strings.Contains(stdout.String(), "AWS::EC2::SecurityGroup HelloSecurityGroup CREATE_COMPLETE") {
		t.Fatalf("expected progress output, got: %s", stdout.String())
	}
}

func TestDeployAWSProgressNoneDisablesProgress(t *testing.T) {
	t.Setenv("CLOUD_FORGE_TELEMETRY", "0")
	indexPath := writeTestCatalog(t)

	var gotInput awsdeploy.DeployInput
	oldNewAWSDeployer := newAWSDeployer
	newAWSDeployer = func(ctx context.Context, cfg awsdeploy.Config) (awsStackDeployer, error) {
		return fakeAWSDeployer{input: &gotInput}, nil
	}
	t.Cleanup(func() {
		newAWSDeployer = oldNewAWSDeployer
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"deploy",
		"gitea",
		"--cloud",
		"aws",
		"--region",
		"us-east-1",
		"--stack-name",
		"test-gitea",
		"--store-url",
		fileURL(indexPath),
		"--cache-dir",
		t.TempDir(),
		"--param",
		"KeyName=my-key",
		"--progress",
		"none",
		"--no-wait-ready",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
	}
	if gotInput.Progress != nil {
		t.Fatal("expected progress callback to be disabled")
	}
}

func TestDeployAWSRequiresCatalogParameters(t *testing.T) {
	t.Setenv("CLOUD_FORGE_TELEMETRY", "0")
	indexPath := writeRequiredParamCatalog(t)

	oldNewAWSDeployer := newAWSDeployer
	newAWSDeployer = func(ctx context.Context, cfg awsdeploy.Config) (awsStackDeployer, error) {
		t.Fatal("AWS deployer should not be constructed when required params are missing")
		return nil, nil
	}
	t.Cleanup(func() {
		newAWSDeployer = oldNewAWSDeployer
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"deploy",
		"gitea",
		"--cloud",
		"aws",
		"--region",
		"us-east-1",
		"--store-url",
		fileURL(indexPath),
		"--cache-dir",
		t.TempDir(),
		"--ssh-key",
		"none",
		"--param",
		"RequiredToken=",
	}, &stdout, &stderr)

	if code != 2 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "missing required aws parameter(s): RequiredToken") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestDeployAWSWaitsForReadyByDefault(t *testing.T) {
	t.Setenv("CLOUD_FORGE_TELEMETRY", "0")
	indexPath := writeTestCatalog(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	oldNewAWSDeployer := newAWSDeployer
	newAWSDeployer = func(ctx context.Context, cfg awsdeploy.Config) (awsStackDeployer, error) {
		return fakeAWSDeployer{
			input: &awsdeploy.DeployInput{},
			output: &awsdeploy.DeployOutput{
				Action:    "created",
				Region:    "us-east-1",
				AccountID: "123456789012",
				StackName: "cloud-forge-gitea",
				Status:    "CREATE_COMPLETE",
				Outputs:   map[string]string{"ServiceURL": server.URL},
			},
		}, nil
	}
	t.Cleanup(func() { newAWSDeployer = oldNewAWSDeployer })

	oldInterval := bootstrapPollInterval
	bootstrapPollInterval = 10 * time.Millisecond
	t.Cleanup(func() { bootstrapPollInterval = oldInterval })

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"deploy",
		"gitea",
		"--cloud",
		"aws",
		"--region",
		"us-east-1",
		"--stack-name",
		"test-gitea",
		"--store-url",
		fileURL(indexPath),
		"--cache-dir",
		t.TempDir(),
		"--ssh-key",
		"none",
		"--timeout",
		"30s",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Service is ready.") {
		t.Fatalf("expected ready message, got: %s", stdout.String())
	}
}

func TestDeployAWSNoWaitReadySkipsProbe(t *testing.T) {
	t.Setenv("CLOUD_FORGE_TELEMETRY", "0")
	indexPath := writeTestCatalog(t)

	oldNewAWSDeployer := newAWSDeployer
	newAWSDeployer = func(ctx context.Context, cfg awsdeploy.Config) (awsStackDeployer, error) {
		return fakeAWSDeployer{
			input: &awsdeploy.DeployInput{},
			output: &awsdeploy.DeployOutput{
				Action:    "created",
				Region:    "us-east-1",
				AccountID: "123456789012",
				StackName: "cloud-forge-gitea",
				Status:    "CREATE_COMPLETE",
				Outputs:   map[string]string{"ServiceURL": "https://127.0.0.1:1"},
			},
		}, nil
	}
	t.Cleanup(func() { newAWSDeployer = oldNewAWSDeployer })

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"deploy",
		"gitea",
		"--cloud",
		"aws",
		"--region",
		"us-east-1",
		"--stack-name",
		"test-gitea",
		"--store-url",
		fileURL(indexPath),
		"--cache-dir",
		t.TempDir(),
		"--ssh-key",
		"none",
		"--no-wait-ready",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "Service is ready.") {
		t.Fatalf("did not expect bootstrap wait, got: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "app bootstrap may still take") {
		t.Fatalf("expected bootstrap note, got: %s", stdout.String())
	}
}

func TestDeployAliyunAutoDefaults(t *testing.T) {
	t.Setenv("CLOUD_FORGE_TELEMETRY", "0")
	indexPath := writeAliyunTestCatalog(t)

	oldResolve := resolveAliyunDeployDefaults
	resolveAliyunDeployDefaults = func(ctx context.Context, cfg aliyundeploy.Config, req aliyundeploy.DeployDefaultsRequest) (aliyundeploy.DeployDefaultsResult, error) {
		if !req.AutoVpc || !req.AutoVSwitch || !req.AutoKey {
			t.Fatalf("expected auto discovery flags, got %+v", req)
		}
		return aliyundeploy.DeployDefaultsResult{
			VpcID:       "vpc-auto",
			VSwitchID:   "vsw-auto",
			KeyPairName: "cloud-forge-default",
			Messages:    []string{"Using VPC vpc-auto and VSwitch vsw-auto"},
		}, nil
	}
	t.Cleanup(func() { resolveAliyunDeployDefaults = oldResolve })

	var gotInput aliyundeploy.DeployInput
	oldNewAliyunDeployer := newAliyunDeployer
	newAliyunDeployer = func(ctx context.Context, cfg aliyundeploy.Config) (aliyunStackDeployer, error) {
		return fakeAliyunDeployer{input: &gotInput}, nil
	}
	t.Cleanup(func() { newAliyunDeployer = oldNewAliyunDeployer })

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"deploy",
		"hello-nginx",
		"--cloud",
		"aliyun",
		"--store-url",
		fileURL(indexPath),
		"--cache-dir",
		t.TempDir(),
		"--dry-run",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
	}
	if gotInput.Parameters["VpcId"] != "vpc-auto" {
		t.Fatalf("VpcId = %q", gotInput.Parameters["VpcId"])
	}
	if gotInput.Parameters["VSwitchId"] != "vsw-auto" {
		t.Fatalf("VSwitchId = %q", gotInput.Parameters["VSwitchId"])
	}
	if gotInput.Parameters["KeyPairName"] != "cloud-forge-default" {
		t.Fatalf("KeyPairName = %q", gotInput.Parameters["KeyPairName"])
	}
	if !strings.Contains(stdout.String(), "Using VPC vpc-auto") {
		t.Fatalf("expected discovery message, got: %s", stdout.String())
	}
}

func TestDeployAliyunUsesCatalogTemplateAndParameters(t *testing.T) {
	t.Setenv("CLOUD_FORGE_TELEMETRY", "0")
	indexPath := writeAliyunTestCatalog(t)

	var gotInput aliyundeploy.DeployInput
	oldNewAliyunDeployer := newAliyunDeployer
	newAliyunDeployer = func(ctx context.Context, cfg aliyundeploy.Config) (aliyunStackDeployer, error) {
		return fakeAliyunDeployer{input: &gotInput}, nil
	}
	t.Cleanup(func() {
		newAliyunDeployer = oldNewAliyunDeployer
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"deploy",
		"hello-nginx",
		"--cloud",
		"aliyun",
		"--region",
		"cn-hongkong",
		"--store-url",
		fileURL(indexPath),
		"--cache-dir",
		t.TempDir(),
		"--vpc-id",
		"vpc-test",
		"--vswitch-id",
		"vsw-test",
		"--key",
		"my-key",
		"--dry-run",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
	}
	if gotInput.StackName != "cloud-forge-hello-nginx" {
		t.Fatalf("unexpected stack name %q", gotInput.StackName)
	}
	if gotInput.Parameters["VpcId"] != "vpc-test" {
		t.Fatalf("unexpected VpcId %q", gotInput.Parameters["VpcId"])
	}
	if gotInput.Parameters["VSwitchId"] != "vsw-test" {
		t.Fatalf("unexpected VSwitchId %q", gotInput.Parameters["VSwitchId"])
	}
	if gotInput.Parameters["KeyPairName"] != "my-key" {
		t.Fatalf("unexpected KeyPairName %q", gotInput.Parameters["KeyPairName"])
	}
	if !gotInput.DryRun {
		t.Fatal("expected dry-run deploy")
	}
}

func TestDeployAliyunManualDomainDNSSkipsReadyWaitByDefault(t *testing.T) {
	t.Setenv("CLOUD_FORGE_TELEMETRY", "0")
	indexPath := writeAliyunTestCatalog(t)

	var gotInput aliyundeploy.DeployInput
	oldNewAliyunDeployer := newAliyunDeployer
	newAliyunDeployer = func(ctx context.Context, cfg aliyundeploy.Config) (aliyunStackDeployer, error) {
		return fakeAliyunDeployer{
			input: &gotInput,
			output: &aliyundeploy.DeployOutput{
				Action:    "created",
				Region:    "cn-hongkong",
				AccountID: "123456789012",
				StackName: "cloud-forge-hello-nginx",
				Status:    "CREATE_COMPLETE",
				Outputs: map[string]string{
					"DomainName": "git.example.com",
					"PublicIP":   "203.0.113.10",
					"ServiceURL": "http://127.0.0.1:1",
				},
			},
		}, nil
	}
	t.Cleanup(func() {
		newAliyunDeployer = oldNewAliyunDeployer
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"deploy",
		"hello-nginx",
		"--cloud",
		"aliyun",
		"--store-url",
		fileURL(indexPath),
		"--cache-dir",
		t.TempDir(),
		"--vpc-id",
		"vpc-test",
		"--vswitch-id",
		"vsw-test",
		"--key",
		"my-key",
		"--domain",
		"git.example.com",
		"--timeout",
		"50ms",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s stdout: %s", code, stderr.String(), stdout.String())
	}
	if !gotInput.Wait {
		t.Fatal("expected stack wait to remain enabled")
	}
	if !strings.Contains(stdout.String(), "skipped the default endpoint wait") {
		t.Fatalf("expected manual DNS wait skip note, got: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "Service is ready.") {
		t.Fatalf("did not expect service ready wait, got: %s", stdout.String())
	}
}

func TestDeployAliyunWaitsForReadyByDefault(t *testing.T) {
	t.Setenv("CLOUD_FORGE_TELEMETRY", "0")
	indexPath := writeAliyunTestCatalog(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	oldNewAliyunDeployer := newAliyunDeployer
	newAliyunDeployer = func(ctx context.Context, cfg aliyundeploy.Config) (aliyunStackDeployer, error) {
		return fakeAliyunDeployer{
			output: &aliyundeploy.DeployOutput{
				Action:    "created",
				Region:    "cn-hongkong",
				AccountID: "123456789012",
				StackName: "cloud-forge-hello-nginx",
				Status:    "CREATE_COMPLETE",
				Outputs:   map[string]string{"ServiceURL": server.URL},
			},
		}, nil
	}
	t.Cleanup(func() { newAliyunDeployer = oldNewAliyunDeployer })

	oldInterval := bootstrapPollInterval
	bootstrapPollInterval = 10 * time.Millisecond
	t.Cleanup(func() { bootstrapPollInterval = oldInterval })

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"deploy",
		"hello-nginx",
		"--cloud",
		"aliyun",
		"--region",
		"cn-hongkong",
		"--store-url",
		fileURL(indexPath),
		"--cache-dir",
		t.TempDir(),
		"--vpc-id",
		"vpc-test",
		"--vswitch-id",
		"vsw-test",
		"--key",
		"my-key",
		"--timeout",
		"30s",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Service is ready.") {
		t.Fatalf("expected ready message, got: %s", stdout.String())
	}
}

func TestDeployAliyunNoWaitReadySkipsProbe(t *testing.T) {
	t.Setenv("CLOUD_FORGE_TELEMETRY", "0")
	indexPath := writeAliyunTestCatalog(t)

	oldNewAliyunDeployer := newAliyunDeployer
	newAliyunDeployer = func(ctx context.Context, cfg aliyundeploy.Config) (aliyunStackDeployer, error) {
		return fakeAliyunDeployer{
			output: &aliyundeploy.DeployOutput{
				Action:    "created",
				Region:    "cn-hongkong",
				AccountID: "123456789012",
				StackName: "cloud-forge-hello-nginx",
				Status:    "CREATE_COMPLETE",
				Outputs:   map[string]string{"ServiceURL": "https://127.0.0.1:1"},
			},
		}, nil
	}
	t.Cleanup(func() { newAliyunDeployer = oldNewAliyunDeployer })

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"deploy",
		"hello-nginx",
		"--cloud",
		"aliyun",
		"--region",
		"cn-hongkong",
		"--store-url",
		fileURL(indexPath),
		"--cache-dir",
		t.TempDir(),
		"--vpc-id",
		"vpc-test",
		"--vswitch-id",
		"vsw-test",
		"--key",
		"my-key",
		"--no-wait-ready",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "Service is ready.") {
		t.Fatalf("did not expect bootstrap wait, got: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "app bootstrap may still take") {
		t.Fatalf("expected bootstrap note, got: %s", stdout.String())
	}
}

func TestDeleteAliyunStack(t *testing.T) {
	t.Setenv("CLOUD_FORGE_TELEMETRY", "0")

	var gotInput aliyundeploy.DestroyInput
	oldNewAliyunDeployer := newAliyunDeployer
	newAliyunDeployer = func(ctx context.Context, cfg aliyundeploy.Config) (aliyunStackDeployer, error) {
		return fakeAliyunDeployer{destroyInput: &gotInput}, nil
	}
	t.Cleanup(func() {
		newAliyunDeployer = oldNewAliyunDeployer
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"delete",
		"cloud-forge-hello-nginx",
		"--cloud",
		"aliyun",
		"--region",
		"cn-hongkong",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
	}
	if gotInput.StackName != "cloud-forge-hello-nginx" {
		t.Fatalf("unexpected stack name %q", gotInput.StackName)
	}
	if !strings.Contains(stdout.String(), "deleted") {
		t.Fatalf("expected delete output, got: %s", stdout.String())
	}
}

func TestDeleteAWSStack(t *testing.T) {
	t.Setenv("CLOUD_FORGE_TELEMETRY", "0")

	var gotInput awsdeploy.DestroyInput
	oldNewAWSDeployer := newAWSDeployer
	newAWSDeployer = func(ctx context.Context, cfg awsdeploy.Config) (awsStackDeployer, error) {
		return fakeAWSDeployer{input: &awsdeploy.DeployInput{}, destroyInput: &gotInput}, nil
	}
	t.Cleanup(func() {
		newAWSDeployer = oldNewAWSDeployer
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"delete",
		"cloud-forge-gitea",
		"--cloud",
		"aws",
		"--region",
		"us-east-1",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
	}
	if gotInput.StackName != "cloud-forge-gitea" {
		t.Fatalf("unexpected stack name %q", gotInput.StackName)
	}
	if !gotInput.Wait {
		t.Fatal("expected delete to wait by default")
	}
	if !strings.Contains(stdout.String(), "DELETE_COMPLETE") {
		t.Fatalf("expected delete output, got: %s", stdout.String())
	}
}

func TestHelpDeploy(t *testing.T) {
	var stdout bytes.Buffer
	code := Run(context.Background(), []string{"help", "deploy"}, &stdout, &stderrBuffer{})
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if !strings.Contains(stdout.String(), "--dry-run") {
		t.Fatalf("expected deploy help, got: %s", stdout.String())
	}
}

type stderrBuffer struct{}

func (stderrBuffer) Write(p []byte) (int, error) { return len(p), nil }

type fakeAWSDeployer struct {
	input          *awsdeploy.DeployInput
	destroyInput   *awsdeploy.DestroyInput
	progressEvents []awsdeploy.StackProgressEvent
	output         *awsdeploy.DeployOutput
}

func (f fakeAWSDeployer) Deploy(ctx context.Context, input awsdeploy.DeployInput) (*awsdeploy.DeployOutput, error) {
	if f.input != nil {
		*f.input = input
	}
	for _, event := range f.progressEvents {
		if input.Progress != nil {
			input.Progress(event)
		}
	}
	if f.output != nil {
		return f.output, nil
	}
	return &awsdeploy.DeployOutput{
		Action:    "validated",
		Region:    "us-east-1",
		AccountID: "123456789012",
		StackName: input.StackName,
		Outputs:   map[string]string{"ServiceURL": "https://example.test"},
	}, nil
}

func (f fakeAWSDeployer) Destroy(ctx context.Context, input awsdeploy.DestroyInput) (*awsdeploy.DestroyOutput, error) {
	if f.destroyInput != nil {
		*f.destroyInput = input
	}
	for _, event := range f.progressEvents {
		if input.Progress != nil {
			input.Progress(event)
		}
	}
	return &awsdeploy.DestroyOutput{
		Action:    "deleted",
		Region:    "us-east-1",
		AccountID: "123456789012",
		StackName: input.StackName,
		Status:    "DELETE_COMPLETE",
	}, nil
}

type fakeAliyunDeployer struct {
	input        *aliyundeploy.DeployInput
	destroyInput *aliyundeploy.DestroyInput
	output       *aliyundeploy.DeployOutput
}

func (f fakeAliyunDeployer) Deploy(ctx context.Context, input aliyundeploy.DeployInput) (*aliyundeploy.DeployOutput, error) {
	if f.input != nil {
		*f.input = input
	}
	if f.output != nil {
		return f.output, nil
	}
	return &aliyundeploy.DeployOutput{
		Action:    "validated",
		Region:    "cn-hongkong",
		AccountID: "123456789012",
		StackName: input.StackName,
		Outputs:   map[string]string{"ServiceURL": "https://example.test"},
	}, nil
}

func (f fakeAliyunDeployer) Destroy(ctx context.Context, input aliyundeploy.DestroyInput) (*aliyundeploy.DestroyOutput, error) {
	if f.destroyInput != nil {
		*f.destroyInput = input
	}
	return &aliyundeploy.DestroyOutput{
		Action:    "deleted",
		Region:    "cn-hongkong",
		AccountID: "123456789012",
		StackName: input.StackName,
		Status:    "DELETE_COMPLETE",
	}, nil
}

func writeAliyunTestCatalog(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "apps", "hello-nginx", "templates", "aliyun.json"), `{"Resources": {}}`)
	indexPath := filepath.Join(root, "index", "apps.json")
	writeFile(t, indexPath, `{
  "catalog_version": "1.0.0",
  "generated_at": "2026-06-30T00:00:00Z",
  "base_url": "https://example.invalid/catalog",
  "store": {"name": "Test Store", "description": "test"},
  "apps": [{
    "id": "hello-nginx",
    "name": "Hello NGINX",
    "desc": "demo",
    "category": "other",
    "tags": ["demo"],
    "clouds": ["aliyun"],
    "version": "1.0.0",
    "price": "free",
    "images": {"aliyun": "aliyun_3_x64_20G_alibase_20260122.vhd"},
    "templates": {
      "aliyun": {
        "path": "apps/hello-nginx/templates/aliyun.json",
        "url": "https://example.invalid/remote.json"
      }
    },
    "params": {
      "InstanceType": {
        "type": "string",
        "aliyun": {
          "default": "ecs.c6.large",
          "options": ["ecs.c6.large"]
        }
      },
      "ImageId": {
        "type": "string",
        "aliyun": {"default": "aliyun_3_x64_20G_alibase_20260122.vhd"}
      },
      "CaddyTlsMode": {
        "type": "string",
        "aliyun": {
          "default": "ip-letsencrypt",
          "options": ["ip-letsencrypt", "http"]
        }
      },
      "KeyPairName": {"type": "string", "aliyun": {"required": true}},
      "VpcId": {"type": "string", "aliyun": {"required": true}},
      "VSwitchId": {"type": "string", "aliyun": {"required": true}},
      "AllowedIP": {"type": "string", "default": "0.0.0.0/0"}
    }
  }]
}`)
	return indexPath
}

func writeTestCatalog(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "apps", "gitea", "templates", "aws.yaml"), "Resources: {}\n")
	indexPath := filepath.Join(root, "index", "apps.json")
	writeFile(t, indexPath, `{
  "catalog_version": "1.0.0",
  "generated_at": "2026-06-30T00:00:00Z",
  "base_url": "https://example.invalid/catalog",
  "store": {"name": "Test Store", "description": "test"},
  "apps": [{
    "id": "gitea",
    "name": "Gitea",
    "desc": "Git hosting",
    "category": "devtools",
    "tags": ["git"],
    "clouds": ["aws"],
    "version": "1.0.0",
    "price": "free",
    "cost_notice": [
      "This deploy has base costs.",
      "Cloud Forge runtime AMI fee applies. AWS resources such as EC2, EBS, public IPv4/Elastic IP, and data transfer are billed separately by AWS."
    ],
    "images": {"aws": "ami-0123456789abcdef0"},
    "templates": {
      "aws": {
        "path": "apps/gitea/templates/aws.yaml",
        "url": "https://example.invalid/remote.yaml"
      }
    },
    "params": {
      "InstanceType": {
        "type": "string",
        "aws": {
          "default": "t3.micro",
          "options": ["t3.micro", "t3.small"]
        }
      },
      "KeyName": {
        "type": "string",
        "aws": {"default": ""}
      },
      "AllowedIP": {
        "type": "string",
        "default": "0.0.0.0/0"
      }
    }
  }]
}`)
	return indexPath
}

func writeRequiredParamCatalog(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "apps", "gitea", "templates", "aws.yaml"), "Resources: {}\n")
	indexPath := filepath.Join(root, "index", "apps.json")
	writeFile(t, indexPath, `{
  "catalog_version": "1.0.0",
  "generated_at": "2026-06-30T00:00:00Z",
  "base_url": "https://example.invalid/catalog",
  "store": {"name": "Test Store", "description": "test"},
  "apps": [{
    "id": "gitea",
    "name": "Gitea",
    "desc": "Git hosting",
    "category": "devtools",
    "tags": ["git"],
    "clouds": ["aws"],
    "version": "1.0.0",
    "images": {"aws": "ami-0123456789abcdef0"},
    "templates": {
      "aws": {
        "path": "apps/gitea/templates/aws.yaml",
        "url": "https://example.invalid/remote.yaml"
      }
    },
    "params": {
      "RequiredToken": {
        "type": "string",
        "aws": {"required": true}
      }
    }
  }]
}`)
	return indexPath
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fileURL(path string) string {
	return (&url.URL{Scheme: "file", Path: path}).String()
}
