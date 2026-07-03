package awsauth

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials/logincreds"
	signintypes "github.com/aws/aws-sdk-go-v2/service/signin/types"
)

func TestBuildAuthorizationURL(t *testing.T) {
	got := buildAuthorizationURL("https://signin.us-east-1.amazonaws.com", sameDeviceClientID, "state-1", "challenge-1", "http://127.0.0.1:12345/oauth/callback")
	if !strings.HasPrefix(got, "https://signin.us-east-1.amazonaws.com/v1/authorize?") {
		t.Fatalf("unexpected authorization URL: %s", got)
	}
	for _, expected := range []string{
		"response_type=code",
		"client_id=arn%3Aaws%3Asignin%3A%3A%3Adevtools%2Fsame-device",
		"state=state-1",
		"code_challenge_method=SHA-256",
		"scope=openid",
		"code_challenge=challenge-1",
		"redirect_uri=http%3A%2F%2F127.0.0.1%3A12345%2Foauth%2Fcallback",
	} {
		if !strings.Contains(got, expected) {
			t.Fatalf("missing %q from %s", expected, got)
		}
	}
}

func TestParseCrossDeviceVerificationCode(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("state=state-1&code=code-1"))
	code, state, err := parseCrossDeviceVerificationCode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if code != "code-1" || state != "state-1" {
		t.Fatalf("unexpected code/state %q %q", code, state)
	}
}

func TestLoginSessionFromIDToken(t *testing.T) {
	sessionARN := "arn:aws:sts::123456789012:assumed-role/Admin/test"
	got, err := loginSessionFromIDToken(fakeIDToken(sessionARN))
	if err != nil {
		t.Fatal(err)
	}
	if got != sessionARN {
		t.Fatalf("expected %q, got %q", sessionARN, got)
	}
}

func TestWriteLoginTokenUsesSDKCachePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sessionARN := "arn:aws:sts::123456789012:assumed-role/Admin/test"
	now := time.Unix(1783033600, 0).UTC()
	expiresIn := int32(900)
	key, err := generateP256Key()
	if err != nil {
		t.Fatal(err)
	}

	err = writeLoginToken(sessionARN, "123456789012", sameDeviceClientID, &signintypes.CreateOAuth2TokenResponseBody{
		AccessToken: &signintypes.AccessToken{
			AccessKeyId:     aws.String("ASIATEST"),
			SecretAccessKey: aws.String("secret"),
			SessionToken:    aws.String("session"),
		},
		ExpiresIn:    &expiresIn,
		RefreshToken: aws.String("refresh"),
		TokenType:    aws.String("aws_sigv4"),
		IdToken:      aws.String(fakeIDToken(sessionARN)),
	}, key, func() time.Time {
		return now
	})
	if err != nil {
		t.Fatal(err)
	}

	cachePath, err := logincreds.StandardCachedTokenFilepath(sessionARN, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(cachePath, filepath.Join(home, ".aws", "login", "cache")) {
		t.Fatalf("unexpected cache path %s", cachePath)
	}
	data, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, expected := range []string{
		`"accessKeyId": "ASIATEST"`,
		`"secretAccessKey": "secret"`,
		`"sessionToken": "session"`,
		`"accountId": "123456789012"`,
		`"clientId": "arn:aws:signin:::devtools/same-device"`,
		`"refreshToken": "refresh"`,
		`"dpopKey": "-----BEGIN EC PRIVATE KEY-----`,
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("missing %s from login token cache:\n%s", expected, content)
		}
	}
}

func fakeIDToken(sessionARN string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"` + sessionARN + `"}`))
	return header + "." + payload + ".signature"
}
