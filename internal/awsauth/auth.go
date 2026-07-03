package awsauth

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/ssocreds"
	"github.com/aws/aws-sdk-go-v2/service/sso"
	ssotypes "github.com/aws/aws-sdk-go-v2/service/sso/types"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	oidctypes "github.com/aws/aws-sdk-go-v2/service/ssooidc/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"golang.org/x/term"
)

const (
	DefaultProfile    = "default"
	DefaultRegion     = "us-east-1"
	DefaultSSORegion  = "us-east-1"
	defaultSSOSession = "cloud-forge"
	identityCenterURL = "https://console.aws.amazon.com/singlesignon/home"
)

var errMissingSSOStartURL = errors.New("IAM Identity Center start URL is required")

type Options struct {
	Profile     string
	Region      string
	Method      string
	SSOStartURL string
	SSORegion   string
	AccountID   string
	RoleName    string
	NoBrowser   bool
	StatusOnly  bool
}

type Identity struct {
	Account string
	UserID  string
	Arn     string
	Region  string
	Profile string
	Source  string
}

type Runner struct {
	In    io.Reader
	Out   io.Writer
	Err   io.Writer
	Open  func(string) error
	Now   func() time.Time
	Stdin *os.File
}

func (r Runner) Run(ctx context.Context, opts Options) error {
	opts = normalizeOptions(opts)
	if err := validateMethod(opts.Method); err != nil {
		return err
	}

	p := newPrompter(r.In, r.Out, r.Stdin)
	open := r.Open
	if open == nil {
		open = openBrowser
	}
	now := r.Now
	if now == nil {
		now = time.Now
	}

	if opts.StatusOnly || opts.Method == "auto" {
		identity, err := CheckIdentity(ctx, opts.Profile, opts.Region)
		if err == nil {
			printIdentity(r.Out, "AWS credentials are ready.", identity)
			return nil
		}
		if opts.StatusOnly {
			return fmt.Errorf("AWS credentials are not ready for profile %q: %w", opts.Profile, err)
		}
		fmt.Fprintf(r.Out, "No working AWS credentials found for profile %q.\n\n", opts.Profile)
	}

	if opts.Method == "auto" || opts.Method == "sso" {
		useSSO := opts.Method == "sso"
		if opts.Method == "auto" {
			answer, err := p.confirm("Use browser sign-in with IAM Identity Center?", true)
			if err != nil {
				return err
			}
			useSSO = answer
		}
		if useSSO {
			if err := r.configureSSO(ctx, opts, p, open, now); err == nil {
				return nil
			} else if opts.Method == "sso" {
				if errors.Is(err, errMissingSSOStartURL) {
					printMissingSSOStartURL(r.Out)
				}
				return err
			} else {
				if errors.Is(err, errMissingSSOStartURL) {
					printMissingSSOStartURL(r.Out)
				} else {
					fmt.Fprintf(r.Err, "Browser sign-in failed: %v\n\n", err)
				}
				answer, askErr := p.confirm("Configure access keys instead?", true)
				if askErr != nil {
					return askErr
				}
				if !answer {
					return err
				}
			}
		}
	}

	return r.configureAccessKey(ctx, opts, p)
}

func CheckIdentity(ctx context.Context, profile, region string) (*Identity, error) {
	profile = defaultString(profile, DefaultProfile)
	region = defaultString(region, DefaultRegion)

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithSharedConfigProfile(profile),
	)
	if err != nil {
		return nil, err
	}
	creds, err := cfg.Credentials.Retrieve(ctx)
	if err != nil {
		return nil, err
	}
	out, err := sts.NewFromConfig(cfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return nil, err
	}
	return &Identity{
		Account: aws.ToString(out.Account),
		UserID:  aws.ToString(out.UserId),
		Arn:     aws.ToString(out.Arn),
		Region:  cfg.Region,
		Profile: profile,
		Source:  creds.Source,
	}, nil
}

