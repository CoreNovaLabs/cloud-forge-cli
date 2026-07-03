package awsauth

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials/ssocreds"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
)

func TestRunnerUsesAWSLoginForBrowserMethod(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	awsDir := filepath.Join(home, ".aws")
	if err := os.MkdirAll(awsDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(awsDir, "credentials"), []byte("[default]\naws_access_key_id = old\naws_secret_access_key = old-secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(awsDir, "config"), []byte("[default]\nregion = us-west-2\nsso_session = old\nsso_account_id = 123456789012\nsso_role_name = Admin\nlogin_session = default\n"), 0600); err != nil {
		t.Fatal(err)
	}

	var gotName string
	var gotArgs []string
	runner := Runner{
		Out: io.Discard,
		Err: io.Discard,
		RunCommand: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			gotName = name
			gotArgs = append([]string(nil), args...)
			return nil
		},
		Check: func(ctx context.Context, profile, region string) (*Identity, error) {
			return &Identity{
				Account: "123456789012",
				Arn:     "arn:aws:sts::123456789012:assumed-role/Admin/test",
				Profile: profile,
				Region:  region,
				Source:  "LoginCredentials",
			}, nil
		},
	}

	if err := runner.Run(t.Context(), Options{
		Method:    "browser",
		Profile:   "default",
		Region:    "us-east-1",
		NoBrowser: true,
	}); err != nil {
		t.Fatal(err)
	}

	if gotName != "aws" {
		t.Fatalf("expected aws command, got %q", gotName)
	}
	for _, expected := range []string{"login", "--profile", "default", "--region", "us-east-1", "--no-cli-pager", "--remote"} {
		if !containsString(gotArgs, expected) {
			t.Fatalf("missing arg %q from %v", expected, gotArgs)
		}
	}

	credentialsData, err := os.ReadFile(filepath.Join(awsDir, "credentials"))
	if err != nil {
		t.Fatal(err)
	}
	if got := sectionBody(string(credentialsData), "default"); strings.Contains(got, "aws_access_key_id") ||
		strings.Contains(got, "aws_secret_access_key") {
		t.Fatalf("static credentials were not removed after browser login:\n%s", got)
	}

	configData, err := os.ReadFile(filepath.Join(awsDir, "config"))
	if err != nil {
		t.Fatal(err)
	}
	profile := sectionBody(string(configData), "default")
	if !strings.Contains(profile, "login_session = default") {
		t.Fatalf("login_session was not preserved:\n%s", profile)
	}
	for _, removed := range []string{"sso_session", "sso_account_id", "sso_role_name"} {
		if strings.Contains(profile, removed) {
			t.Fatalf("%s was not removed after browser login:\n%s", removed, profile)
		}
	}
}

func TestSetINIValuesUpdatesExistingSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte("[default]\nregion = us-west-2\noutput = json\n\n[profile dev]\nregion = eu-west-1\n"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := setINIValues(path, "default", map[string]string{
		"region": "us-east-1",
	}, 0600); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "[default]\nregion = us-east-1\noutput = json") {
		t.Fatalf("default section was not updated correctly:\n%s", content)
	}
	if !strings.Contains(content, "[profile dev]\nregion = eu-west-1") {
		t.Fatalf("unrelated section was modified:\n%s", content)
	}
}

