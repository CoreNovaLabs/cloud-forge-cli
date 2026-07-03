package awsauth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/logincreds"
	"github.com/aws/aws-sdk-go-v2/service/signin"
	signintypes "github.com/aws/aws-sdk-go-v2/service/signin/types"
	"github.com/aws/smithy-go/middleware"
	"github.com/aws/smithy-go/ptr"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

const (
	sameDeviceClientID  = "arn:aws:signin:::devtools/same-device"
	crossDeviceClientID = "arn:aws:signin:::devtools/cross-device"
	authCallbackPath    = "/oauth/callback"
	authTimeout         = 10 * time.Minute
)

type browserLoginOptions struct {
	Region    string
	NoBrowser bool
	In        io.Reader
	Out       io.Writer
	Err       io.Writer
	Open      func(string) error
	Now       func() time.Time
}

type browserLoginResult struct {
	SessionARN string
}

type signinTokenClient interface {
	CreateOAuth2Token(context.Context, *signin.CreateOAuth2TokenInput, ...func(*signin.Options)) (*signin.CreateOAuth2TokenOutput, error)
}

func runBrowserLogin(ctx context.Context, opts browserLoginOptions) (*browserLoginResult, error) {
	if opts.Out == nil {
		opts.Out = io.Discard
	}
	if opts.Err == nil {
		opts.Err = io.Discard
	}
	if opts.In == nil {
		opts.In = os.Stdin
	}
	if opts.Open == nil {
		opts.Open = openBrowser
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}

	region := defaultString(opts.Region, DefaultRegion)
	baseURL, err := signInBaseURL(region)
	if err != nil {
		return nil, err
	}
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithCredentialsProvider(aws.AnonymousCredentials{}),
	)
	if err != nil {
		return nil, fmt.Errorf("load AWS Sign-In config: %w", err)
	}
	client := signin.NewFromConfig(cfg)

	key, err := generateP256Key()
	if err != nil {
		return nil, err
	}
	state, err := randomUUID()
	if err != nil {
		return nil, err
	}
	codeVerifier, codeChallenge, err := pkcePair()
	if err != nil {
		return nil, err
	}

	if opts.NoBrowser {
		return runCrossDeviceBrowserLogin(ctx, client, baseURL, key, state, codeVerifier, codeChallenge, opts)
	}
	return runSameDeviceBrowserLogin(ctx, client, baseURL, key, state, codeVerifier, codeChallenge, opts)
}

func runSameDeviceBrowserLogin(ctx context.Context, client signinTokenClient, baseURL string, key *ecdsa.PrivateKey, state, codeVerifier, codeChallenge string, opts browserLoginOptions) (*browserLoginResult, error) {
	receiver, err := newAuthCodeReceiver()
	if err != nil {
		return nil, err
	}
	defer receiver.Close()

	authURL := buildAuthorizationURL(baseURL, sameDeviceClientID, state, codeChallenge, receiver.RedirectURI())
	fmt.Fprintln(opts.Out, "Attempting to open your default browser.")
	fmt.Fprintln(opts.Out, "If the browser does not open, open the following URL:")
	fmt.Fprintf(opts.Out, "\n%s\n\n", authURL)
	if err := opts.Open(authURL); err != nil {
		fmt.Fprintf(opts.Err, "Could not open browser automatically: %v\n", err)
	}

	code, returnedState, err := receiver.Wait(ctx, authTimeout)
	if err != nil {
		return nil, err
	}
	if returnedState != state {
		return nil, fmt.Errorf("browser sign-in state mismatch")
	}

	return exchangeAndCacheLoginToken(ctx, client, baseURL, sameDeviceClientID, code, codeVerifier, receiver.RedirectURI(), key, opts.Now)
}

