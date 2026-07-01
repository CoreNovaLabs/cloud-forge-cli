package store

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

func verifyChecksum(data []byte, expected string) error {
	parts := strings.SplitN(expected, ":", 2)
	if len(parts) != 2 || parts[0] != "sha256" {
		return fmt.Errorf("store: unsupported checksum format %q", expected)
	}

	sum := sha256.Sum256(data)
	actual := hex.EncodeToString(sum[:])
	if actual != parts[1] {
		return fmt.Errorf("store: checksum mismatch: want %s got %s", parts[1], actual)
	}
	return nil
}