func (r Runner) configureSSO(ctx context.Context, opts Options, p *prompter, open func(string) error, now func() time.Time) error {
	startURL := strings.TrimSpace(opts.SSOStartURL)
	if startURL == "" {
		var err error
		startURL, err = p.ask("IAM Identity Center start URL (example: https://example.awsapps.com/start): ", "")
		if err != nil {
			return err
		}
	}
	if startURL == "" {
		return errMissingSSOStartURL
	}
	if _, err := url.ParseRequestURI(startURL); err != nil {
		return fmt.Errorf("invalid IAM Identity Center start URL %q: %w", startURL, err)
	}

	ssoRegion := strings.TrimSpace(opts.SSORegion)
	if ssoRegion == "" {
		var err error
		ssoRegion, err = p.ask("IAM Identity Center region", DefaultSSORegion)
		if err != nil {
			return err
		}
	}

	oidcCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(ssoRegion),
		config.WithCredentialsProvider(aws.AnonymousCredentials{}),
	)
	if err != nil {
		return fmt.Errorf("load IAM Identity Center OIDC config: %w", err)
	}
	oidc := ssooidc.NewFromConfig(oidcCfg)

	register, err := oidc.RegisterClient(ctx, &ssooidc.RegisterClientInput{
		ClientName: aws.String("cloud-forge-cli"),
		ClientType: aws.String("public"),
		GrantTypes: []string{
			"urn:ietf:params:oauth:grant-type:device_code",
			"refresh_token",
		},
		Scopes: []string{"sso:account:access"},
	})
	if err != nil {
		return fmt.Errorf("register IAM Identity Center client: %w", err)
	}

	device, err := oidc.StartDeviceAuthorization(ctx, &ssooidc.StartDeviceAuthorizationInput{
		ClientId:     register.ClientId,
		ClientSecret: register.ClientSecret,
		StartUrl:     aws.String(startURL),
	})
	if err != nil {
		return fmt.Errorf("start browser sign-in: %w", err)
	}

	fmt.Fprintln(r.Out, "Open this URL to sign in:")
	if aws.ToString(device.VerificationUriComplete) != "" {
		fmt.Fprintf(r.Out, "%s\n", aws.ToString(device.VerificationUriComplete))
	} else {
		fmt.Fprintf(r.Out, "%s\n", aws.ToString(device.VerificationUri))
		fmt.Fprintf(r.Out, "User code: %s\n", aws.ToString(device.UserCode))
	}
	if !opts.NoBrowser && aws.ToString(device.VerificationUriComplete) != "" {
		if err := open(aws.ToString(device.VerificationUriComplete)); err != nil {
			fmt.Fprintf(r.Err, "Could not open browser automatically: %v\n", err)
			fmt.Fprintln(r.Err, "Use the URL printed above to continue sign-in.")
		}
	}

	token, err := waitForDeviceToken(ctx, oidc, register, device, now)
	if err != nil {
		return err
	}

	portalCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(ssoRegion),
		config.WithCredentialsProvider(aws.AnonymousCredentials{}),
	)
	if err != nil {
		return fmt.Errorf("load IAM Identity Center portal config: %w", err)
	}
	portal := sso.NewFromConfig(portalCfg)

	accountID, err := chooseAccount(ctx, portal, aws.ToString(token.AccessToken), opts.AccountID, p)
	if err != nil {
		return err
	}
	roleName, err := chooseRole(ctx, portal, aws.ToString(token.AccessToken), accountID, opts.RoleName, p)
	if err != nil {
		return err
	}

	sessionName := defaultSSOSession
	if opts.Profile != DefaultProfile {
		sessionName = "cloud-forge-" + sanitizeSessionName(opts.Profile)
	}
	if err := writeSSOToken(sessionName, startURL, ssoRegion, register, token, now); err != nil {
		return err
	}
	if err := writeSSOProfile(opts.Profile, opts.Region, sessionName, startURL, ssoRegion, accountID, roleName); err != nil {
		return err
	}

	identity, err := CheckIdentity(ctx, opts.Profile, opts.Region)
	if err != nil {
		return fmt.Errorf("verify IAM Identity Center profile: %w", err)
	}
	printIdentity(r.Out, "Browser sign-in complete.", identity)
	return nil
}

