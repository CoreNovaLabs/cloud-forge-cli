package awsdeploy

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
)

const DefaultTimeout = 30 * time.Minute

type Config struct {
	Region  string
	Profile string
}

type Deployer struct {
	region string
	cfn    *cloudformation.Client
	sts    *sts.Client
}

type DeployInput struct {
	AppID        string
	AppVersion   string
	StackName    string
	TemplateBody string
	Parameters   map[string]string
	DryRun       bool
	Wait         bool
	Timeout      time.Duration
	Progress     func(StackProgressEvent)
}

type DeployOutput struct {
	Action    string
	Region    string
	AccountID string
	StackName string
	StackID   string
	Status    string
	Outputs   map[string]string
}

type StackProgressEvent struct {
	Timestamp            time.Time
	LogicalResourceID    string
	ResourceType         string
	ResourceStatus       string
	ResourceStatusReason string
}

func New(ctx context.Context, cfg Config) (*Deployer, error) {
	awsCfg, err := loadAWSConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return &Deployer{
		region: awsCfg.Region,
		cfn:    cloudformation.NewFromConfig(awsCfg),
		sts:    sts.NewFromConfig(awsCfg),
	}, nil
}

func loadAWSConfig(ctx context.Context, cfg Config) (awssdk.Config, error) {
	var options []func(*config.LoadOptions) error
	if strings.TrimSpace(cfg.Region) != "" {
		options = append(options, config.WithRegion(cfg.Region))
	}
	if cfg.Profile != "" {
		options = append(options, config.WithSharedConfigProfile(cfg.Profile))
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, options...)
	if err != nil {
		return awssdk.Config{}, fmt.Errorf("load AWS config: %w", err)
	}
	if awsCfg.Region == "" {
		return awssdk.Config{}, fmt.Errorf("aws region is required; set --region, AWS_REGION, AWS_DEFAULT_REGION, or a profile region")
	}
	return awsCfg, nil
}

func (d *Deployer) Deploy(ctx context.Context, input DeployInput) (*DeployOutput, error) {
	if err := validateInput(input); err != nil {
		return nil, err
	}
	if input.Timeout == 0 {
		input.Timeout = DefaultTimeout
	}

	identity, err := d.sts.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return nil, fmt.Errorf("check AWS identity: %w", err)
	}

	if _, err := d.cfn.ValidateTemplate(ctx, &cloudformation.ValidateTemplateInput{
		TemplateBody: awssdk.String(input.TemplateBody),
	}); err != nil {
		return nil, fmt.Errorf("validate CloudFormation template: %w", err)
	}

	out := &DeployOutput{
		Action:    "validated",
		Region:    d.region,
		AccountID: awssdk.ToString(identity.Account),
		StackName: input.StackName,
		Outputs:   map[string]string{},
	}
	if input.DryRun {
		return out, nil
	}

	existing, err := d.describeStack(ctx, input.StackName)
	if err != nil {
		return nil, err
	}

	params := cloudFormationParameters(input.Parameters)
	if existing == nil {
		operationStarted := time.Now().Add(-10 * time.Second)
		stackID, err := d.createStack(ctx, input, params)
		if err != nil {
			return nil, err
		}
		out.Action = "created"
		out.StackID = stackID
		if input.Wait {
			if err := d.waitCreate(ctx, input.StackName, input.Timeout, operationStarted, input.Progress); err != nil {
				return nil, err
			}
			return d.finalOutput(ctx, out, input.StackName)
		}
		out.Status = "CREATE_IN_PROGRESS"
		return out, nil
	}

	out.StackID = awssdk.ToString(existing.StackId)
	operationStarted := time.Now().Add(-10 * time.Second)
	if err := d.updateStack(ctx, input, params); err != nil {
		if isNoUpdatesError(err) {
			out.Action = "unchanged"
			return d.finalOutput(ctx, out, input.StackName)
		}
		return nil, err
	}
	out.Action = "updated"
	if input.Wait {
		if err := d.waitUpdate(ctx, input.StackName, input.Timeout, operationStarted, input.Progress); err != nil {
			return nil, err
		}
		return d.finalOutput(ctx, out, input.StackName)
	}
	out.Status = "UPDATE_IN_PROGRESS"
	return out, nil
}

