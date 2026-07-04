package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cloud-forge/cli/internal/awsauth"
	"github.com/cloud-forge/cli/internal/awsdeploy"
	"github.com/cloud-forge/cli/internal/aliyunauth"
	"github.com/cloud-forge/cli/internal/telemetry"
	"github.com/cloud-forge/cli/pkg/store"
)

var Version = "0.3.0"

const (
	defaultAWSRegion    = "us-east-1"
	defaultAliyunRegion = "cn-hongkong"
	defaultCatalogBaseURL   = "https://cdn.jsdelivr.net/gh/CoreNovaLabs/cloud-forge-catalog@main"
	fallbackCatalogBaseURL  = "https://raw.githubusercontent.com/CoreNovaLabs/cloud-forge-catalog/main"
	defaultStoreURL         = defaultCatalogBaseURL + "/index/apps.json"
	defaultStoreFallbackURL = fallbackCatalogBaseURL + "/index/apps.json"
)

type commonFlags struct {
	storeURL          string
	cacheDir          string
	cacheTTL          time.Duration
	telemetryEndpoint string
	cloud             string
	category          string
	tags              listFlag
}

type listFlag []string

type keyValueFlag map[string]string

type deployFlags struct {
	region       string
	profile      string
	stackName    string
	instanceType string
	keyName      string
	sshKey       string
	sshKeyPath   string
	domainName   string
	hostedZoneID string
	diskSize     string
	vpcID        string
	subnetID     string
	vswitchID    string
	allowedIP    string
	imageID      string
	latestAMIID  string
	caddyTlsMode string
	parameters   keyValueFlag
	dryRun       bool
	noWait       bool
	waitReady    bool
	noWaitReady  bool
	timeout      time.Duration
	progress     string
}

type deleteFlags struct {
	region   string
	profile  string
	noWait   bool
	timeout  time.Duration
	progress string
}

type authFlags struct {
	profile   string
	region    string
	method    string
	noBrowser bool
}

type awsStackDeployer interface {
	Deploy(context.Context, awsdeploy.DeployInput) (*awsdeploy.DeployOutput, error)
	Destroy(context.Context, awsdeploy.DestroyInput) (*awsdeploy.DestroyOutput, error)
}

var newAWSDeployer = func(ctx context.Context, cfg awsdeploy.Config) (awsStackDeployer, error) {
	return awsdeploy.New(ctx, cfg)
}

var ensureAWSKeyPair = awsdeploy.EnsureKeyPair