func waitForDeviceToken(ctx context.Context, client *ssooidc.Client, register *ssooidc.RegisterClientOutput, device *ssooidc.StartDeviceAuthorizationOutput, now func() time.Time) (*ssooidc.CreateTokenOutput, error) {
	interval := time.Duration(device.Interval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	deadline := now().Add(time.Duration(device.ExpiresIn) * time.Second)

	for now().Before(deadline) {
		out, err := client.CreateToken(ctx, &ssooidc.CreateTokenInput{
			ClientId:     register.ClientId,
			ClientSecret: register.ClientSecret,
			DeviceCode:   device.DeviceCode,
			GrantType:    aws.String("urn:ietf:params:oauth:grant-type:device_code"),
		})
		if err == nil {
			return out, nil
		}

		var pending *oidctypes.AuthorizationPendingException
		if errors.As(err, &pending) {
			time.Sleep(interval)
			continue
		}
		var slowDown *oidctypes.SlowDownException
		if errors.As(err, &slowDown) {
			interval += 5 * time.Second
			time.Sleep(interval)
			continue
		}
		return nil, fmt.Errorf("complete browser sign-in: %w", err)
	}
	return nil, fmt.Errorf("browser sign-in timed out")
}

func chooseAccount(ctx context.Context, client *sso.Client, accessToken, requested string, p *prompter) (string, error) {
	accounts, err := listAccounts(ctx, client, accessToken)
	if err != nil {
		return "", err
	}
	if len(accounts) == 0 {
		return "", fmt.Errorf("no AWS accounts are assigned to this IAM Identity Center user")
	}
	if requested != "" {
		for _, account := range accounts {
			if aws.ToString(account.AccountId) == requested {
				return requested, nil
			}
		}
		return "", fmt.Errorf("requested AWS account %q is not assigned to this user", requested)
	}
	if len(accounts) == 1 {
		return aws.ToString(accounts[0].AccountId), nil
	}

	fmt.Fprintln(p.out, "Available AWS accounts:")
	for i, account := range accounts {
		name := aws.ToString(account.AccountName)
		if name == "" {
			name = aws.ToString(account.EmailAddress)
		}
		fmt.Fprintf(p.out, "  %d. %s %s\n", i+1, aws.ToString(account.AccountId), name)
	}
	answer, err := p.ask("Choose account number", "1")
	if err != nil {
		return "", err
	}
	index, err := strconv.Atoi(answer)
	if err != nil || index < 1 || index > len(accounts) {
		return "", fmt.Errorf("invalid account selection %q", answer)
	}
	return aws.ToString(accounts[index-1].AccountId), nil
}

func listAccounts(ctx context.Context, client *sso.Client, accessToken string) ([]ssotypes.AccountInfo, error) {
	var accounts []ssotypes.AccountInfo
	paginator := sso.NewListAccountsPaginator(client, &sso.ListAccountsInput{
		AccessToken: aws.String(accessToken),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list IAM Identity Center accounts: %w", err)
		}
		accounts = append(accounts, page.AccountList...)
	}
	sort.Slice(accounts, func(i, j int) bool {
		return aws.ToString(accounts[i].AccountId) < aws.ToString(accounts[j].AccountId)
	})
	return accounts, nil
}

func chooseRole(ctx context.Context, client *sso.Client, accessToken, accountID, requested string, p *prompter) (string, error) {
	roles, err := listRoles(ctx, client, accessToken, accountID)
	if err != nil {
		return "", err
	}
	if len(roles) == 0 {
		return "", fmt.Errorf("no IAM Identity Center roles are assigned for account %s", accountID)
	}
	if requested != "" {
		for _, role := range roles {
			if aws.ToString(role.RoleName) == requested {
				return requested, nil
			}
		}
		return "", fmt.Errorf("requested role %q is not assigned for account %s", requested, accountID)
	}
	if len(roles) == 1 {
		return aws.ToString(roles[0].RoleName), nil
	}

	fmt.Fprintf(p.out, "Available roles for account %s:\n", accountID)
	for i, role := range roles {
		fmt.Fprintf(p.out, "  %d. %s\n", i+1, aws.ToString(role.RoleName))
	}
	answer, err := p.ask("Choose role number", "1")
	if err != nil {
		return "", err
	}
	index, err := strconv.Atoi(answer)
	if err != nil || index < 1 || index > len(roles) {
		return "", fmt.Errorf("invalid role selection %q", answer)
	}
	return aws.ToString(roles[index-1].RoleName), nil
}

func listRoles(ctx context.Context, client *sso.Client, accessToken, accountID string) ([]ssotypes.RoleInfo, error) {
	var roles []ssotypes.RoleInfo
	paginator := sso.NewListAccountRolesPaginator(client, &sso.ListAccountRolesInput{
		AccessToken: aws.String(accessToken),
		AccountId:   aws.String(accountID),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list IAM Identity Center roles: %w", err)
		}
		roles = append(roles, page.RoleList...)
	}
	sort.Slice(roles, func(i, j int) bool {
		return aws.ToString(roles[i].RoleName) < aws.ToString(roles[j].RoleName)
	})
	return roles, nil
}

func (r Runner) configureAccessKey(ctx context.Context, opts Options, p *prompter) error {
	fmt.Fprintln(r.Out, "Configure AWS access keys.")
	accessKeyID, err := p.ask("AWS Access Key ID: ", "")
	if err != nil {
		return err
	}
	secretAccessKey, err := p.askSecret("AWS Secret Access Key: ")
	if err != nil {
		return err
	}
	if strings.TrimSpace(accessKeyID) == "" || strings.TrimSpace(secretAccessKey) == "" {
		return fmt.Errorf("AWS access key ID and secret access key are required")
	}

	if err := writeStaticCredentials(opts.Profile, opts.Region, strings.TrimSpace(accessKeyID), strings.TrimSpace(secretAccessKey)); err != nil {
		return err
	}
	identity, err := CheckIdentity(ctx, opts.Profile, opts.Region)
	if err != nil {
		return fmt.Errorf("verify access key profile: %w", err)
	}
	printIdentity(r.Out, "Access key configuration complete.", identity)
	return nil
}

func writeStaticCredentials(profile, region, accessKeyID, secretAccessKey string) error {
	credentialsPath, configPath, err := awsConfigPaths()
	if err != nil {
		return err
	}
	if err := updateINIValues(credentialsPath, profile, map[string]string{
		"aws_access_key_id":     accessKeyID,
		"aws_secret_access_key": secretAccessKey,
	}, sessionCredentialKeys(), 0600); err != nil {
		return err
	}
	return updateINIValues(configPath, configSection(profile), map[string]string{
		"region": region,
	}, ssoProfileKeys(), 0600)
}

func writeSSOProfile(profile, region, sessionName, startURL, ssoRegion, accountID, roleName string) error {
	credentialsPath, configPath, err := awsConfigPaths()
	if err != nil {
		return err
	}
	if err := updateINIValues(credentialsPath, profile, nil, staticCredentialKeys(), 0600); err != nil {
		return err
	}
	if err := updateINIValues(configPath, configSection(profile), map[string]string{
		"region":         region,
		"sso_session":    sessionName,
		"sso_account_id": accountID,
		"sso_role_name":  roleName,
	}, append(staticCredentialKeys(), legacySSOProfileKeys()...), 0600); err != nil {
		return err
	}
	return setINIValues(configPath, "sso-session "+sessionName, map[string]string{
		"sso_start_url":           startURL,
		"sso_region":              ssoRegion,
		"sso_registration_scopes": "sso:account:access",
	}, 0600)
}

func writeSSOToken(sessionName, startURL, ssoRegion string, register *ssooidc.RegisterClientOutput, token *ssooidc.CreateTokenOutput, now func() time.Time) error {
	cachePath, err := ssocreds.StandardCachedTokenFilepath(sessionName)
	if err != nil {
		return err
	}
	expiresAt := now().Add(time.Duration(token.ExpiresIn) * time.Second).UTC().Format(time.RFC3339)
	registrationExpiresAt := time.Unix(register.ClientSecretExpiresAt, 0).UTC().Format(time.RFC3339)
	data := map[string]any{
		"accessToken":           aws.ToString(token.AccessToken),
		"expiresAt":             expiresAt,
		"refreshToken":          aws.ToString(token.RefreshToken),
		"clientId":              aws.ToString(register.ClientId),
		"clientSecret":          aws.ToString(register.ClientSecret),
		"registrationExpiresAt": registrationExpiresAt,
		"startUrl":              startURL,
		"region":                ssoRegion,
	}
	return writeJSONFile(cachePath, data, 0600)
}

func setINIValues(path, section string, values map[string]string, mode os.FileMode) error {
	return updateINIValues(path, section, values, nil, mode)
}

func updateINIValues(path, section string, values map[string]string, removeKeys []string, mode os.FileMode) error {
	var lines []string
	if data, err := os.ReadFile(path); err == nil {
		lines = strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	} else if len(values) == 0 {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	header := "[" + section + "]"
	start, end := -1, len(lines)
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			if trimmed == header {
				start = i
				end = len(lines)
				continue
			}
			if start >= 0 {
				end = i
				break
			}
		}
	}

	sectionLines := []string{header}
	if start >= 0 {
		sectionLines = append(sectionLines, lines[start+1:end]...)
	}
	removeSet := make(map[string]bool, len(removeKeys))
	for _, key := range removeKeys {
		removeSet[strings.ToLower(strings.TrimSpace(key))] = true
	}
	valueByKey := make(map[string]string, len(values))
	canonicalKey := make(map[string]string, len(values))
	for key, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(key))
		valueByKey[normalized] = value
		canonicalKey[normalized] = strings.TrimSpace(key)
	}

	updated := map[string]bool{}
	filtered := sectionLines[:1]
	for _, line := range sectionLines[1:] {
		key, _, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			filtered = append(filtered, line)
			continue
		}
		normalized := strings.ToLower(strings.TrimSpace(key))
		if removeSet[normalized] {
			continue
		}
		if value, exists := valueByKey[normalized]; exists {
			keyName := canonicalKey[normalized]
			if keyName == "" {
				keyName = strings.TrimSpace(key)
			}
			filtered = append(filtered, keyName+" = "+value)
			updated[normalized] = true
			continue
		}
		filtered = append(filtered, line)
	}
	sectionLines = filtered

	keys := make([]string, 0, len(values))
	for key := range values {
		normalized := strings.ToLower(strings.TrimSpace(key))
		if !updated[normalized] {
			keys = append(keys, normalized)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		keyName := canonicalKey[key]
		sectionLines = append(sectionLines, keyName+" = "+valueByKey[key])
	}

	if start >= 0 {
		lines = append(append(lines[:start], sectionLines...), lines[end:]...)
	} else {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, sectionLines...)
	}
	content := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(path, []byte(content), mode)
}