func TestWriteStaticCredentialsUsesAWSStandardFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := filepath.Join(home, ".aws", "config")
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("[default]\nregion = us-west-2\nsso_session = old\nsso_account_id = 123456789012\nsso_role_name = Admin\n"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := writeStaticCredentials("default", "us-east-1", "AKIATEST", "secret"); err != nil {
		t.Fatal(err)
	}

	credentialsData, err := os.ReadFile(filepath.Join(home, ".aws", "credentials"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(credentialsData); !strings.Contains(got, "[default]") ||
		!strings.Contains(got, "aws_access_key_id = AKIATEST") ||
		!strings.Contains(got, "aws_secret_access_key = secret") {
		t.Fatalf("unexpected credentials file:\n%s", got)
	}

	configData, err := os.ReadFile(filepath.Join(home, ".aws", "config"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(configData); !strings.Contains(got, "[default]") ||
		!strings.Contains(got, "region = us-east-1") {
		t.Fatalf("unexpected config file:\n%s", got)
	}
	if got := sectionBody(string(configData), "default"); strings.Contains(got, "sso_session") ||
		strings.Contains(got, "sso_account_id") ||
		strings.Contains(got, "sso_role_name") {
		t.Fatalf("SSO settings were not removed from static credentials profile:\n%s", got)
	}
}

func TestWriteSSOProfileRemovesStaticCredentials(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	awsDir := filepath.Join(home, ".aws")
	if err := os.MkdirAll(awsDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(awsDir, "credentials"), []byte("[default]\naws_access_key_id = old\naws_secret_access_key = old-secret\naws_session_token = old-token\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(awsDir, "config"), []byte("[default]\nregion = us-west-2\nsso_start_url = https://old.awsapps.com/start\nsso_region = us-west-2\n"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := writeSSOProfile("default", "us-east-1", "cloud-forge", "https://d-example.awsapps.com/start", "us-east-1", "123456789012", "AdministratorAccess"); err != nil {
		t.Fatal(err)
	}

	credentialsData, err := os.ReadFile(filepath.Join(awsDir, "credentials"))
	if err != nil {
		t.Fatal(err)
	}
	if got := sectionBody(string(credentialsData), "default"); strings.Contains(got, "aws_access_key_id") ||
		strings.Contains(got, "aws_secret_access_key") ||
		strings.Contains(got, "aws_session_token") {
		t.Fatalf("static credentials were not removed from SSO profile:\n%s", got)
	}

	configData, err := os.ReadFile(filepath.Join(awsDir, "config"))
	if err != nil {
		t.Fatal(err)
	}
	profile := sectionBody(string(configData), "default")
	for _, expected := range []string{
		"region = us-east-1",
		"sso_session = cloud-forge",
		"sso_account_id = 123456789012",
		"sso_role_name = AdministratorAccess",
	} {
		if !strings.Contains(profile, expected) {
			t.Fatalf("missing %q from SSO profile:\n%s", expected, profile)
		}
	}
	if strings.Contains(profile, "sso_start_url") || strings.Contains(profile, "sso_region") {
		t.Fatalf("legacy SSO settings were not removed from profile section:\n%s", profile)
	}
	session := sectionBody(string(configData), "sso-session cloud-forge")
	if !strings.Contains(session, "sso_start_url = https://d-example.awsapps.com/start") ||
		!strings.Contains(session, "sso_region = us-east-1") {
		t.Fatalf("unexpected sso-session section:\n%s", session)
	}
}

func TestWriteSSOTokenUsesSDKCachePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	now := time.Unix(1783033600, 0).UTC()

	err := writeSSOToken("cloud-forge", "https://d-example.awsapps.com/start", "us-east-1", &ssooidc.RegisterClientOutput{
		ClientId:              aws.String("client-id"),
		ClientSecret:          aws.String("client-secret"),
		ClientSecretExpiresAt: now.Add(time.Hour).Unix(),
	}, &ssooidc.CreateTokenOutput{
		AccessToken:  aws.String("access-token"),
		RefreshToken: aws.String("refresh-token"),
		ExpiresIn:    600,
	}, func() time.Time {
		return now
	})
	if err != nil {
		t.Fatal(err)
	}

	cachePath, err := ssocreds.StandardCachedTokenFilepath("cloud-forge")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, expected := range []string{
		`"accessToken": "access-token"`,
		`"refreshToken": "refresh-token"`,
		`"clientId": "client-id"`,
		`"clientSecret": "client-secret"`,
		`"startUrl": "https://d-example.awsapps.com/start"`,
		`"region": "us-east-1"`,
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("missing %s in token cache:\n%s", expected, content)
		}
	}
}

func sectionBody(content, section string) string {
	header := "[" + section + "]"
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	inSection := false
	var out []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			if inSection {
				break
			}
			inSection = trimmed == header
			continue
		}
		if inSection {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
