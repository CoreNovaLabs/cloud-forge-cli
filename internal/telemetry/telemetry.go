package telemetry

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	DefaultEndpoint = "https://telemetry.corenovacloud.com/v1/events"
	defaultTimeout  = 1500 * time.Millisecond
)

type Client struct {
	endpoint    string
	anonymousID string
	cliVersion  string
	httpClient  *http.Client
	disabled    bool
}

type Event struct {
	Event          string `json:"event"`
	AnonymousID    string `json:"anonymous_id"`
	CLIVersion     string `json:"cli_version"`
	CatalogVersion string `json:"catalog_version,omitempty"`
	AppID          string `json:"app_id,omitempty"`
	AppVersion     string `json:"app_version,omitempty"`
	Cloud          string `json:"cloud,omitempty"`
	OS             string `json:"os,omitempty"`
	Arch           string `json:"arch,omitempty"`
	Status         string `json:"status,omitempty"`
	DurationMS     int64  `json:"duration_ms,omitempty"`
	ErrorCode      string `json:"error_code,omitempty"`
	CreatedAt      string `json:"created_at,omitempty"`
}

type Config struct {
	CacheDir   string
	Endpoint   string
	CLIVersion string
}

func NewClient(cfg Config) *Client {
	if telemetryDisabled() {
		return &Client{disabled: true}
	}

	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}

	anonymousID, err := loadOrCreateAnonymousID(cfg.CacheDir)
	if err != nil {
		return &Client{disabled: true}
	}

	return &Client{
		endpoint:    endpoint,
		anonymousID: anonymousID,
		cliVersion:  cfg.CLIVersion,
		httpClient:  &http.Client{Timeout: defaultTimeout},
	}
}

func (c *Client) Track(ctx context.Context, event Event) {
	if c == nil || c.disabled {
		return
	}

	event.AnonymousID = c.anonymousID
	event.CLIVersion = c.cliVersion
	event.OS = runtime.GOOS
	event.Arch = runtime.GOARCH
	event.CreatedAt = time.Now().UTC().Format(time.RFC3339)

	go c.post(ctx, event)
}

func (c *Client) post(ctx context.Context, event Event) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), defaultTimeout)
	defer cancel()

	body, err := json.Marshal(event)
	if err != nil {
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("user-agent", "cloud-forge/"+c.cliVersion)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
}

func loadOrCreateAnonymousID(cacheDir string) (string, error) {
	path := filepath.Join(configDir(cacheDir), "telemetry_id")
	if data, err := os.ReadFile(path); err == nil {
		id := strings.TrimSpace(string(data))
		if id != "" {
			return id, nil
		}
	}

	id, err := newAnonymousID()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(id+"\n"), 0o600); err != nil {
		return "", err
	}
	return id, nil
}

func newAnonymousID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return "install_" + hex.EncodeToString(buf[:]), nil
}

func configDir(cacheDir string) string {
	if cacheDir != "" {
		return cacheDir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "cloud-forge")
	}
	return filepath.Join(home, ".cloud-forge")
}

func telemetryDisabled() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("CLOUD_FORGE_TELEMETRY")))
	return value == "0" || value == "false" || value == "off" || value == "disabled"
}

func EventStatus(err error) (status string, code string) {
	if err == nil {
		return "success", ""
	}
	return "failed", fmt.Sprintf("%T", err)
}