func staticCredentialKeys() []string {
	return []string{
		"aws_access_key_id",
		"aws_secret_access_key",
		"aws_session_token",
		"aws_security_token",
	}
}

func sessionCredentialKeys() []string {
	return []string{
		"aws_session_token",
		"aws_security_token",
	}
}

func legacySSOProfileKeys() []string {
	return []string{
		"sso_start_url",
		"sso_region",
		"sso_registration_scopes",
	}
}

func ssoProfileKeys() []string {
	return append([]string{
		"sso_session",
		"sso_account_id",
		"sso_role_name",
	}, legacySSOProfileKeys()...)
}

func writeJSONFile(path string, value any, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, mode)
}

func awsConfigPaths() (string, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	awsDir := filepath.Join(home, ".aws")
	return filepath.Join(awsDir, "credentials"), filepath.Join(awsDir, "config"), nil
}

func configSection(profile string) string {
	if profile == DefaultProfile {
		return DefaultProfile
	}
	return "profile " + profile
}

func normalizeOptions(opts Options) Options {
	opts.Profile = defaultString(opts.Profile, DefaultProfile)
	opts.Region = defaultString(opts.Region, DefaultRegion)
	opts.Method = defaultString(strings.ToLower(strings.TrimSpace(opts.Method)), "auto")
	if opts.Method == "browser" {
		opts.Method = "sso"
	}
	opts.SSORegion = defaultString(opts.SSORegion, DefaultSSORegion)
	return opts
}

