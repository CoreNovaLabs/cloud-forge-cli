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

func TestWindowsProtocolCommandKeepsWindowOpen(t *testing.T) {
	command := windowsProtocolCommand(`C:\Program Files\Cloud Forge\cloud-forge.exe`)

	for _, want := range []string{
		"powershell.exe",
		"-NoExit",
		`'C:\Program Files\Cloud Forge\cloud-forge.exe'`,
		"launch-url $args[0]",
		`"%1"`,
	} {
		if !strings.Contains(command, want) {
			t.Fatalf("command missing %q: %s", want, command)
		}
	}
	if strings.Contains(command, "cmd.exe") || strings.Contains(command, `/k`) {
		t.Fatalf("command should avoid cmd.exe parsing: %s", command)
	}
}

func TestWindowsProtocolCommandEscapesPowerShellSingleQuotes(t *testing.T) {
	command := windowsProtocolCommand(`C:\Cloud Forge's CLI\cloud-forge.exe`)
	if !strings.Contains(command, `'C:\Cloud Forge''s CLI\cloud-forge.exe'`) {
		t.Fatalf("command did not escape PowerShell single quote: %s", command)
	}
}