func runCrossDeviceBrowserLogin(ctx context.Context, client signinTokenClient, baseURL string, key *ecdsa.PrivateKey, state, codeVerifier, codeChallenge string, opts browserLoginOptions) (*browserLoginResult, error) {
	redirectURI := strings.TrimRight(baseURL, "/") + "/v1/sessions/confirmation"
	authURL := buildAuthorizationURL(baseURL, crossDeviceClientID, state, codeChallenge, redirectURI)

	fmt.Fprintln(opts.Out, "Browser will not be automatically opened.")
	fmt.Fprintln(opts.Out, "Please visit the following URL:")
	fmt.Fprintf(opts.Out, "\n%s\n\n", authURL)

	p := newPrompter(opts.In, opts.Out, nil)
	verificationCode, err := p.ask("Enter the authorization code displayed in your browser: ", "")
	if err != nil {
		return nil, err
	}
	code, returnedState, err := parseCrossDeviceVerificationCode(verificationCode)
	if err != nil {
		return nil, err
	}
	if returnedState != state {
		return nil, fmt.Errorf("browser sign-in state mismatch")
	}

	return exchangeAndCacheLoginToken(ctx, client, baseURL, crossDeviceClientID, code, codeVerifier, redirectURI, key, opts.Now)
}

func exchangeAndCacheLoginToken(ctx context.Context, client signinTokenClient, baseURL, clientID, code, codeVerifier, redirectURI string, key *ecdsa.PrivateKey, now func() time.Time) (*browserLoginResult, error) {
	out, err := client.CreateOAuth2Token(ctx, &signin.CreateOAuth2TokenInput{
		TokenInput: &signintypes.CreateOAuth2TokenRequestBody{
			ClientId:     aws.String(clientID),
			GrantType:    aws.String("authorization_code"),
			Code:         aws.String(code),
			CodeVerifier: aws.String(codeVerifier),
			RedirectUri:  aws.String(redirectURI),
		},
	}, signin.WithAPIOptions(func(stack *middleware.Stack) error {
		return stack.Finalize.Add(&dpopSigner{Key: key}, middleware.After)
	}))
	if err != nil {
		return nil, fmt.Errorf("exchange browser authorization code: %w", err)
	}
	if out == nil || out.TokenOutput == nil {
		return nil, fmt.Errorf("exchange browser authorization code: empty token response")
	}
	token := out.TokenOutput
	if token.IdToken == nil || *token.IdToken == "" {
		return nil, fmt.Errorf("exchange browser authorization code: missing identity token")
	}
	sessionARN, err := loginSessionFromIDToken(*token.IdToken)
	if err != nil {
		return nil, err
	}
	accountID, err := accountIDFromARN(sessionARN)
	if err != nil {
		return nil, err
	}
	if err := writeLoginToken(sessionARN, accountID, clientID, token, key, now); err != nil {
		return nil, err
	}
	return &browserLoginResult{SessionARN: sessionARN}, nil
}

func signInBaseURL(region string) (string, error) {
	endpoint, err := signin.NewDefaultEndpointResolverV2().ResolveEndpoint(context.Background(), signin.EndpointParameters{
		Region:         ptr.String(region),
		IsControlPlane: ptr.Bool(false),
	})
	if err != nil {
		return "", fmt.Errorf("resolve AWS Sign-In endpoint: %w", err)
	}
	url := endpoint.URI.String()
	if url == "" {
		return "", fmt.Errorf("resolve AWS Sign-In endpoint: empty endpoint URL")
	}
	return strings.TrimRight(url, "/"), nil
}

func buildAuthorizationURL(baseURL, clientID, state, codeChallenge, redirectURI string) string {
	u, _ := url.Parse(strings.TrimRight(baseURL, "/") + "/v1/authorize")
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", clientID)
	q.Set("state", state)
	q.Set("code_challenge_method", "SHA-256")
	q.Set("scope", "openid")
	q.Set("code_challenge", codeChallenge)
	if redirectURI != "" {
		q.Set("redirect_uri", redirectURI)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

type authCodeReceiver struct {
	listener net.Listener
	server   *http.Server
	result   chan authCodeResult
}

type authCodeResult struct {
	Code  string
	State string
	Err   error
}

func newAuthCodeReceiver() (*authCodeReceiver, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("start browser sign-in callback server: %w", err)
	}
	receiver := &authCodeReceiver{
		listener: listener,
		result:   make(chan authCodeResult, 1),
	}
	mux := http.NewServeMux()
	mux.HandleFunc(authCallbackPath, receiver.handleCallback)
	receiver.server = &http.Server{Handler: mux}
	go func() {
		if err := receiver.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			select {
			case receiver.result <- authCodeResult{Err: err}:
			default:
			}
		}
	}()
	return receiver, nil
}

