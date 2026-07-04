package aliyunauth

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveCredentialsAndCheckIdentity(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	cfg := Config{
		Profile:         "default",
		Region:          DefaultRegion,
		AccessKeyID:     "AKTEST",
		AccessKeySecret: "SKTEST",
	}
	if err := SaveCredentials(cfg); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, ".cloud-forge", "aliyun", "credentials")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "AKTEST") {
		t.Fatalf("unexpected credentials file: %s", string(data))
	}

	runner := Runner{
		In:  strings.NewReader("\n"),
		Out: &bytes.Buffer{},
		Err: &bytes.Buffer{},
		Check: func(ctx context.Context, cfg Config) (*Identity, error) {
			return &Identity{Account: "123", Region: DefaultRegion, Profile: "default"}, nil
		},
	}
	if err := runner.Run(context.Background(), Options{StatusOnly: true}); err != nil {
		t.Fatalf("status: %v", err)
	}
}
