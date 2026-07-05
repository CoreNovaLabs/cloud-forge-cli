package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/cloud-forge/cli/internal/aliyundeploy"
	"github.com/cloud-forge/cli/internal/telemetry"
	"github.com/cloud-forge/cli/pkg/store"
)

type aliyunStackDeployer interface {
	Deploy(context.Context, aliyundeploy.DeployInput) (*aliyundeploy.DeployOutput, error)
	Destroy(context.Context, aliyundeploy.DestroyInput) (*aliyundeploy.DestroyOutput, error)
}

var newAliyunDeployer = func(ctx context.Context, cfg aliyundeploy.Config) (aliyunStackDeployer, error) {
	return aliyundeploy.New(ctx, cfg)
}

var resolveAliyunDeployDefaults = aliyundeploy.ResolveDeployDefaults

func runAliyunDelete(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	started := time.Now()
	flags := newFlagSet("delete", stderr)
	common := addCommonFlags(flags)
	del := addDeleteFlags(flags)
	positionals, err := parseInterspersed(flags, args)
	if err != nil {
		return 2
	}
	if len(positionals) != 1 {
		fmt.Fprintln(stderr, "usage: cloud-forge delete <stack-name> --cloud aliyun [--region cn-hongkong]")
		return 2
	}
	applyAliyunRegionDefaults(flags, &del.region)
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

	deployer, err := newAliyunDeployer(ctx, aliyundeploy.Config{Region: del.region})
	if err != nil {
		track(common, ctx, telemetry.Event{
			Event: "destroy", Cloud: "aliyun", Status: "failed",
			DurationMS: durationMS(started), ErrorCode: "aliyun_config",
		})
		printUserError(stderr, err)
		return 1
	}

	fmt.Fprintf(stdout, "Deleting Aliyun stack %s\n", stackName)
	var progress func(aliyundeploy.StackProgressEvent)
	if !del.noWait && del.progress == "plain" {
		progress = printAliyunStackProgress(stdout)
	}

	result, err := deployer.Destroy(ctx, aliyundeploy.DestroyInput{
		StackName: stackName,
		Wait:      !del.noWait,
		Timeout:   del.timeout,
		Progress:  progress,
	})
	if err != nil {
		track(common, ctx, telemetry.Event{
			Event: "destroy", Cloud: "aliyun", Status: "failed",
			DurationMS: durationMS(started), ErrorCode: "aliyun_delete",
		})
		printUserError(stderr, err)
		return 1
	}

	track(common, ctx, telemetry.Event{
		Event: "destroy", Cloud: "aliyun", Status: "success", DurationMS: durationMS(started),
	})
	printAliyunDeleteResult(stdout, result)
	return 0
}

