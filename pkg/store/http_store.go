package store

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// HTTPStore 从远程或本地 file:// URL 拉取 catalog，并缓存到本地。
type HTTPStore struct {
	cfg        Config
	catalog    *Catalog
	httpClient *http.Client
}

func NewHTTPStore(cfg Config) *HTTPStore {
	if cfg.CacheTTL == 0 {
		cfg.CacheTTL = 24 * time.Hour
	}
	if cfg.HTTPTimeout == 0 {
		cfg.HTTPTimeout = 30 * time.Second
	}
	return &HTTPStore{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: cfg.HTTPTimeout},
	}
}

func (s *HTTPStore) Sync(ctx context.Context) error {
	if s.cfg.IndexURL == "" {
		return fmt.Errorf("store: index URL is required")
	}

	cachePath := s.indexCachePath()
	if !s.isCacheStale(cachePath) {
		catalog, err := s.loadCatalogFromFile(cachePath)
		if err == nil {
			s.catalog = catalog
			return nil
		}
	}

	body, err := s.fetchIndex(ctx)
	if err != nil {
		return err
	}

	var catalog Catalog
	if err := json.Unmarshal(body, &catalog); err != nil {
		return fmt.Errorf("store: parse catalog: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(cachePath, body, 0o644); err != nil {
		return err
	}

	s.catalog = &catalog
	return nil
}

func (s *HTTPStore) List(filter Filter) ([]App, error) {
	if s.catalog == nil {
		return nil, fmt.Errorf("store: catalog not loaded, call Sync first")
	}

	var out []App
	for _, app := range s.catalog.Apps {
		if !matchFilter(app, filter) {
			continue
		}
		out = append(out, app)
	}
	return out, nil
}

func (s *HTTPStore) Get(appID string) (*App, error) {
	if s.catalog == nil {
		return nil, fmt.Errorf("store: catalog not loaded, call Sync first")
	}

	for i := range s.catalog.Apps {
		if s.catalog.Apps[i].ID == appID {
			app := s.catalog.Apps[i]
			return &app, nil
		}
	}
	return nil, fmt.Errorf("store: app %q not found", appID)
}

func (s *HTTPStore) GetTemplate(ctx context.Context, appID, cloud string) (string, error) {
	app, err := s.Get(appID)
	if err != nil {
		return "", err
	}

	ref, ok := app.Templates[cloud]
	if !ok {
		return "", fmt.Errorf("store: app %q has no template for cloud %q", appID, cloud)
	}

	cachePath := s.templateCachePath(appID, cloud)
	if data, err := os.ReadFile(cachePath); err == nil {
		if ref.Checksum == "" || verifyChecksum(data, ref.Checksum) == nil {
			return string(data), nil
		}
	}

	raw, err := s.fetchTemplate(ctx, ref)
	if err != nil {
		return "", err
	}

	if ref.Checksum != "" {
		if err := verifyChecksum(raw, ref.Checksum); err != nil {
			return "", err
		}
	}

	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(cachePath, raw, 0o644); err != nil {
		return "", err
	}

	return string(raw), nil
}

func (s *HTTPStore) fetchIndex(ctx context.Context) ([]byte, error) {
	path, ok, err := localPathFromURL(s.cfg.IndexURL)
	if err != nil {
		return nil, err
	}

	if ok {
		return os.ReadFile(path)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.IndexURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("store: fetch index %s: %w", s.cfg.IndexURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("store: fetch index %s: HTTP %d", s.cfg.IndexURL, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func (s *HTTPStore) fetchTemplate(ctx context.Context, ref TemplateRef) ([]byte, error) {
	if ref.Path != "" {
		if localRoot, ok := s.localCatalogRoot(); ok {
			localPath := filepath.Join(localRoot, filepath.FromSlash(ref.Path))
			if data, err := os.ReadFile(localPath); err == nil {
				return data, nil
			}
		}
	}

	templateURL := ref.URL
	if templateURL == "" && ref.Path != "" && s.catalog != nil {
		templateURL = strings.TrimRight(s.catalog.BaseURL, "/") + "/" + strings.TrimLeft(ref.Path, "/")
	}
	if templateURL == "" {
		return nil, fmt.Errorf("store: template URL is empty")
	}

	path, ok, err := localPathFromURL(templateURL)
	if err != nil {
		return nil, err
	}

	if ok {
		return os.ReadFile(path)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, templateURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("store: fetch template %s: %w", templateURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("store: fetch template %s: HTTP %d", templateURL, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func (s *HTTPStore) indexCachePath() string {
	dir := s.cfg.CacheDir
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".cloud-forge", "cache")
	}
	return filepath.Join(dir, "index", "apps.json")
}

func (s *HTTPStore) templateCachePath(appID, cloud string) string {
	dir := s.cfg.CacheDir
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".cloud-forge", "cache")
	}
	ext := "yaml"
	if cloud == "aliyun" {
		ext = "json"
	}
	return filepath.Join(dir, "templates", appID, cloud+"."+ext)
}

func (s *HTTPStore) isCacheStale(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return true
	}
	return time.Since(info.ModTime()) > s.cfg.CacheTTL
}

func (s *HTTPStore) loadCatalogFromFile(path string) (*Catalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var catalog Catalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil, err
	}
	return &catalog, nil
}

func (s *HTTPStore) localCatalogRoot() (string, bool) {
	indexPath, ok, err := localPathFromURL(s.cfg.IndexURL)
	if err != nil || !ok {
		return "", false
	}

	dir := filepath.Dir(indexPath)
	if filepath.Base(dir) == "index" {
		return filepath.Dir(dir), true
	}
	return dir, true
}

func localPathFromURL(raw string) (string, bool, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", false, err
	}

	if u.Scheme == "" {
		return raw, true, nil
	}
	if u.Scheme != "file" {
		return "", false, nil
	}
	if u.Host != "" && u.Host != "localhost" {
		return "", false, fmt.Errorf("store: unsupported file URL host %q", u.Host)
	}
	if u.Path == "" {
		return "", false, fmt.Errorf("store: empty file URL path")
	}
	return u.Path, true, nil
}

func matchFilter(app App, f Filter) bool {
	if f.Category != "" && app.Category != f.Category {
		return false
	}
	if f.Cloud != "" && !contains(app.Clouds, f.Cloud) {
		return false
	}
	for _, tag := range f.Tags {
		if !contains(app.Tags, tag) {
			return false
		}
	}
	if f.Query != "" && !matchQuery(app, f.Query) {
		return false
	}
	return true
}

func matchQuery(app App, q string) bool {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return true
	}
	haystack := strings.ToLower(strings.Join([]string{
		app.ID, app.Name, app.Desc, strings.Join(app.Tags, " "),
	}, " "))
	return strings.Contains(haystack, q)
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
