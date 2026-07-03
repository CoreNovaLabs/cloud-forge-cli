package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	if !strings.Contains(stderr.String(), "missing required AWS parameter(s): RequiredToken") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
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
}

func (f fakeAWSDeployer) Deploy(ctx context.Context, input awsdeploy.DeployInput) (*awsdeploy.DeployOutput, error) {
	*f.input = input
	for _, event := range f.progressEvents {
		if input.Progress != nil {
			input.Progress(event)
		}
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