func runAliyunDeploy(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	started := time.Now()
	flags := newFlagSet("deploy", stderr)
	common := addCommonFlags(flags)
	deploy := addDeployFlags(flags)
	positionals, err := parseInterspersed(flags, args)
	if err != nil {
		return 2
	}
	if len(positionals) != 1 {
		fmt.Fprintln(stderr, "usage: cloud-forge deploy <app> --cloud aliyun [--region cn-hongkong] [--param Name=Value]")
		return 2
	}
	applyAliyunRegionDefaults(flags, &deploy.region)
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
			Event: "deploy", AppID: appID, Cloud: "aliyun", Status: "failed",
			DurationMS: durationMS(started), ErrorCode: "load_store",
		})
		return code
	}

	app, err := st.Get(appID)
	if err != nil {
		track(common, ctx, telemetry.Event{
			Event: "deploy", AppID: appID, Cloud: "aliyun", Status: "failed",
			DurationMS: durationMS(started), ErrorCode: "app_not_found",
		})
		printUserError(stderr, err)
		return 1
	}
	if !contains(app.Clouds, "aliyun") {
		track(common, ctx, telemetry.Event{
			Event: "deploy", AppID: app.ID, AppVersion: app.Version, Cloud: "aliyun",
			Status: "failed", DurationMS: durationMS(started), ErrorCode: "cloud_not_supported",
		})
		fmt.Fprintf(stderr, "app %q does not support aliyun\n", app.ID)
		return 1
	}

	body, err := st.GetTemplate(ctx, app.ID, "aliyun")
	if err != nil {
		track(common, ctx, telemetry.Event{
			Event: "deploy", AppID: app.ID, AppVersion: app.Version, Cloud: "aliyun",
			Status: "failed", DurationMS: durationMS(started), ErrorCode: "template_fetch",
		})
		printUserError(stderr, err)
		return 1
	}

	adminPassword, err := configureAdminPassword(app, deploy)
	if err != nil {
		track(common, ctx, telemetry.Event{
			Event: "deploy", AppID: app.ID, AppVersion: app.Version, Cloud: "aliyun",
			Status: "failed", DurationMS: durationMS(started), ErrorCode: "invalid_parameters",
		})
		fmt.Fprintf(stderr, "%v\n", err)
		return 2
	}

	deployer, err := newAliyunDeployer(ctx, aliyundeploy.Config{Region: deploy.region})
	if err != nil {
		track(common, ctx, telemetry.Event{
			Event: "deploy", AppID: app.ID, AppVersion: app.Version, Cloud: "aliyun",
			Status: "failed", DurationMS: durationMS(started), ErrorCode: "aliyun_config",
		})
		printUserError(stderr, err)
		return 1
	}

	if err := configureAliyunDeployDefaults(ctx, flags, deploy, stdout, deploy.dryRun); err != nil {
		track(common, ctx, telemetry.Event{
			Event: "deploy", AppID: app.ID, AppVersion: app.Version, Cloud: "aliyun",
			Status: "failed", DurationMS: durationMS(started), ErrorCode: "invalid_parameters",
		})
		printUserError(stderr, err)
		return 2
	}

	if err := validateDomainConfig("aliyun", deploy, stderr); err != nil {
		track(common, ctx, telemetry.Event{
			Event: "deploy", AppID: app.ID, AppVersion: app.Version, Cloud: "aliyun",
			Status: "failed", DurationMS: durationMS(started), ErrorCode: "invalid_parameters",
		})
		fmt.Fprintf(stderr, "%v\n", err)
		return 2
	}

	parameters, err := buildAliyunDeployParameters(app, deploy)
	if err != nil {
		track(common, ctx, telemetry.Event{
			Event: "deploy", AppID: app.ID, AppVersion: app.Version, Cloud: "aliyun",
			Status: "failed", DurationMS: durationMS(started), ErrorCode: "invalid_parameters",
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

	if !deploy.dryRun {
		printAliyunDeployWarnings(stderr, app, parameters)
	}

	fmt.Fprintf(stdout, "Deploying %s to Aliyun stack %s\n", app.ID, stackName)
	printDomainDeployHints("aliyun", deploy, stdout)
	fmt.Fprintln(stdout, "Note: Aliyun defaults to cn-hongkong. First bootstrap may take 8-15 minutes.")
	if aliyundeploy.MainlandChinaRegion(deploy.region) {
		fmt.Fprintln(stdout, "Warning: mainland China regions may fail bootstrap due to Docker Hub / catalog CDN network restrictions.")
	}
	if deploy.dryRun {
		fmt.Fprintln(stdout, "Mode: dry-run")
	}

	var progress func(aliyundeploy.StackProgressEvent)
	if !deploy.dryRun && !deploy.noWait && deploy.progress == "plain" {
		progress = printAliyunStackProgress(stdout)
	}

	result, err := deployer.Deploy(ctx, aliyundeploy.DeployInput{
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
			Event: "deploy", AppID: app.ID, AppVersion: app.Version, Cloud: "aliyun",
			Status: "failed", DurationMS: durationMS(started), ErrorCode: "aliyun_deploy",
		})
		printUserError(stderr, err)
		return 1
	}

	track(common, ctx, telemetry.Event{
		Event: "deploy", AppID: app.ID, AppVersion: app.Version, Cloud: "aliyun",
		Status: "success", DurationMS: durationMS(started),
	})

	waitReady := deploy.waitReady && !deploy.noWaitReady
	if !deploy.dryRun && !deploy.noWait && waitReady {
		deadline := started.Add(deploy.timeout)
		showProgress := deploy.progress == "plain"
		if err := waitServiceReady(ctx, stdout, result.Outputs, deadline, showProgress); err != nil {
			printAliyunDeployResult(stdout, result)
			printAdminPassword(stdout, adminPassword, deploy.dryRun)
			fmt.Fprintf(stderr, "\n%v\n", err)
			return 1
		}
	} else if !deploy.dryRun && !deploy.noWait {
		fmt.Fprintln(stdout, "\nNote: Stack is ready; app bootstrap may still take 8-15 minutes (pass --wait-ready or omit --no-wait-ready to wait).")
	}

	printAliyunDeployResult(stdout, result)
	printAdminPassword(stdout, adminPassword, deploy.dryRun)
	if !deploy.dryRun {
		fmt.Fprintf(stdout, "\nTo remove later: cloud-forge delete %s --cloud aliyun --region %s\n", stackName, result.Region)
	}
	return 0
}

func configureAliyunDeployDefaults(ctx context.Context, flags *flag.FlagSet, deploy *deployFlags, stdout io.Writer, dryRun bool) error {
	autoVpc := !flagWasSet(flags, "vpc") && !flagWasSet(flags, "vpc-id") &&
		strings.TrimSpace(deploy.vpcID) == "" && strings.TrimSpace(os.Getenv("ALIYUN_VPC_ID")) == ""
	autoVSwitch := !flagWasSet(flags, "vswitch-id") &&
		strings.TrimSpace(deploy.vswitchID) == "" && strings.TrimSpace(os.Getenv("ALIYUN_VSWITCH_ID")) == ""
	autoKey := !flagWasSet(flags, "key") && !flagWasSet(flags, "key-name") &&
		strings.TrimSpace(deploy.keyName) == "" && strings.TrimSpace(os.Getenv("ALIYUN_KEY_NAME")) == ""

	if !autoVpc && !autoVSwitch && !autoKey {
		return nil
	}

	result, err := resolveAliyunDeployDefaults(ctx, aliyundeploy.Config{Region: deploy.region}, aliyundeploy.DeployDefaultsRequest{
		VpcID:       deploy.vpcID,
		VSwitchID:   deploy.vswitchID,
		KeyPairName: deploy.keyName,
		AutoVpc:     autoVpc,
		AutoVSwitch: autoVSwitch,
		AutoKey:     autoKey,
		DryRun:      dryRun,
	})
	if err != nil {
		return err
	}

	deploy.vpcID = result.VpcID
	deploy.vswitchID = result.VSwitchID
	deploy.keyName = result.KeyPairName
	for _, msg := range result.Messages {
		fmt.Fprintln(stdout, msg)
	}
	return nil
}

func buildAliyunDeployParameters(app *store.App, deploy *deployFlags) (map[string]string, error) {
	params := make(map[string]string)
	for name, definition := range app.Params {
		if !paramAppliesToCloud(definition, "aliyun") {
			continue
		}
		if value := defaultParamValue(definition, "aliyun"); value != "" {
			params[name] = value
		}
	}

	setParameter(params, "InstanceType", deploy.instanceType)
	setParameter(params, "KeyPairName", deploy.keyName)
	setParameter(params, "DomainName", deploy.domainName)
	setParameter(params, "DiskSize", deploy.diskSize)
	setParameter(params, "VpcId", deploy.vpcID)
	setParameter(params, "VSwitchId", deploy.vswitchID)
	setParameter(params, "AllowedIP", deploy.allowedIP)
	setParameter(params, "ImageId", deploy.imageID)
	setParameter(params, "CaddyTlsMode", deploy.caddyTlsMode)
	setParameter(params, "CaddyEmail", deploy.caddyEmail)
	setParameter(params, "DnsDomainName", deploy.dnsDomainName)
	if deploy.domainName != "" && deploy.dnsDomainName != "" {
		if rr, err := resolveDnsRR(deploy.domainName, deploy.dnsDomainName); err != nil {
			return nil, err
		} else {
			params["DnsRR"] = rr
		}
	}
	for name, value := range deploy.parameters {
		params[name] = value
	}

	if err := validateRequiredParams(app.Params, params, "aliyun"); err != nil {
		return nil, err
	}
	if err := validateParamOptions(app.Params, params, "aliyun"); err != nil {
		return nil, err
	}
	return params, nil
}

func applyAliyunRegionDefaults(flags *flag.FlagSet, region *string) {
	if !flagWasSet(flags, "region") {
		*region = defaultAliyunRegion
	}
}

func printAliyunStackProgress(stdout io.Writer) func(aliyundeploy.StackProgressEvent) {
	return func(event aliyundeploy.StackProgressEvent) {
		printStackProgressLine(stdout, stackProgressLine{
			Timestamp:            event.Timestamp,
			ResourceType:         event.ResourceType,
			LogicalResourceID:    event.LogicalResourceID,
			ResourceStatus:       event.ResourceStatus,
			ResourceStatusReason: event.ResourceStatusReason,
			PublicIP:             event.PublicIP,
		})
	}
}

func printAliyunDeployResult(stdout io.Writer, result *aliyundeploy.DeployOutput) {
	fmt.Fprintf(stdout, "Aliyun account: %s\n", result.AccountID)
	fmt.Fprintf(stdout, "Aliyun region:  %s\n", result.Region)
	fmt.Fprintf(stdout, "Action:         %s\n", result.Action)
	if result.StackName != "" {
		fmt.Fprintf(stdout, "Stack name:     %s\n", result.StackName)
	}
	if result.Status != "" {
		fmt.Fprintf(stdout, "Status:         %s\n", result.Status)
	}
	if result.StackID != "" {
		fmt.Fprintf(stdout, "Stack ID:       %s\n", result.StackID)
	}
	if len(result.Outputs) == 0 {
		return
	}
	fmt.Fprintln(stdout, "\nOutputs:")
	for _, key := range sortedKeys(result.Outputs) {
		fmt.Fprintf(stdout, "  %-16s %s\n", key, result.Outputs[key])
	}
}

func printAliyunDeleteResult(w io.Writer, result *aliyundeploy.DestroyOutput) {
	fmt.Fprintf(w, "Aliyun account: %s\n", result.AccountID)
	fmt.Fprintf(w, "Aliyun region:  %s\n", result.Region)
	fmt.Fprintf(w, "Action:         %s\n", result.Action)
	if result.StackName != "" {
		fmt.Fprintf(w, "Stack name:     %s\n", result.StackName)
	}
	if result.Status != "" {
		fmt.Fprintf(w, "Status:         %s\n", result.Status)
	}
}

func printAliyunDeployWarnings(w io.Writer, app *store.App, params map[string]string) {
	fmt.Fprintln(w, "Warning: This deploy creates billable Aliyun resources such as ECS, EIP, disks, and data transfer.")
	if app != nil && len(app.CostNotice) > 0 {
		for _, line := range app.CostNotice {
			line = strings.TrimSpace(line)
			if line != "" {
				fmt.Fprintf(w, "Warning: %s\n", line)
			}
		}
	}
	if allowedIP := params["AllowedIP"]; allowedIP == "0.0.0.0/0" || allowedIP == "" {
		fmt.Fprintln(w, "Warning: SSH is open to 0.0.0.0/0. Restrict access with --allowed-ip <your-ip>/32.")
	}
}

func stringsTrimOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
