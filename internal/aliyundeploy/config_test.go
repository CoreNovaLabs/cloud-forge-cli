package aliyundeploy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")
	content := "[default]\naccess_key_id = test-id\naccess_key_secret = test-secret\nregion = cn-hongkong\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	origHome := os.Getenv("HOME")
	t.Setenv("HOME", dir)
	_ = origHome

	// CredentialsPath uses ~/.cloud-forge/aliyun/credentials, override by writing there.
	credDir := filepath.Join(dir, ".cloud-forge", "aliyun")
	if err := os.MkdirAll(credDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(credDir, "credentials"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(Config{})
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.AccessKeyID != "test-id" {
		t.Fatalf("AccessKeyID = %q", cfg.AccessKeyID)
	}
	if cfg.Region != SupportedRegion {
		t.Fatalf("Region = %q", cfg.Region)
	}
}

func TestLoadConfigAcceptsOtherRegion(t *testing.T) {
	cfg, err := LoadConfig(Config{
		Region:          "cn-hangzhou",
		AccessKeyID:     "id",
		AccessKeySecret: "secret",
	})
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Region != "cn-hangzhou" {
		t.Fatalf("Region = %q", cfg.Region)
	}
}

func TestMainlandChinaRegion(t *testing.T) {
	if !MainlandChinaRegion("cn-hangzhou") {
		t.Fatal("expected cn-hangzhou to be mainland")
	}
	if MainlandChinaRegion("cn-hongkong") {
		t.Fatal("cn-hongkong should not be mainland")
	}
	if MainlandChinaRegion("ap-southeast-1") {
		t.Fatal("ap-southeast-1 should not be mainland")
	}
}

func TestValidateInput(t *testing.T) {
	if err := validateInput(DeployInput{}); err == nil {
		t.Fatal("expected error for empty stack name")
	}
}
