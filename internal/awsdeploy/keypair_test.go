package awsdeploy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsurePrivateKeyCreatesAndReusesRSAKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cloud-forge-default.pem")

	publicKey, created, err := ensurePrivateKey(path, DefaultKeyPairName)
	if err != nil {
		t.Fatalf("ensurePrivateKey create: %v", err)
	}
	if !created {
		t.Fatal("expected private key to be created")
	}
	if !strings.HasPrefix(publicKey, "ssh-rsa ") {
		t.Fatalf("unexpected public key format: %q", publicKey)
	}
	if !strings.Contains(publicKey, DefaultKeyPairName) {
		t.Fatalf("expected public key comment %q, got %q", DefaultKeyPairName, publicKey)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat private key: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("unexpected private key permissions %o", got)
	}

	reusedPublicKey, created, err := ensurePrivateKey(path, DefaultKeyPairName)
	if err != nil {
		t.Fatalf("ensurePrivateKey reuse: %v", err)
	}
	if created {
		t.Fatal("expected existing private key to be reused")
	}
	if reusedPublicKey != publicKey {
		t.Fatal("expected reused private key to produce the same public key")
	}
}