func validateMethod(method string) error {
	switch method {
	case "auto", "sso", "browser", "access-key":
		return nil
	default:
		return fmt.Errorf("invalid auth method %q; use auto, sso, or access-key", method)
	}
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func sanitizeSessionName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-_")
	if out == "" {
		return shortHash(value)
	}
	return out
}

func shortHash(value string) string {
	sum := sha1.Sum([]byte(value))
	return hex.EncodeToString(sum[:])[:10]
}

func printIdentity(w io.Writer, title string, identity *Identity) {
	fmt.Fprintln(w, title)
	fmt.Fprintf(w, "Profile: %s\n", identity.Profile)
	fmt.Fprintf(w, "Region:  %s\n", identity.Region)
	if identity.Source != "" {
		fmt.Fprintf(w, "Source:  %s\n", identity.Source)
	}
	fmt.Fprintf(w, "Account: %s\n", identity.Account)
	fmt.Fprintf(w, "Arn:     %s\n", identity.Arn)
}

func printMissingSSOStartURL(w io.Writer) {
	fmt.Fprintln(w, "Browser sign-in needs your IAM Identity Center start URL before Cloud Forge can request a sign-in link.")
	fmt.Fprintf(w, "Open IAM Identity Center to find or create it: %s\n", identityCenterURL)
	fmt.Fprintln(w, "Then run: cloud-forge auth aws --method sso --sso-start-url https://example.awsapps.com/start")
	fmt.Fprintln(w)
}