var validStackName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]{0,127}$`)

func (f *listFlag) String() string {
	return strings.Join(*f, ",")
}

func (f *listFlag) Set(value string) error {
	if value == "" {
		return nil
	}
	*f = append(*f, value)
	return nil
}

func (f keyValueFlag) String() string {
	if len(f) == 0 {
		return ""
	}
	keys := make([]string, 0, len(f))
	for key := range f {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+f[key])
	}
	return strings.Join(parts, ",")
}

func (f keyValueFlag) Set(value string) error {
	key, val, ok := strings.Cut(value, "=")
	key = strings.TrimSpace(key)
	if !ok || key == "" {
		return fmt.Errorf("expected Name=Value")
	}
	f[key] = val
	return nil
}

// Run executes the CLI and returns a process exit code.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return RunWithIO(ctx, args, os.Stdin, stdout, stderr)
}

func RunWithIO(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	switch args[0] {
	case "search":
		if wantsHelp(args[1:]) {
			printCommandHelp(stdout, "search")
			return 0
		}
		return runSearch(ctx, args[1:], stdout, stderr)
	case "show":
		if wantsHelp(args[1:]) {
			printCommandHelp(stdout, "show")
			return 0
		}
		return runShow(ctx, args[1:], stdout, stderr)
	case "template":
		if wantsHelp(args[1:]) {
			printCommandHelp(stdout, "template")
			return 0
		}
		return runTemplate(ctx, args[1:], stdout, stderr)
	case "deploy":
		if wantsHelp(args[1:]) {
			printCommandHelp(stdout, "deploy")
			return 0
		}
		return runDeploy(ctx, args[1:], stdout, stderr)
	case "delete", "destroy":
		if wantsHelp(args[1:]) {
			printCommandHelp(stdout, "delete")
			return 0
		}
		return runDelete(ctx, args[1:], stdout, stderr)
	case "auth":
		if wantsHelp(args[1:]) {
			topic := helpTopic(args[1:])
			if topic == "" {
				topic = "auth"
			}
			printCommandHelp(stdout, topic)
			return 0
		}
		return runAuth(ctx, args[1:], stdin, stdout, stderr)
	case "version":
		fmt.Fprintf(stdout, "cloud-forge %s\n", Version)
		return 0
	case "help", "-h", "--help":
		if len(args) > 1 {
			printCommandHelp(stdout, strings.Join(args[1:], " "))
			return 0
		}
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func runAuth(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: cloud-forge auth <aws|aliyun> [status]")
		return 2
	}
	switch args[0] {
	case "aws":
		return runAWSAuth(ctx, args[1:], stdin, stdout, stderr)
	case "aliyun":
		return runAliyunAuth(ctx, args[1:], stdin, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "auth supports aws or aliyun, got %q\n", args[0])
		return 2
	}
}

func runAWSAuth(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := newFlagSet("auth aws", stderr)
	auth := addAuthFlags(flags)
	positionals, err := parseInterspersed(flags, args)
	if err != nil {
		return 2
	}
	if len(positionals) > 1 {
		fmt.Fprintln(stderr, "usage: cloud-forge auth aws [status] [--method auto|browser|access-key]")
		return 2
	}
	statusOnly := false
	if len(positionals) == 1 {
		if positionals[0] != "status" {
			fmt.Fprintf(stderr, "unknown auth aws command %q\n", positionals[0])
			return 2
		}
		statusOnly = true
	}

	var stdinFile *os.File
	if f, ok := stdin.(*os.File); ok {
		stdinFile = f
	}
	runner := awsauth.Runner{
		In:    stdin,
		Out:   stdout,
		Err:   stderr,
		Stdin: stdinFile,
	}
	err = runner.Run(ctx, awsauth.Options{
		Profile:    auth.profile,
		Region:     auth.region,
		Method:     auth.method,
		NoBrowser:  auth.noBrowser,
		StatusOnly: statusOnly,
	})
	if err != nil {
		printUserError(stderr, err)
		return 1
	}
	return 0
}

func runAliyunAuth(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := newFlagSet("auth aliyun", stderr)
	auth := addAliyunAuthFlags(flags)
	positionals, err := parseInterspersed(flags, args)
	if err != nil {
		return 2
	}
	if len(positionals) > 1 {
		fmt.Fprintln(stderr, "usage: cloud-forge auth aliyun [status]")
		return 2
	}
	statusOnly := false
	if len(positionals) == 1 {
		if positionals[0] != "status" {
			fmt.Fprintf(stderr, "unknown auth aliyun command %q\n", positionals[0])
			return 2
		}
		statusOnly = true
	}

	var stdinFile *os.File
	if f, ok := stdin.(*os.File); ok {
		stdinFile = f
	}
	runner := aliyunauth.Runner{
		In:    stdin,
		Out:   stdout,
		Err:   stderr,
		Stdin: stdinFile,
	}
	err = runner.Run(ctx, aliyunauth.Options{
		Profile:    auth.profile,
		Region:     auth.region,
		StatusOnly: statusOnly,
	})
	if err != nil {
		printUserError(stderr, err)
		return 1
	}
	return 0
}

func runDelete(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	started := time.Now()
	flags := newFlagSet("delete", stderr)
	common := addCommonFlags(flags)
	del := addDeleteFlags(flags)
	positionals, err := parseInterspersed(flags, args)
	if err != nil {
		return 2
	}
	if len(positionals) != 1 {
		fmt.Fprintln(stderr, "usage: cloud-forge delete <stack-name> --cloud aws [--region us-east-1]")
		return 2
	}
	if common.cloud == "" {
		common.cloud = "aws"
	}
	if common.cloud == "aliyun" {
		return runAliyunDelete(ctx, args, stdout, stderr)
	}
	if common.cloud != "aws" {
		fmt.Fprintf(stderr, "delete supports aws and aliyun only.\n")
		return 2
	}
	stackName := positionals[0]
	if err := validateStackName(stackName); err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 2
	}
	progressMode, err := normalizeProgressMode(del.progress)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 2
	}
	del.progress = progressMode

	deployer, err := newAWSDeployer(ctx, awsdeploy.Config{
		Region:  del.region,
		Profile: del.profile,
	})
	if err != nil {
		track(common, ctx, telemetry.Event{
			Event:      "destroy",
			Cloud:      common.cloud,
			Status:     "failed",
			DurationMS: durationMS(started),
			ErrorCode:  "aws_config",
		})
		printUserError(stderr, err)
		return 1
	}

	fmt.Fprintf(stdout, "Deleting AWS stack %s\n", stackName)
	var progress func(awsdeploy.StackProgressEvent)
	if !del.noWait && del.progress == "plain" {
		progress = printStackProgress(stdout)
	}

	result, err := deployer.Destroy(ctx, awsdeploy.DestroyInput{
		StackName: stackName,
		Wait:      !del.noWait,
		Timeout:   del.timeout,
		Progress:  progress,
	})
	if err != nil {
		track(common, ctx, telemetry.Event{
			Event:      "destroy",
			Cloud:      common.cloud,
			Status:     "failed",
			DurationMS: durationMS(started),
			ErrorCode:  "aws_delete",
		})
		printUserError(stderr, err)
		return 1
	}

	track(common, ctx, telemetry.Event{
		Event:      "destroy",
		Cloud:      common.cloud,
		Status:     "success",
		DurationMS: durationMS(started),
	})

	printDeleteResult(stdout, result)
	return 0
}

func runDeploy(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	started := time.Now()
	flags := newFlagSet("deploy", stderr)
	common := addCommonFlags(flags)
	deploy := addDeployFlags(flags)
	positionals, err := parseInterspersed(flags, args)
	if err != nil {
		return 2
	}
	if len(positionals) != 1 {
		fmt.Fprintln(stderr, "usage: cloud-forge deploy <app> --cloud aws [--region us-east-1] [--param Name=Value]")
		return 2
	}
	if common.cloud == "" {
		common.cloud = "aws"
	}
	if common.cloud == "aliyun" {
		return runAliyunDeploy(ctx, args, stdout, stderr)
	}
	if common.cloud != "aws" {
		fmt.Fprintf(stderr, "deploy supports aws and aliyun only.\n")
		return 2
	}
	keyNameFlagSet := flagWasSet(flags, "key") || flagWasSet(flags, "key-name")
	progressMode, err := normalizeProgressMode(deploy.progress)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 2
	}
	deploy.progress = progressMode

	appID := positionals[0]
	st, code := loadStore(ctx, common, stderr)
	if code != 0 {
		track(common, ctx, telemetry.Event{
			Event:      "deploy",
			AppID:      appID,
			Cloud:      common.cloud,
			Status:     "failed",
			DurationMS: durationMS(started),
			ErrorCode:  "load_store",
		})
		return code
	}

	app, err := st.Get(appID)
	if err != nil {
		track(common, ctx, telemetry.Event{
			Event:      "deploy",
			AppID:      appID,
			Cloud:      common.cloud,
			Status:     "failed",
			DurationMS: durationMS(started),
			ErrorCode:  "app_not_found",
		})
		printUserError(stderr, err)
		return 1
	}
	if !contains(app.Clouds, "aws") {
		track(common, ctx, telemetry.Event{
			Event:      "deploy",
			AppID:      app.ID,
			AppVersion: app.Version,
			Cloud:      common.cloud,
			Status:     "failed",
			DurationMS: durationMS(started),
			ErrorCode:  "cloud_not_supported",
		})
		fmt.Fprintf(stderr, "app %q does not support aws\n", app.ID)
		return 1
	}

	body, err := st.GetTemplate(ctx, app.ID, "aws")
	if err != nil {
		track(common, ctx, telemetry.Event{
			Event:      "deploy",
			AppID:      app.ID,
			AppVersion: app.Version,
			Cloud:      common.cloud,
			Status:     "failed",
			DurationMS: durationMS(started),
			ErrorCode:  "template_fetch",
		})
		printUserError(stderr, err)
		return 1
	}

	autoKeyPair, err := configureAWSSSHKey(app, deploy, keyNameFlagSet)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 2
	}

	parameters, err := buildAWSDeployParameters(app, deploy)
	if err != nil {
		track(common, ctx, telemetry.Event{
			Event:      "deploy",
			AppID:      app.ID,
			AppVersion: app.Version,
			Cloud:      common.cloud,
			Status:     "failed",
			DurationMS: durationMS(started),
			ErrorCode:  "invalid_parameters",
		})
		fmt.Fprintf(stderr, "%v\n", err)
		return 2
	}

	stackName := deploy.stackName
	if stackName == "" {
		stackName = defaultStackName(app.ID)
	}
	if err := validateStackName(stackName); err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 2
	}

	deployer, err := newAWSDeployer(ctx, awsdeploy.Config{
		Region:  deploy.region,
		Profile: deploy.profile,
	})
	if err != nil {
		track(common, ctx, telemetry.Event{
			Event:      "deploy",
			AppID:      app.ID,
			AppVersion: app.Version,
			Cloud:      common.cloud,
			Status:     "failed",
			DurationMS: durationMS(started),
			ErrorCode:  "aws_config",
		})
		printUserError(stderr, err)
		return 1
	}

	var keyPair *awsdeploy.EnsureKeyPairOutput
	if autoKeyPair && !deploy.dryRun {
		keyPair, err = ensureAWSKeyPair(ctx, awsdeploy.Config{
			Region:  deploy.region,
			Profile: deploy.profile,
		}, awsdeploy.EnsureKeyPairInput{
			KeyName:        parameters["KeyName"],
			PrivateKeyPath: deploy.sshKeyPath,
		})
		if err != nil {
			track(common, ctx, telemetry.Event{
				Event:      "deploy",
				AppID:      app.ID,
				AppVersion: app.Version,
				Cloud:      common.cloud,
				Status:     "failed",
				DurationMS: durationMS(started),
				ErrorCode:  "aws_key_pair",
			})
			printUserError(stderr, err)
			return 1
		}
	}

	if !deploy.dryRun {
		printDeployWarnings(stderr, app, parameters)
	}

	fmt.Fprintf(stdout, "Deploying %s to AWS stack %s\n", app.ID, stackName)
	if keyPair != nil {
		fmt.Fprintf(stdout, "SSH key pair: %s\n", keyPair.KeyName)
		fmt.Fprintf(stdout, "SSH private key: %s\n", keyPair.PrivateKeyPath)
	}
	if deploy.dryRun {
		fmt.Fprintln(stdout, "Mode: dry-run")
	}

	var progress func(awsdeploy.StackProgressEvent)
	if !deploy.dryRun && !deploy.noWait && deploy.progress == "plain" {
		progress = printStackProgress(stdout)
	}

	result, err := deployer.Deploy(ctx, awsdeploy.DeployInput{
		AppID:        app.ID,
		AppVersion:   app.Version,
		StackName:    stackName,
		TemplateBody: body,
		Parameters:   parameters,
		DryRun:       deploy.dryRun,
		Wait:         !deploy.noWait,
		Timeout:      deploy.timeout,
		Progress:     progress,
	})
	if err != nil {
		track(common, ctx, telemetry.Event{
			Event:      "deploy",
			AppID:      app.ID,
			AppVersion: app.Version,
			Cloud:      common.cloud,
			Status:     "failed",
			DurationMS: durationMS(started),
			ErrorCode:  "aws_deploy",
		})
		printUserError(stderr, err)
		return 1
	}

	track(common, ctx, telemetry.Event{
		Event:      "deploy",
		AppID:      app.ID,
		AppVersion: app.Version,
		Cloud:      common.cloud,
		Status:     "success",
		DurationMS: durationMS(started),
	})

	waitReady := deploy.waitReady && !deploy.noWaitReady
	if !deploy.dryRun && !deploy.noWait && waitReady {
		deadline := started.Add(deploy.timeout)
		showProgress := deploy.progress == "plain"
		if err := waitServiceReady(ctx, stdout, result.Outputs, deadline, showProgress); err != nil {
			printDeployResult(stdout, result)
			fmt.Fprintf(stderr, "\n%v\n", err)
			return 1
		}
	} else if !deploy.dryRun && !deploy.noWait {
		fmt.Fprintln(stdout, "\nNote: Stack is ready; app bootstrap may still take a few minutes (pass --wait-ready or omit --no-wait-ready to wait).")
	}

	printDeployResult(stdout, result)
	if !deploy.dryRun {
		fmt.Fprintf(stdout, "\nTo remove later: cloud-forge delete %s --cloud aws --region %s\n", stackName, result.Region)
	}
	return 0
}

func runSearch(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	started := time.Now()
	flags := newFlagSet("search", stderr)
	common := addCommonFlags(flags)
	positionals, err := parseInterspersed(flags, args)
	if err != nil {
		return 2
	}

	query := strings.Join(positionals, " ")
	apps, code := loadApps(ctx, common, store.Filter{
		Query:    query,
		Cloud:    common.cloud,
		Category: common.category,
		Tags:     []string(common.tags),
	}, stderr)
	if code != 0 {
		track(common, ctx, telemetry.Event{
			Event:      "search",
			Cloud:      common.cloud,
			Status:     "failed",
			DurationMS: durationMS(started),
			ErrorCode:  "load_apps",
		})
		return code
	}

	track(common, ctx, telemetry.Event{
		Event:      "search",
		Cloud:      common.cloud,
		Status:     "success",
		DurationMS: durationMS(started),
	})

	if len(apps) == 0 {
		fmt.Fprintln(stdout, "No apps found.")
		return 0
	}

	fmt.Fprintf(stdout, "%-18s %-18s %-12s %-15s %s\n", "ID", "NAME", "CATEGORY", "CLOUDS", "PRICE")
	for _, app := range apps {
		fmt.Fprintf(stdout, "%-18s %-18s %-12s %-15s %s\n",
			app.ID,
			app.Name,
			app.Category,
			strings.Join(app.Clouds, ","),
			app.Price,
		)
	}
	return 0
}

func runShow(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	started := time.Now()
	flags := newFlagSet("show", stderr)
	common := addCommonFlags(flags)
	positionals, err := parseInterspersed(flags, args)
	if err != nil {
		return 2
	}
	if len(positionals) != 1 {
		fmt.Fprintln(stderr, "usage: cloud-forge show <app> [flags]")
		return 2
	}

	st, code := loadStore(ctx, common, stderr)
	if code != 0 {
		track(common, ctx, telemetry.Event{
			Event:      "show",
			AppID:      positionals[0],
			Status:     "failed",
			DurationMS: durationMS(started),
			ErrorCode:  "load_store",
		})
		return code
	}

	app, err := st.Get(positionals[0])
	if err != nil {
		track(common, ctx, telemetry.Event{
			Event:      "show",
			AppID:      positionals[0],
			Status:     "failed",
			DurationMS: durationMS(started),
			ErrorCode:  "app_not_found",
		})
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}

	track(common, ctx, telemetry.Event{
		Event:      "show",
		AppID:      app.ID,
		AppVersion: app.Version,
		Status:     "success",
		DurationMS: durationMS(started),
	})

	fmt.Fprintf(stdout, "%s (%s)\n", app.Name, app.ID)
	fmt.Fprintf(stdout, "Description: %s\n", app.Desc)
	fmt.Fprintf(stdout, "Version:     %s\n", app.Version)
	fmt.Fprintf(stdout, "Category:    %s\n", app.Category)
	fmt.Fprintf(stdout, "Clouds:      %s\n", formatCloudSupport(app.Clouds))
	if app.Price != "" {
		fmt.Fprintf(stdout, "Price:       %s\n", app.Price)
	}
	if len(app.CostNotice) > 0 {
		fmt.Fprintln(stdout, "Cost notice:")
		for _, line := range app.CostNotice {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			fmt.Fprintf(stdout, "  %s\n", line)
		}
	}

	if len(app.Images) > 0 {
		fmt.Fprintln(stdout, "\nImages:")
		for _, cloud := range sortedKeys(app.Images) {
			fmt.Fprintf(stdout, "  %-8s %s\n", cloud, app.Images[cloud])
		}
	}

	if len(app.Params) > 0 {
		fmt.Fprintln(stdout, "\nParameters:")
		for _, name := range sortedKeys(app.Params) {
			param := app.Params[name]
			required := "optional"
			if param.Required || cloudRequired(param.AWS) || cloudRequired(param.Aliyun) {
				required = "required"
			}
			secret := ""
			if param.Secret {
				secret = " secret"
			}
			fmt.Fprintf(stdout, "  %-16s %-8s %s%s\n", name, required, param.Type, secret)
			if def := defaultParamValue(param, "aws"); def != "" {
				fmt.Fprintf(stdout, "    default (aws): %s\n", def)
			}
			if options := paramOptions(param, "aws"); len(options) > 0 {
				fmt.Fprintf(stdout, "    options (aws): %s\n", strings.Join(options, ", "))
			}
		}
	}

	return 0
}

func formatCloudSupport(clouds []string) string {
	return strings.Join(clouds, ", ")
}

func runTemplate(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	started := time.Now()
	flags := newFlagSet("template", stderr)
	common := addCommonFlags(flags)
	positionals, err := parseInterspersed(flags, args)
	if err != nil {
		return 2
	}
	if len(positionals) != 1 {
		fmt.Fprintln(stderr, "usage: cloud-forge template <app> --cloud <aws|aliyun> [flags]")
		return 2
	}
	if common.cloud == "" {
		common.cloud = "aws"
	}

	st, code := loadStore(ctx, common, stderr)
	if code != 0 {
		track(common, ctx, telemetry.Event{
			Event:      "template_fetch",
			AppID:      positionals[0],
			Cloud:      common.cloud,
			Status:     "failed",
			DurationMS: durationMS(started),
			ErrorCode:  "load_store",
		})
		return code
	}

	app, appErr := st.Get(positionals[0])
	body, err := st.GetTemplate(ctx, positionals[0], common.cloud)
	if err != nil {
		track(common, ctx, telemetry.Event{
			Event:      "template_fetch",
			AppID:      positionals[0],
			Cloud:      common.cloud,
			Status:     "failed",
			DurationMS: durationMS(started),
			ErrorCode:  "template_fetch",
		})
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	event := telemetry.Event{
		Event:      "template_fetch",
		AppID:      positionals[0],
		Cloud:      common.cloud,
		Status:     "success",
		DurationMS: durationMS(started),
	}
	if appErr == nil && app != nil {
		event.AppID = app.ID
		event.AppVersion = app.Version
	}
	track(common, ctx, event)

	fmt.Fprint(stdout, body)
	if !strings.HasSuffix(body, "\n") {
		fmt.Fprintln(stdout)
	}
	return 0
}

func addCommonFlags(flags *flag.FlagSet) *commonFlags {
	common := &commonFlags{
		storeURL: defaultStoreURL,
		cacheTTL: 24 * time.Hour,
	}
	if envURL := os.Getenv("CLOUD_FORGE_STORE_URL"); envURL != "" {
		common.storeURL = envURL
	}
	flags.StringVar(&common.storeURL, "store-url", common.storeURL, "catalog index URL or local path")
	flags.StringVar(&common.cacheDir, "cache-dir", "", "cache directory")
	flags.DurationVar(&common.cacheTTL, "cache-ttl", common.cacheTTL, "catalog cache TTL")
	flags.StringVar(&common.telemetryEndpoint, "telemetry-endpoint", telemetryEndpointFromEnv(), "telemetry endpoint URL")
	flags.StringVar(&common.cloud, "cloud", "", "cloud provider filter")
	flags.StringVar(&common.category, "category", "", "category filter")
	flags.Var(&common.tags, "tag", "tag filter; may be repeated")
	return common
}

func addDeployFlags(flags *flag.FlagSet) *deployFlags {
	deploy := &deployFlags{
		parameters: keyValueFlag{},
		region:     defaultAWSRegion,
		timeout:    awsdeploy.DefaultTimeout,
		waitReady:  true,
	}
	flags.StringVar(&deploy.region, "region", deploy.region, "AWS region")
	flags.StringVar(&deploy.profile, "profile", "", "AWS shared config profile")
	flags.StringVar(&deploy.stackName, "stack-name", "", "CloudFormation stack name")
	flags.StringVar(&deploy.instanceType, "instance-type", "", "CloudFormation InstanceType parameter")
	flags.StringVar(&deploy.keyName, "key", "", "existing AWS EC2 key pair name")
	flags.StringVar(&deploy.keyName, "key-name", "", "existing AWS EC2 key pair name")
	flags.StringVar(&deploy.sshKey, "ssh-key", "auto", "SSH key mode: auto or none")
	flags.StringVar(&deploy.sshKeyPath, "ssh-key-path", "", "private key path for --ssh-key auto")
	flags.StringVar(&deploy.domainName, "domain", "", "CloudFormation DomainName parameter")
	flags.StringVar(&deploy.hostedZoneID, "hosted-zone-id", "", "CloudFormation HostedZoneId parameter")
	flags.StringVar(&deploy.diskSize, "disk-size", "", "CloudFormation DiskSize parameter")
	flags.StringVar(&deploy.vpcID, "vpc", "", "CloudFormation VpcId parameter")
	flags.StringVar(&deploy.vpcID, "vpc-id", "", "CloudFormation VpcId parameter")
	flags.StringVar(&deploy.subnetID, "subnet", "", "CloudFormation SubnetId / ROS VSwitchId parameter")
	flags.StringVar(&deploy.subnetID, "subnet-id", "", "CloudFormation SubnetId parameter")
	flags.StringVar(&deploy.vswitchID, "vswitch-id", "", "ROS VSwitchId parameter (aliyun)")
	flags.StringVar(&deploy.allowedIP, "allowed-ip", "", "CloudFormation AllowedIP parameter")
	flags.StringVar(&deploy.imageID, "image-id", "", "CloudFormation ImageId parameter")
	flags.StringVar(&deploy.latestAMIID, "latest-ami-id", "", "CloudFormation LatestAmiId parameter")
	flags.StringVar(&deploy.caddyTlsMode, "caddy-tls-mode", "", "CloudFormation CaddyTlsMode parameter")
	flags.Var(deploy.parameters, "param", "CloudFormation parameter override as Name=Value; may be repeated")
	flags.Var(deploy.parameters, "parameter", "CloudFormation parameter override as Name=Value; may be repeated")
	flags.BoolVar(&deploy.dryRun, "dry-run", false, "validate template and parameters without creating or updating a stack")
	flags.BoolVar(&deploy.noWait, "no-wait", false, "return immediately after starting stack create or update")
	flags.BoolVar(&deploy.waitReady, "wait-ready", true, "wait for app bootstrap after the stack completes (default true)")
	flags.BoolVar(&deploy.noWaitReady, "no-wait-ready", false, "return after stack completes without waiting for the app endpoint")
	flags.DurationVar(&deploy.timeout, "timeout", deploy.timeout, "maximum time to wait for stack completion and app bootstrap")
	flags.StringVar(&deploy.progress, "progress", "plain", "deployment progress output: plain or none")
	return deploy
}

func addDeleteFlags(flags *flag.FlagSet) *deleteFlags {
	del := &deleteFlags{
		region:   defaultAWSRegion,
		timeout:  awsdeploy.DefaultTimeout,
		progress: "plain",
	}
	flags.StringVar(&del.region, "region", del.region, "AWS region")
	flags.StringVar(&del.profile, "profile", "", "AWS shared config profile")
	flags.BoolVar(&del.noWait, "no-wait", false, "return immediately after starting stack deletion")
	flags.DurationVar(&del.timeout, "timeout", del.timeout, "maximum time to wait for stack deletion")
	flags.StringVar(&del.progress, "progress", del.progress, "deletion progress output: plain or none")
	return del
}

func addAliyunAuthFlags(flags *flag.FlagSet) *authFlags {
	auth := &authFlags{
		profile: aliyunauth.DefaultProfile,
		region:  defaultAliyunRegion,
	}
	flags.StringVar(&auth.profile, "profile", auth.profile, "Aliyun credentials profile")
	flags.StringVar(&auth.region, "region", auth.region, "default Aliyun region (v1: cn-hongkong)")
	return auth
}

func addAuthFlags(flags *flag.FlagSet) *authFlags {
	auth := &authFlags{
		profile: defaultAuthProfile(),
		region:  defaultAWSRegion,
		method:  "browser",
	}
	flags.StringVar(&auth.profile, "profile", auth.profile, "AWS profile to check or write")
	flags.StringVar(&auth.region, "region", auth.region, "default AWS region for the profile")
	flags.StringVar(&auth.method, "method", auth.method, "auth method: auto, browser, or access-key")
	flags.BoolVar(&auth.noBrowser, "no-browser", false, "print the browser sign-in URL without opening a browser")
	return auth
}

func defaultAuthProfile() string {
	if profile := strings.TrimSpace(os.Getenv("AWS_PROFILE")); profile != "" {
		return profile
	}
	return awsauth.DefaultProfile
}

func track(common *commonFlags, ctx context.Context, event telemetry.Event) {
	client := telemetry.NewClient(telemetry.Config{
		CacheDir:   common.cacheDir,
		Endpoint:   common.telemetryEndpoint,
		CLIVersion: Version,
	})
	client.Track(ctx, event)
}

func telemetryEndpointFromEnv() string {
	if endpoint := os.Getenv("CLOUD_FORGE_TELEMETRY_ENDPOINT"); endpoint != "" {
		return endpoint
	}
	return telemetry.DefaultEndpoint
}

func durationMS(started time.Time) int64 {
	return time.Since(started).Milliseconds()
}

func loadApps(ctx context.Context, common *commonFlags, filter store.Filter, stderr io.Writer) ([]store.App, int) {
	st, code := loadStore(ctx, common, stderr)
	if code != 0 {
		return nil, code
	}

	apps, err := st.List(filter)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return nil, 1
	}
	return apps, 0
}

func loadStore(ctx context.Context, common *commonFlags, stderr io.Writer) (store.Store, int) {
	cfg := store.Config{
		IndexURL: common.storeURL,
		CacheDir: common.cacheDir,
		CacheTTL: common.cacheTTL,
	}
	if common.storeURL == defaultStoreURL {
		cfg.IndexFallbackURLs = []string{defaultStoreFallbackURL}
		cfg.TemplateBaseURLs = []string{defaultCatalogBaseURL, fallbackCatalogBaseURL}
	}

	st := store.NewHTTPStore(cfg)
	if err := st.Sync(ctx); err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return nil, 1
	}
	return st, 0
}

func newFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	return flags
}

func parseInterspersed(flags *flag.FlagSet, args []string) ([]string, error) {
	flagArgs := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			flagArgs = append(flagArgs, arg)
			if flagTakesValue(flags, arg) && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				flagArgs = append(flagArgs, args[i+1])
				i++
			}
			continue
		}
		positionals = append(positionals, arg)
	}

	if err := flags.Parse(flagArgs); err != nil {
		return nil, err
	}
	return positionals, nil
}

func flagTakesValue(flags *flag.FlagSet, arg string) bool {
	name := strings.TrimLeft(arg, "-")
	if name == "" || strings.Contains(name, "=") {
		return false
	}
	f := flags.Lookup(name)
	if f == nil {
		return true
	}
	boolValue, ok := f.Value.(interface{ IsBoolFlag() bool })
	return !ok || !boolValue.IsBoolFlag()
}

func cloudRequired(param *store.CloudParam) bool {
	return param != nil && param.Required
}

func sortedKeys[V any](items map[string]V) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func flagWasSet(flags *flag.FlagSet, name string) bool {
	seen := false
	flags.Visit(func(f *flag.Flag) {
		if f.Name == name {
			seen = true
		}
	})
	return seen
}

func configureAWSSSHKey(app *store.App, deploy *deployFlags, keyNameFlagSet bool) (bool, error) {
	mode, err := normalizeSSHKeyMode(deploy.sshKey)
	if err != nil {
		return false, err
	}
	deploy.sshKey = mode

	keyNameParamSet := false
	if deploy.parameters != nil {
		_, keyNameParamSet = deploy.parameters["KeyName"]
	}
	explicitKeyName := keyNameFlagSet || keyNameParamSet
	if mode == "none" {
		if explicitKeyName {
			return false, fmt.Errorf("--ssh-key none cannot be combined with --key, --key-name, or --param KeyName=...")
		}
		if strings.TrimSpace(deploy.sshKeyPath) != "" {
			return false, fmt.Errorf("--ssh-key-path can only be used with --ssh-key auto")
		}
		return false, nil
	}
	if explicitKeyName {
		if strings.TrimSpace(deploy.sshKeyPath) != "" {
			return false, fmt.Errorf("--ssh-key-path can only be used when Cloud Forge manages the SSH key")
		}
		return false, nil
	}
	if !appAcceptsAWSParam(app, "KeyName") {
		return false, nil
	}
	deploy.keyName = awsdeploy.DefaultKeyPairName
	return true, nil
}

func normalizeSSHKeyMode(value string) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(value))
	if mode == "" {
		mode = "auto"
	}
	switch mode {
	case "auto", "none":
		return mode, nil
	default:
		return "", fmt.Errorf("invalid --ssh-key %q; use auto or none", value)
	}
}

func normalizeProgressMode(value string) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(value))
	if mode == "" {
		mode = "plain"
	}
	switch mode {
	case "plain", "none":
		return mode, nil
	default:
		return "", fmt.Errorf("invalid --progress %q; use plain or none", value)
	}
}

func appAcceptsAWSParam(app *store.App, name string) bool {
	if app == nil || app.Params == nil {
		return false
	}
	definition, ok := app.Params[name]
	return ok && paramAppliesToCloud(definition, "aws")
}

func buildAWSDeployParameters(app *store.App, deploy *deployFlags) (map[string]string, error) {
	params := make(map[string]string)
	skipKeyNameDefault := deploy.sshKey == "none"
	for name, definition := range app.Params {
		if !paramAppliesToCloud(definition, "aws") {
			continue
		}
		if skipKeyNameDefault && name == "KeyName" {
			continue
		}
		if value := defaultParamValue(definition, "aws"); value != "" {
			params[name] = value
		}
	}

	setParameter(params, "InstanceType", deploy.instanceType)
	setParameter(params, "KeyName", deploy.keyName)
	setParameter(params, "DomainName", deploy.domainName)
	setParameter(params, "HostedZoneId", deploy.hostedZoneID)
	setParameter(params, "DiskSize", deploy.diskSize)
	setParameter(params, "VpcId", deploy.vpcID)
	setParameter(params, "SubnetId", deploy.subnetID)
	setParameter(params, "AllowedIP", deploy.allowedIP)
	setParameter(params, "ImageId", deploy.imageID)
	setParameter(params, "LatestAmiId", deploy.latestAMIID)
	setParameter(params, "CaddyTlsMode", deploy.caddyTlsMode)
	for name, value := range deploy.parameters {
		params[name] = value
	}

	if err := validateRequiredParams(app.Params, params, "aws"); err != nil {
		return nil, err
	}
	if err := validateParamOptions(app.Params, params, "aws"); err != nil {
		return nil, err
	}
	return params, nil
}

func defaultParamValue(definition store.ParamDefinition, cloud string) string {
	if cloud == "aws" && definition.AWS != nil && definition.AWS.Default != "" {
		return definition.AWS.Default
	}
	if cloud == "aliyun" && definition.Aliyun != nil && definition.Aliyun.Default != "" {
		return definition.Aliyun.Default
	}
	return scalarString(definition.Default)
}

func paramAppliesToCloud(definition store.ParamDefinition, cloud string) bool {
	switch cloud {
	case "aws":
		return definition.AWS != nil || definition.Aliyun == nil
	case "aliyun":
		return definition.Aliyun != nil || definition.AWS == nil
	default:
		return true
	}
}

func scalarString(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	default:
		return fmt.Sprint(v)
	}
}

func setParameter(params map[string]string, name, value string) {
	if value != "" {
		params[name] = value
	}
}

func validateRequiredParams(definitions map[string]store.ParamDefinition, values map[string]string, cloud string) error {
	var missing []string
	for name, definition := range definitions {
		if !paramAppliesToCloud(definition, cloud) {
			continue
		}
		required := definition.Required
		if cloud == "aws" {
			required = required || cloudRequired(definition.AWS)
		}
		if cloud == "aliyun" {
			required = required || cloudRequired(definition.Aliyun)
		}
		if required && values[name] == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return fmt.Errorf("missing required %s parameter(s): %s", cloud, strings.Join(missing, ", "))
}

func validateParamOptions(definitions map[string]store.ParamDefinition, values map[string]string, cloud string) error {
	for name, value := range values {
		if value == "" {
			continue
		}
		options := paramOptions(definitions[name], cloud)
		if len(options) == 0 || contains(options, value) {
			continue
		}
		return fmt.Errorf("invalid value %q for %s; allowed values: %s", value, name, strings.Join(options, ", "))
	}
	return nil
}

func paramOptions(definition store.ParamDefinition, cloud string) []string {
	if cloud == "aws" && definition.AWS != nil && len(definition.AWS.Options) > 0 {
		return definition.AWS.Options
	}
	if cloud == "aliyun" && definition.Aliyun != nil && len(definition.Aliyun.Options) > 0 {
		return definition.Aliyun.Options
	}
	return definition.Options
}

func defaultStackName(appID string) string {
	name := "cloud-forge-" + appID
	if len(name) <= 128 {
		return name
	}
	return strings.TrimRight(name[:128], "-")
}

func validateStackName(name string) error {
	if validStackName.MatchString(name) {
		return nil
	}
	return fmt.Errorf("invalid stack name %q; use 1-128 alphanumeric or hyphen characters, starting with a letter", name)
}

func printStackProgress(stdout io.Writer) func(awsdeploy.StackProgressEvent) {
	return func(event awsdeploy.StackProgressEvent) {
		timestamp := event.Timestamp.Local().Format("15:04:05")
		resourceType := strings.TrimSpace(event.ResourceType)
		resourceID := strings.TrimSpace(event.LogicalResourceID)
		status := strings.TrimSpace(event.ResourceStatus)

		if resourceType == "" {
			resourceType = "-"
		}
		if resourceID == "" {
			resourceID = "-"
		}
		if status == "" {
			status = "-"
		}

		fmt.Fprintf(stdout, "[%s] %s %s %s", timestamp, resourceType, resourceID, status)
		if reason := strings.TrimSpace(event.ResourceStatusReason); reason != "" {
			fmt.Fprintf(stdout, " - %s", reason)
		}
		fmt.Fprintln(stdout)
	}
}

func printDeployResult(stdout io.Writer, result *awsdeploy.DeployOutput) {
	fmt.Fprintf(stdout, "AWS account: %s\n", result.AccountID)
	fmt.Fprintf(stdout, "AWS region:  %s\n", result.Region)
	fmt.Fprintf(stdout, "Action:      %s\n", result.Action)
	if result.StackName != "" {
		fmt.Fprintf(stdout, "Stack name:  %s\n", result.StackName)
	}
	if result.Status != "" {
		fmt.Fprintf(stdout, "Status:      %s\n", result.Status)
	}
	if result.StackID != "" {
		fmt.Fprintf(stdout, "Stack ID:    %s\n", result.StackID)
	}
	if len(result.Outputs) == 0 {
		return
	}
	fmt.Fprintln(stdout, "\nOutputs:")
	for _, key := range sortedKeys(result.Outputs) {
		fmt.Fprintf(stdout, "  %-16s %s\n", key, result.Outputs[key])
	}
}
