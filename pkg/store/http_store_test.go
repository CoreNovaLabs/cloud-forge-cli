package store_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloud-forge/cli/pkg/store"
)

func TestHTTPStore_LocalFileCatalog(t *testing.T) {
	catalogRoot := t.TempDir()
	indexPath := filepath.Join(catalogRoot, "index", "apps.json")
	templatePath := filepath.Join(catalogRoot, "apps", "gitea", "templates", "aws.yaml")

	writeFile(t, templatePath, "AWSTemplateFormatVersion: '2010-09-09'\nResources: {}\n")
	writeFile(t, indexPath, `{
  "catalog_version": "1.0.0",
  "generated_at": "2026-06-30T00:00:00Z",
  "base_url": "https://example.invalid/cloud-forge-catalog",
  "store": {
    "name": "Cloud Forge App Store",
    "description": "test catalog"
  },
  "apps": [
    {
      "id": "gitea",
      "name": "Gitea",
      "desc": "Git hosting",
      "category": "devtools",
      "tags": ["git", "self-hosted"],
      "clouds": ["aws"],
      "version": "1.0.0",
      "images": {"aws": "ami-0123456789abcdef0"},
      "templates": {
        "aws": {
          "path": "apps/gitea/templates/aws.yaml",
          "url": "https://example.invalid/should-not-be-used.yaml"
        }
      }
    }
  ]
}`)

	s := store.NewHTTPStore(store.Config{
		IndexURL: fileURL(indexPath),
		CacheDir: filepath.Join(t.TempDir(), "cache"),
		CacheTTL: time.Minute,
	})

	if err := s.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}

	apps, err := s.List(store.Filter{Query: "gitea"})
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 1 {
		t.Fatalf("expected 1 app, got %d", len(apps))
	}

	body, err := s.GetTemplate(context.Background(), "gitea", "aws")
	if err != nil {
		t.Fatal(err)
	}
	if body == "" {
		t.Fatal("expected non-empty template")
	}
	if body != "AWSTemplateFormatVersion: '2010-09-09'\nResources: {}\n" {
		t.Fatalf("unexpected template body: %q", body)
	}

	byCategory, err := s.List(store.Filter{Category: "devtools", Cloud: "aws"})
	if err != nil {
		t.Fatal(err)
	}
	if len(byCategory) < 1 {
		t.Fatalf("expected devtools on aws, got %d", len(byCategory))
	}
}

func TestHTTPStore_IndexFallbackURL(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "primary unavailable", http.StatusBadGateway)
	}))
	defer primary.Close()

	var fallback *httptest.Server
	fallback = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index/apps.json":
			fmt.Fprintf(w, `{
  "catalog_version": "1.0.0",
  "generated_at": "2026-06-30T00:00:00Z",
  "base_url": "%s",
  "store": {"name": "Test Store", "description": "test"},
  "apps": [{
    "id": "gitea",
    "name": "Gitea",
    "desc": "Git hosting",
    "category": "devtools",
    "clouds": ["aws"],
    "version": "1.0.0",
    "images": {"aws": "ami-0123456789abcdef0"},
    "templates": {"aws": {"path": "apps/gitea/templates/aws.yaml"}}
  }]
}`, fallback.URL)
		case "/apps/gitea/templates/aws.yaml":
			fmt.Fprintln(w, "Resources: {}")
		default:
			http.NotFound(w, r)
		}
	}))
	defer fallback.Close()

	s := store.NewHTTPStore(store.Config{
		IndexURL:          primary.URL + "/index/apps.json",
		IndexFallbackURLs: []string{fallback.URL + "/index/apps.json"},
		CacheDir:          filepath.Join(t.TempDir(), "cache"),
		CacheTTL:          time.Minute,
	})

	if err := s.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}

	body, err := s.GetTemplate(context.Background(), "gitea", "aws")
	if err != nil {
		t.Fatal(err)
	}
	if body != "Resources: {}\n" {
		t.Fatalf("unexpected template body: %q", body)
	}
}

func TestHTTPStore_TemplateBaseURLFallback(t *testing.T) {
	templatePrimary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "template unavailable", http.StatusBadGateway)
	}))
	defer templatePrimary.Close()

	templateFallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/apps/gitea/templates/aws.yaml" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintln(w, "Resources: {}")
	}))
	defer templateFallback.Close()

	index := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/apps/gitea/templates/aws.yaml" {
			http.Error(w, "index host template unavailable", http.StatusBadGateway)
			return
		}
		fmt.Fprintf(w, `{
  "catalog_version": "1.0.0",
  "generated_at": "2026-06-30T00:00:00Z",
  "base_url": "https://example.invalid/catalog",
  "store": {"name": "Test Store", "description": "test"},
  "apps": [{
    "id": "gitea",
    "name": "Gitea",
    "desc": "Git hosting",
    "category": "devtools",
    "clouds": ["aws"],
    "version": "1.0.0",
    "images": {"aws": "ami-0123456789abcdef0"},
    "templates": {"aws": {"path": "apps/gitea/templates/aws.yaml"}}
  }]
}`)
	}))
	defer index.Close()

	s := store.NewHTTPStore(store.Config{
		IndexURL:         index.URL + "/index/apps.json",
		TemplateBaseURLs: []string{templatePrimary.URL, templateFallback.URL},
		CacheDir:         filepath.Join(t.TempDir(), "cache"),
		CacheTTL:         time.Minute,
	})

	if err := s.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}

	body, err := s.GetTemplate(context.Background(), "gitea", "aws")
	if err != nil {
		t.Fatal(err)
	}
	if body != "Resources: {}\n" {
		t.Fatalf("unexpected template body: %q", body)
	}
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