func (r *authCodeReceiver) RedirectURI() string {
	return "http://" + r.listener.Addr().String() + authCallbackPath
}

func (r *authCodeReceiver) Wait(ctx context.Context, timeout time.Duration) (string, string, error) {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case result := <-r.result:
		if result.Err != nil {
			return "", "", result.Err
		}
		if result.Code == "" || result.State == "" {
			return "", "", fmt.Errorf("browser sign-in did not return an authorization code")
		}
		return result.Code, result.State, nil
	case <-waitCtx.Done():
		return "", "", fmt.Errorf("browser sign-in timed out")
	}
}

func (r *authCodeReceiver) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = r.server.Shutdown(ctx)
}

func (r *authCodeReceiver) handleCallback(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, "<!doctype html><html><body><h1>AWS sign-in complete</h1><p>You can return to Cloud Forge.</p></body></html>")

	query := req.URL.Query()
	if query.Get("error") != "" {
		r.sendResult(authCodeResult{Err: fmt.Errorf("browser sign-in returned error: %s", query.Get("error"))})
		return
	}
	r.sendResult(authCodeResult{
		Code:  query.Get("code"),
		State: query.Get("state"),
	})
}

func (r *authCodeReceiver) sendResult(result authCodeResult) {
	select {
	case r.result <- result:
	default:
	}
}

func parseCrossDeviceVerificationCode(value string) (string, string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", fmt.Errorf("authorization code is required")
	}
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		raw, err = base64.RawStdEncoding.DecodeString(value)
	}
	if err != nil {
		return "", "", fmt.Errorf("decode authorization code: %w", err)
	}
	params, err := url.ParseQuery(string(raw))
	if err != nil {
		return "", "", fmt.Errorf("parse authorization code: %w", err)
	}
	code := params.Get("code")
	if code == "" {
		code = params.Get("auth_code")
	}
	state := params.Get("state")
	if code == "" || state == "" {
		return "", "", fmt.Errorf("authorization code is missing code or state")
	}
	return code, state, nil
}

func pkcePair() (string, string, error) {
	verifierBytes := make([]byte, 48)
	if _, err := cryptorand.Read(verifierBytes); err != nil {
		return "", "", fmt.Errorf("generate PKCE verifier: %w", err)
	}
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func randomUUID() (string, error) {
	var b [16]byte
	if _, err := cryptorand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate OAuth state: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4],
		b[4:6],
		b[6:8],
		b[8:10],
		b[10:16],
	), nil
}

func generateP256Key() (*ecdsa.PrivateKey, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), cryptorand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate DPoP key: %w", err)
	}
	return key, nil
}

type dpopSigner struct {
	Key *ecdsa.PrivateKey
}

func (*dpopSigner) ID() string {
	return "cloudForgeDPoP"
}

func (s *dpopSigner) HandleFinalize(ctx context.Context, in middleware.FinalizeInput, next middleware.FinalizeHandler) (middleware.FinalizeOutput, middleware.Metadata, error) {
	req, ok := in.Request.(*smithyhttp.Request)
	if !ok {
		return middleware.FinalizeOutput{}, middleware.Metadata{}, fmt.Errorf("unexpected request transport %T", in.Request)
	}
	dpop, err := buildDPoPHeader(s.Key, req.URL.String(), time.Now)
	if err != nil {
		return middleware.FinalizeOutput{}, middleware.Metadata{}, err
	}
	req.Header.Set("DPoP", dpop)
	return next.HandleFinalize(ctx, in)
}

