package aliyunauth

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloud-forge/cli/internal/aliyundeploy"
	openapi "github.com/alibabacloud-go/darabonba-openapi/client"
	sts "github.com/alibabacloud-go/sts-20150401/client"
	"github.com/alibabacloud-go/tea/tea"
	"golang.org/x/term"
)

const (
	DefaultRegion  = aliyundeploy.DefaultRegion
	DefaultProfile = aliyundeploy.DefaultProfile
)

type Options struct {
	Profile    string
	Region     string
	StatusOnly bool
}

type Identity struct {
	Account string
	UserID  string
	Arn     string
	Region  string
	Profile string
}

type Runner struct {
	In    io.Reader
	Out   io.Writer
	Err   io.Writer
	Stdin *os.File
	Check func(context.Context, Config) (*Identity, error)
}

type Config struct {
	Profile         string
	Region          string
	AccessKeyID     string
	AccessKeySecret string
	SecurityToken   string
}

func (r Runner) Run(ctx context.Context, opts Options) error {
	opts = normalizeOptions(opts)
	p := newPrompter(r.In, r.Out, r.Stdin)

	if opts.StatusOnly {
		identity, err := r.checkIdentity(ctx, opts)
		if err != nil {
			return fmt.Errorf("aliyun credentials are not ready: %w", err)
		}
		printIdentity(r.Out, "Aliyun credentials are ready.", identity)
		return nil
	}

	identity, err := r.checkIdentity(ctx, opts)
	if err == nil {
		printIdentity(r.Out, "Aliyun credentials are already configured.", identity)
		answer, askErr := p.confirm("Reconfigure access keys?", false)
		if askErr != nil {
			return askErr
		}
		if !answer {
			return nil
		}
	}

	accessKeyID, err := p.askSecret("Aliyun AccessKey ID: ", false)
	if err != nil {
		return err
	}
	accessKeySecret, err := p.askSecret("Aliyun AccessKey Secret: ", true)
	if err != nil {
		return err
	}
	region, err := p.ask("Default region", opts.Region)
	if err != nil {
		return err
	}
	if region != aliyundeploy.SupportedRegion {
		return fmt.Errorf("aliyun v1 only supports region %s", aliyundeploy.SupportedRegion)
	}

	cfg := Config{
		Profile:         opts.Profile,
		Region:          region,
		AccessKeyID:     strings.TrimSpace(accessKeyID),
		AccessKeySecret: strings.TrimSpace(accessKeySecret),
	}
	if err := SaveCredentials(cfg); err != nil {
		return err
	}

	identity, err = r.checkIdentity(ctx, opts)
	if err != nil {
		return fmt.Errorf("saved credentials but verification failed: %w", err)
	}
	printIdentity(r.Out, "Aliyun credentials saved.", identity)
	return nil
}

func (r Runner) checkIdentity(ctx context.Context, opts Options) (*Identity, error) {
	cfg := Config{
		Profile: opts.Profile,
		Region:  opts.Region,
	}
	if r.Check != nil {
		return r.Check(ctx, cfg)
	}
	return CheckIdentity(ctx, cfg)
}

func CheckIdentity(ctx context.Context, cfg Config) (*Identity, error) {
	loaded, err := aliyundeploy.LoadConfig(aliyundeploy.Config{
		Region:          cfg.Region,
		AccessKeyID:     cfg.AccessKeyID,
		AccessKeySecret: cfg.AccessKeySecret,
		SecurityToken:   cfg.SecurityToken,
	})
	if err != nil {
		return nil, err
	}

	openCfg := &openapi.Config{
		AccessKeyId:     tea.String(loaded.AccessKeyID),
		AccessKeySecret: tea.String(loaded.AccessKeySecret),
		RegionId:        tea.String(loaded.Region),
	}
	if loaded.SecurityToken != "" {
		openCfg.SecurityToken = tea.String(loaded.SecurityToken)
	}
	client, err := sts.NewClient(openCfg)
	if err != nil {
		return nil, err
	}
	out, err := client.GetCallerIdentity()
	if err != nil {
		return nil, err
	}
	return &Identity{
		Account: tea.StringValue(out.Body.AccountId),
		UserID:  tea.StringValue(out.Body.UserId),
		Arn:     tea.StringValue(out.Body.Arn),
		Region:  loaded.Region,
		Profile: defaultString(cfg.Profile, DefaultProfile),
	}, nil
}

func SaveCredentials(cfg Config) error {
	path, err := aliyundeploy.CredentialsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	profile := defaultString(cfg.Profile, DefaultProfile)
	content := fmt.Sprintf("[%s]\naccess_key_id = %s\naccess_key_secret = %s\nregion = %s\n",
		profile,
		cfg.AccessKeyID,
		cfg.AccessKeySecret,
		defaultString(cfg.Region, DefaultRegion),
	)
	return os.WriteFile(path, []byte(content), 0600)
}

func normalizeOptions(opts Options) Options {
	opts.Profile = defaultString(opts.Profile, DefaultProfile)
	opts.Region = defaultString(opts.Region, DefaultRegion)
	return opts
}

func printIdentity(w io.Writer, title string, identity *Identity) {
	fmt.Fprintln(w, title)
	if identity == nil {
		return
	}
	fmt.Fprintf(w, "Account: %s\n", identity.Account)
	if identity.UserID != "" {
		fmt.Fprintf(w, "User ID: %s\n", identity.UserID)
	}
	if identity.Arn != "" {
		fmt.Fprintf(w, "ARN:     %s\n", identity.Arn)
	}
	fmt.Fprintf(w, "Region:  %s\n", identity.Region)
	fmt.Fprintf(w, "Profile: %s\n", identity.Profile)
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

type prompter struct {
	in    io.Reader
	out   io.Writer
	stdin *os.File
}

func newPrompter(in io.Reader, out io.Writer, stdin *os.File) *prompter {
	return &prompter{in: in, out: out, stdin: stdin}
}

func (p *prompter) ask(label, defaultValue string) (string, error) {
	if defaultValue != "" {
		fmt.Fprintf(p.out, "%s [%s]: ", label, defaultValue)
	} else {
		fmt.Fprintf(p.out, "%s: ", label)
	}
	line, err := p.readLine()
	if err != nil {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultValue, nil
	}
	return line, nil
}

func (p *prompter) askSecret(label string, secret bool) (string, error) {
	fmt.Fprint(p.out, label)
	if secret && p.stdin != nil && term.IsTerminal(int(p.stdin.Fd())) {
		bytes, err := term.ReadPassword(int(p.stdin.Fd()))
		fmt.Fprintln(p.out)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(bytes)), nil
	}
	return p.readLine()
}

func (p *prompter) confirm(label string, defaultYes bool) (bool, error) {
	suffix := "[y/N]"
	if defaultYes {
		suffix = "[Y/n]"
	}
	fmt.Fprintf(p.out, "%s %s: ", label, suffix)
	line, err := p.readLine()
	if err != nil {
		return false, err
	}
	line = strings.ToLower(strings.TrimSpace(line))
	if line == "" {
		return defaultYes, nil
	}
	return line == "y" || line == "yes", nil
}

func (p *prompter) readLine() (string, error) {
	reader := bufio.NewReader(p.in)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}