func validateInput(input DeployInput) error {
	if strings.TrimSpace(input.StackName) == "" {
		return fmt.Errorf("stack name is required")
	}
	if strings.TrimSpace(input.TemplateBody) == "" {
		return fmt.Errorf("template body is required")
	}
	return nil
}

func (d *Deployer) createStack(ctx context.Context, input DeployInput, params []cfntypes.Parameter) (string, error) {
	out, err := d.cfn.CreateStack(ctx, &cloudformation.CreateStackInput{
		StackName:    awssdk.String(input.StackName),
		TemplateBody: awssdk.String(input.TemplateBody),
		Parameters:   params,
		Capabilities: defaultCapabilities(),
		Tags:         cloudForgeTags(input.AppID, input.AppVersion),
	})
	if err != nil {
		return "", fmt.Errorf("create CloudFormation stack %q: %w", input.StackName, err)
	}
	return awssdk.ToString(out.StackId), nil
}

func (d *Deployer) updateStack(ctx context.Context, input DeployInput, params []cfntypes.Parameter) error {
	_, err := d.cfn.UpdateStack(ctx, &cloudformation.UpdateStackInput{
		StackName:    awssdk.String(input.StackName),
		TemplateBody: awssdk.String(input.TemplateBody),
		Parameters:   params,
		Capabilities: defaultCapabilities(),
		Tags:         cloudForgeTags(input.AppID, input.AppVersion),
	})
	if err != nil {
		return fmt.Errorf("update CloudFormation stack %q: %w", input.StackName, err)
	}
	return nil
}

func (d *Deployer) describeStack(ctx context.Context, stackName string) (*cfntypes.Stack, error) {
	out, err := d.cfn.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
		StackName: awssdk.String(stackName),
	})
	if err != nil {
		if isStackNotFoundError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("describe CloudFormation stack %q: %w", stackName, err)
	}
	if len(out.Stacks) == 0 {
		return nil, nil
	}
	return &out.Stacks[0], nil
}

func (d *Deployer) waitCreate(ctx context.Context, stackName string, timeout time.Duration, since time.Time, progress func(StackProgressEvent)) error {
	return d.waitStack(ctx, stackName, timeout, since, progress, "create", map[string]bool{
		string(cfntypes.StackStatusCreateComplete): true,
	})
}

func (d *Deployer) waitUpdate(ctx context.Context, stackName string, timeout time.Duration, since time.Time, progress func(StackProgressEvent)) error {
	return d.waitStack(ctx, stackName, timeout, since, progress, "update", map[string]bool{
		string(cfntypes.StackStatusUpdateComplete): true,
	})
}

func (d *Deployer) waitStack(ctx context.Context, stackName string, timeout time.Duration, since time.Time, progress func(StackProgressEvent), operation string, success map[string]bool) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	seenEvents := map[string]bool{}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		if err := d.emitStackEvents(waitCtx, stackName, since, seenEvents, progress); err != nil {
			return err
		}
		stack, err := d.describeStack(waitCtx, stackName)
		if err != nil {
			return err
		}
		if stack != nil {
			status := string(stack.StackStatus)
			if success[status] {
				if err := d.emitStackEvents(waitCtx, stackName, since, seenEvents, progress); err != nil {
					return err
				}
				return nil
			}
			if isFailedStackStatus(status) {
				if err := d.emitStackEvents(waitCtx, stackName, since, seenEvents, progress); err != nil {
					return err
				}
				reason := awssdk.ToString(stack.StackStatusReason)
				if reason != "" {
					return fmt.Errorf("wait for CloudFormation stack %q %s: stack status %s: %s", stackName, operation, status, reason)
				}
				return fmt.Errorf("wait for CloudFormation stack %q %s: stack status %s", stackName, operation, status)
			}
		}

		select {
		case <-waitCtx.Done():
			if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
				return fmt.Errorf("wait for CloudFormation stack %q %s: timed out after %s", stackName, operation, timeout)
			}
			return waitCtx.Err()
		case <-ticker.C:
		}
	}
}

