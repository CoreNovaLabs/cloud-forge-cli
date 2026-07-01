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
