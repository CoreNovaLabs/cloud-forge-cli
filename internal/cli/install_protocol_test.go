package cli

import (
	"strings"
	"testing"
)

func TestDarwinProtocolScriptOpensCommandFileInTerminal(t *testing.T) {
	script := darwinProtocolScript("/tmp/cloud forge/bin/cloud-forge")

	for _, want := range []string{
		"cloud-forge-launches",
		".command",
		"launch-url",
		"open -a Terminal",
		"quoted form of this_URL",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script missing %q:\n%s", want, script)
		}
	}
	if strings.Contains(script, `tell application "Terminal"`) {
		t.Fatalf("script should not require Terminal AppleEvents automation:\n%s", script)
	}
	if !strings.Contains(script, `"/tmp/cloud forge/bin/cloud-forge"`) {
		t.Fatalf("script did not preserve executable path:\n%s", script)
	}
}
