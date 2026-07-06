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
	command := windowsProtocolCommand(`C:\Users\me\.cloud-forge\protocol\launcher.ps1`)

	for _, want := range []string{
		"powershell.exe",
		"-NoExit",
		"-File",
		`"C:\Users\me\.cloud-forge\protocol\launcher.ps1"`,
		`"%1"`,
	} {
		if !strings.Contains(command, want) {
			t.Fatalf("command missing %q: %s", want, command)
		}
	}
	for _, bad := range []string{"cmd.exe", `/k`, "-Command", "$args[0]"} {
		if strings.Contains(command, bad) {
			t.Fatalf("command should avoid %q parsing: %s", bad, command)
		}
	}
}

func TestWindowsProtocolScriptPassesLaunchURLAsArgument(t *testing.T) {
	script := windowsProtocolScript(`C:\Cloud Forge's CLI\cloud-forge.exe`)

	for _, want := range []string{
		"param(",
		"[string]$LaunchUrl",
		`'C:\Cloud Forge''s CLI\cloud-forge.exe'`,
		"& $CloudForge launch-url $LaunchUrl",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script missing %q:\n%s", want, script)
		}
	}
	for _, bad := range []string{"$args[0]", "cloud-forge://", "&cloud="} {
		if strings.Contains(script, bad) {
			t.Fatalf("script should not inline URL parsing token %q:\n%s", bad, script)
		}
	}
}
