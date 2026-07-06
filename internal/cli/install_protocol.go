package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func runInstallProtocol(args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("install-protocol", stderr)
	positionals, err := parseInterspersed(flags, args)
	if err != nil {
		return 2
	}
	if len(positionals) != 0 {
		fmt.Fprintln(stderr, "usage: cloud-forge install-protocol")
		return 2
	}

	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(stderr, "resolve cloud-forge executable: %v\n", err)
		return 1
	}
	exe, _ = filepath.Abs(exe)

	switch runtime.GOOS {
	case "darwin":
		if err := installProtocolDarwin(exe); err != nil {
			fmt.Fprintf(stderr, "install macOS URL handler: %v\n", err)
			return 1
		}
	case "linux":
		if err := installProtocolLinux(exe); err != nil {
			fmt.Fprintf(stderr, "install Linux URL handler: %v\n", err)
			return 1
		}
	case "windows":
		if err := installProtocolWindows(exe); err != nil {
			fmt.Fprintf(stderr, "install Windows URL handler: %v\n", err)
			return 1
		}
	default:
		fmt.Fprintf(stderr, "install-protocol is not supported on %s\n", runtime.GOOS)
		return 1
	}

	fmt.Fprintln(stdout, "Registered cloud-forge:// URL handler.")
	fmt.Fprintln(stdout, "You can now open Cloud Forge launch links from the browser.")
	return 0
}

func installProtocolDarwin(exe string) error {
	protocolDir, err := cloudForgeProtocolDir()
	if err != nil {
		return err
	}
	appPath := filepath.Join(protocolDir, "Cloud Forge URL Handler.app")
	scriptPath := filepath.Join(protocolDir, "handler.applescript")
	if err := os.MkdirAll(protocolDir, 0o755); err != nil {
		return err
	}
	if err := os.RemoveAll(appPath); err != nil {
		return err
	}

	script := darwinProtocolScript(exe)
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		return err
	}
	if err := exec.Command("osacompile", "-o", appPath, scriptPath).Run(); err != nil {
		return fmt.Errorf("compile AppleScript handler: %w", err)
	}

	plist := filepath.Join(appPath, "Contents", "Info.plist")
	_ = exec.Command(plistBuddyPath, "-c", "Delete :CFBundleURLTypes", plist).Run()
	if err := plistSetString(plist, "CFBundleIdentifier", "com.corenovalabs.cloud-forge.url-handler"); err != nil {
		return err
	}
	if err := plistSetString(plist, "CFBundleName", "Cloud Forge URL Handler"); err != nil {
		return err
	}
	commands := []string{
		"Add :CFBundleURLTypes array",
		"Add :CFBundleURLTypes:0 dict",
		"Add :CFBundleURLTypes:0:CFBundleURLName string Cloud Forge Launcher",
		"Add :CFBundleURLTypes:0:CFBundleURLSchemes array",
		"Add :CFBundleURLTypes:0:CFBundleURLSchemes:0 string cloud-forge",
	}
	for _, command := range commands {
		if err := exec.Command(plistBuddyPath, "-c", command, plist).Run(); err != nil {
			return fmt.Errorf("update Info.plist %q: %w", command, err)
		}
	}

	lsregister := "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
	_ = exec.Command(lsregister, "-f", appPath).Run()
	return nil
}

func installProtocolLinux(exe string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	appDir := filepath.Join(home, ".local", "share", "applications")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return err
	}
	desktopPath := filepath.Join(appDir, "cloud-forge-url-handler.desktop")
	content := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=Cloud Forge URL Handler
Exec=%s launch-url %%u
Terminal=true
NoDisplay=true
MimeType=x-scheme-handler/cloud-forge;
`, desktopExecQuote(exe))
	if err := os.WriteFile(desktopPath, []byte(content), 0o644); err != nil {
		return err
	}
	if err := exec.Command("xdg-mime", "default", filepath.Base(desktopPath), "x-scheme-handler/cloud-forge").Run(); err != nil {
		return fmt.Errorf("register xdg-mime handler: %w", err)
	}
	return nil
}

func installProtocolWindows(exe string) error {
	command := `"` + exe + `" launch-url "%1"`
	commands := [][]string{
		{"add", `HKCU\Software\Classes\cloud-forge`, "/ve", "/d", "URL:Cloud Forge Protocol", "/f"},
		{"add", `HKCU\Software\Classes\cloud-forge`, "/v", "URL Protocol", "/d", "", "/f"},
		{"add", `HKCU\Software\Classes\cloud-forge\shell\open\command`, "/ve", "/d", command, "/f"},
	}
	for _, args := range commands {
		if err := exec.Command("reg", args...).Run(); err != nil {
			return fmt.Errorf("reg %s: %w", strings.Join(args, " "), err)
		}
	}
	return nil
}

func cloudForgeProtocolDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cloud-forge", "protocol"), nil
}

func darwinProtocolScript(exe string) string {
	return fmt.Sprintf(`on open location this_URL
  set cliPath to %s
  set launchDir to (POSIX path of (path to temporary items)) & "cloud-forge-launches"
  do shell script "mkdir -p " & quoted form of launchDir
  set scriptPath to launchDir & "/cloud-forge-" & (do shell script "uuidgen") & ".command"
  set commandText to "#!/bin/zsh" & linefeed & "exec " & quoted form of cliPath & " launch-url " & quoted form of this_URL & linefeed
  set fileRef to open for access POSIX file scriptPath with write permission
  try
    set eof fileRef to 0
    write commandText to fileRef
  on error errMsg number errNum
    try
      close access fileRef
    end try
    error errMsg number errNum
  end try
  close access fileRef
  do shell script "chmod +x " & quoted form of scriptPath & " && open -a Terminal " & quoted form of scriptPath
end open location
`, appleScriptString(exe))
}

const plistBuddyPath = "/usr/libexec/PlistBuddy"

func plistSetString(plist, key, value string) error {
	if err := exec.Command(plistBuddyPath, "-c", fmt.Sprintf("Set :%s %s", key, value), plist).Run(); err == nil {
		return nil
	}
	if err := exec.Command(plistBuddyPath, "-c", fmt.Sprintf("Add :%s string %s", key, value), plist).Run(); err != nil {
		return fmt.Errorf("update Info.plist %s: %w", key, err)
	}
	return nil
}

func appleScriptString(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}

func desktopExecQuote(value string) string {
	if !strings.ContainsAny(value, " \t\n\"'\\") {
		return value
	}
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}