func openBrowser(target string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", target).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", target).Start()
	default:
		return exec.Command("xdg-open", target).Start()
	}
}

type prompter struct {
	reader *bufio.Reader
	out    io.Writer
	stdin  *os.File
}

func newPrompter(in io.Reader, out io.Writer, stdin *os.File) *prompter {
	if in == nil {
		in = os.Stdin
	}
	if out == nil {
		out = io.Discard
	}
	return &prompter{
		reader: bufio.NewReader(in),
		out:    out,
		stdin:  stdin,
	}
}

func (p *prompter) ask(prompt, fallback string) (string, error) {
	if fallback == "" {
		fmt.Fprint(p.out, prompt)
	} else {
		fmt.Fprintf(p.out, "%s [%s]: ", strings.TrimSpace(prompt), fallback)
	}
	value, err := p.reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, nil
	}
	return value, nil
}

func (p *prompter) askSecret(prompt string) (string, error) {
	fmt.Fprint(p.out, prompt)
	if p.stdin != nil && term.IsTerminal(int(p.stdin.Fd())) {
		data, err := term.ReadPassword(int(p.stdin.Fd()))
		fmt.Fprintln(p.out)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	value, err := p.reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

func (p *prompter) confirm(prompt string, fallback bool) (bool, error) {
	defaultText := "y"
	if !fallback {
		defaultText = "n"
	}
	answer, err := p.ask(prompt, defaultText)
	if err != nil {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "", "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		return false, fmt.Errorf("expected yes or no, got %q", answer)
	}
}