func (d *Deployer) emitStackEvents(ctx context.Context, stackName string, since time.Time, seen map[string]bool, progress func(StackProgressEvent)) error {
	if progress == nil {
		return nil
	}
	out, err := d.cfn.DescribeStackEvents(ctx, &cloudformation.DescribeStackEventsInput{
		StackName: awssdk.String(stackName),
	})
	if err != nil {
		if isStackNotFoundError(err) {
			return nil
		}
		return fmt.Errorf("describe CloudFormation stack events %q: %w", stackName, err)
	}

	events := make([]cfntypes.StackEvent, 0, len(out.StackEvents))
	for _, event := range out.StackEvents {
		eventID := awssdk.ToString(event.EventId)
		if eventID == "" || seen[eventID] {
			continue
		}
		if event.Timestamp == nil || event.Timestamp.Before(since) {
			continue
		}
		events = append(events, event)
		seen[eventID] = true
	}
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		progress(StackProgressEvent{
			Timestamp:            awssdk.ToTime(event.Timestamp),
			LogicalResourceID:    awssdk.ToString(event.LogicalResourceId),
			ResourceType:         awssdk.ToString(event.ResourceType),
			ResourceStatus:       string(event.ResourceStatus),
			ResourceStatusReason: awssdk.ToString(event.ResourceStatusReason),
		})
	}
	return nil
}

func (d *Deployer) finalOutput(ctx context.Context, out *DeployOutput, stackName string) (*DeployOutput, error) {
	stack, err := d.describeStack(ctx, stackName)
	if err != nil {
		return nil, err
	}
	if stack == nil {
		return out, nil
	}
	out.StackID = awssdk.ToString(stack.StackId)
	out.Status = string(stack.StackStatus)
	out.Outputs = stackOutputs(stack.Outputs)
	return out, nil
}

func cloudFormationParameters(values map[string]string) []cfntypes.Parameter {
	keys := make([]string, 0, len(values))
	for key, value := range values {
		if strings.TrimSpace(key) == "" || value == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	params := make([]cfntypes.Parameter, 0, len(keys))
	for _, key := range keys {
		value := values[key]
		params = append(params, cfntypes.Parameter{
			ParameterKey:   awssdk.String(key),
			ParameterValue: awssdk.String(value),
		})
	}
	return params
}

func stackOutputs(outputs []cfntypes.Output) map[string]string {
	out := make(map[string]string, len(outputs))
	for _, item := range outputs {
		key := awssdk.ToString(item.OutputKey)
		if key == "" {
			continue
		}
		out[key] = awssdk.ToString(item.OutputValue)
	}
	return out
}

func cloudForgeTags(appID, appVersion string) []cfntypes.Tag {
	tags := []cfntypes.Tag{
		{Key: awssdk.String("cloud-forge:managed-by"), Value: awssdk.String("cloud-forge-cli")},
		{Key: awssdk.String("cloud-forge:app"), Value: awssdk.String(appID)},
	}
	if appVersion != "" {
		tags = append(tags, cfntypes.Tag{
			Key:   awssdk.String("cloud-forge:app-version"),
			Value: awssdk.String(appVersion),
		})
	}
	return tags
}

func defaultCapabilities() []cfntypes.Capability {
	return []cfntypes.Capability{
		cfntypes.CapabilityCapabilityIam,
		cfntypes.CapabilityCapabilityNamedIam,
		cfntypes.CapabilityCapabilityAutoExpand,
	}
}

func isStackNotFoundError(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.ErrorCode() == "ValidationError" && strings.Contains(apiErr.ErrorMessage(), "does not exist")
}

func isNoUpdatesError(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.ErrorCode() == "ValidationError" && strings.Contains(apiErr.ErrorMessage(), "No updates are to be performed")
}

func isFailedStackStatus(status string) bool {
	switch status {
	case string(cfntypes.StackStatusCreateFailed),
		string(cfntypes.StackStatusRollbackInProgress),
		string(cfntypes.StackStatusRollbackFailed),
		string(cfntypes.StackStatusRollbackComplete),
		string(cfntypes.StackStatusDeleteFailed),
		string(cfntypes.StackStatusUpdateFailed),
		string(cfntypes.StackStatusUpdateRollbackInProgress),
		string(cfntypes.StackStatusUpdateRollbackFailed),
		string(cfntypes.StackStatusUpdateRollbackCompleteCleanupInProgress),
		string(cfntypes.StackStatusUpdateRollbackComplete),
		string(cfntypes.StackStatusImportRollbackInProgress),
		string(cfntypes.StackStatusImportRollbackFailed),
		string(cfntypes.StackStatusImportRollbackComplete):
		return true
	default:
		return false
	}
}