func buildDPoPHeader(key *ecdsa.PrivateKey, htu string, now func() time.Time) (string, error) {
	if key == nil {
		return "", fmt.Errorf("missing DPoP key")
	}
	jti, err := randomUUID()
	if err != nil {
		return "", err
	}
	header, err := jsonBase64(map[string]any{
		"typ": "dpop+jwt",
		"alg": "ES256",
		"jwk": map[string]string{
			"kty": "EC",
			"x":   base64.RawURLEncoding.EncodeToString(pad32(key.X.Bytes())),
			"y":   base64.RawURLEncoding.EncodeToString(pad32(key.Y.Bytes())),
			"crv": "P-256",
		},
	})
	if err != nil {
		return "", err
	}
	payload, err := jsonBase64(map[string]any{
		"htm": "POST",
		"htu": htu,
		"iat": now().Unix(),
		"jti": jti,
	})
	if err != nil {
		return "", err
	}
	message := header + "." + payload
	sum := sha256.Sum256([]byte(message))
	r, s, err := ecdsa.Sign(cryptorand.Reader, key, sum[:])
	if err != nil {
		return "", fmt.Errorf("sign DPoP proof: %w", err)
	}
	signature := make([]byte, 64)
	r.FillBytes(signature[:32])
	s.FillBytes(signature[32:])
	return message + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func jsonBase64(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func pad32(value []byte) []byte {
	if len(value) >= 32 {
		return value
	}
	out := make([]byte, 32)
	copy(out[32-len(value):], value)
	return out
}

func loginSessionFromIDToken(idToken string) (string, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid AWS identity token")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		payload, err = base64.URLEncoding.DecodeString(parts[1])
	}
	if err != nil {
		return "", fmt.Errorf("decode AWS identity token: %w", err)
	}
	var claims struct {
		Subject string `json:"sub"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("parse AWS identity token: %w", err)
	}
	if claims.Subject == "" {
		return "", fmt.Errorf("AWS identity token is missing subject")
	}
	return claims.Subject, nil
}

func accountIDFromARN(arn string) (string, error) {
	parts := strings.Split(arn, ":")
	if len(parts) < 6 || parts[4] == "" {
		return "", fmt.Errorf("could not extract AWS account ID from %q", arn)
	}
	return parts[4], nil
}

func writeLoginToken(sessionARN, accountID, clientID string, token *signintypes.CreateOAuth2TokenResponseBody, key *ecdsa.PrivateKey, now func() time.Time) error {
	if now == nil {
		now = time.Now
	}
	if token == nil || token.AccessToken == nil ||
		token.AccessToken.AccessKeyId == nil ||
		token.AccessToken.SecretAccessKey == nil ||
		token.AccessToken.SessionToken == nil ||
		token.ExpiresIn == nil ||
		token.RefreshToken == nil ||
		token.TokenType == nil {
		return fmt.Errorf("AWS Sign-In token response is incomplete")
	}
	keyPEM, err := ecPrivateKeyPEM(key)
	if err != nil {
		return err
	}
	cachePath, err := logincreds.StandardCachedTokenFilepath(sessionARN, os.Getenv("AWS_LOGIN_CACHE_DIRECTORY"))
	if err != nil {
		return err
	}
	expiresAt := now().Add(time.Duration(*token.ExpiresIn) * time.Second).UTC().Format(time.RFC3339)
	data := map[string]any{
		"accessToken": map[string]string{
			"accessKeyId":     aws.ToString(token.AccessToken.AccessKeyId),
			"secretAccessKey": aws.ToString(token.AccessToken.SecretAccessKey),
			"sessionToken":    aws.ToString(token.AccessToken.SessionToken),
			"accountId":       accountID,
			"expiresAt":       expiresAt,
		},
		"tokenType":     aws.ToString(token.TokenType),
		"clientId":      clientID,
		"refreshToken":  aws.ToString(token.RefreshToken),
		"identityToken": aws.ToString(token.IdToken),
		"idToken":       aws.ToString(token.IdToken),
		"dpopKey":       keyPEM,
	}
	return writeJSONFile(cachePath, data, 0600)
}

func ecPrivateKeyPEM(key *ecdsa.PrivateKey) (string, error) {
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return "", fmt.Errorf("marshal DPoP key: %w", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: der,
	})), nil
}
