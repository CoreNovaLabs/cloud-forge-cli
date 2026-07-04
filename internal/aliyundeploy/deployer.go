package aliyundeploy

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	openapi "github.com/alibabacloud-go/darabonba-openapi/client"
	ros "github.com/alibabacloud-go/ros-20190910/v3/client"
	sts "github.com/alibabacloud-go/sts-20150401/client"
	"github.com/alibabacloud-go/tea/tea"
)

const DefaultTimeout = 30 * time.Minute

type Deployer struct {
	region string
	ros    *ros.Client
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

type DestroyInput struct {
	StackName string
	Wait      bool
	Timeout   time.Duration
	Progress  func(StackProgressEvent)
}

type DestroyOutput struct {
	Action    string
	Region    string
	AccountID string
	StackName string
	Status    string
}

type StackProgressEvent struct {
	Timestamp            time.Time
	LogicalResourceID    string
	ResourceType         string
	ResourceStatus       string
	ResourceStatusReason string
}

func New(ctx context.Context, cfg Config) (*Deployer, error) {
	loaded, err := LoadConfig(cfg)
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

	rosClient, err := ros.NewClient(openCfg)
	if err != nil {
		return nil, fmt.Errorf("create ROS client: %w", err)
	}
	stsClient, err := sts.NewClient(openCfg)
	if err != nil {
		return nil, fmt.Errorf("create STS client: %w", err)
	}

	return &Deployer{
		region: loaded.Region,
		ros:    rosClient,
		sts:    stsClient,
	}, nil
}

func (d *Deployer) Deploy(ctx context.Context, input DeployInput) (*DeployOutput, error) {
	if err := validateInput(input); err != nil {
		return nil, err
	}
	if input.Timeout == 0 {
		input.Timeout = DefaultTimeout
	}

	identity, err := d.sts.GetCallerIdentity()
	if err != nil {
		return nil, fmt.Errorf("check aliyun identity: %w", err)
	}

	if _, err := d.ros.ValidateTemplate(&ros.ValidateTemplateRequest{
		TemplateBody: tea.String(input.TemplateBody),
	}); err != nil {
		return nil, fmt.Errorf("validate ROS template: %w", err)
	}

	out := &DeployOutput{
		Action:    "validated",
		Region:    d.region,
		AccountID: tea.StringValue(identity.Body.AccountId),
		StackName: input.StackName,
		Outputs:   map[string]string{},
	}
	if input.DryRun {
		return out, nil
	}

	existing, err := d.getStack(input.StackName)
	if err != nil {
		return nil, err
	}

	params := rosParameters(input.Parameters)
	if existing == nil {
		operationStarted := time.Now().Add(-10 * time.Second)
		createOut, err := d.ros.CreateStack(&ros.CreateStackRequest{
			RegionId:     tea.String(d.region),
			StackName:    tea.String(input.StackName),
			TemplateBody: tea.String(input.TemplateBody),
			Parameters:   params,
			Tags:         cloudForgeTags(input.AppID, input.AppVersion),
		})
		if err != nil {
			return nil, fmt.Errorf("create ROS stack %q: %w", input.StackName, err)
		}
		out.Action = "created"
		out.StackID = tea.StringValue(createOut.Body.StackId)
		if input.Wait {
			if err := d.waitCreate(ctx, input.StackName, input.Timeout, operationStarted, input.Progress); err != nil {
				return nil, err
			}
			return d.finalOutput(out, input.StackName)
		}
		out.Status = "CREATE_IN_PROGRESS"
		return out, nil
	}

	out.StackID = tea.StringValue(existing.StackId)
	operationStarted := time.Now().Add(-10 * time.Second)
	if _, err := d.ros.UpdateStack(&ros.UpdateStackRequest{
		RegionId:     tea.String(d.region),
		StackId:      existing.StackId,
		TemplateBody: tea.String(input.TemplateBody),
		Parameters:   updateParameters(input.Parameters),
		Tags:         updateTags(input.AppID, input.AppVersion),
	}); err != nil {
		if isNoUpdatesError(err) {
			out.Action = "unchanged"
			return d.finalOutput(out, input.StackName)
		}
		return nil, fmt.Errorf("update ROS stack %q: %w", input.StackName, err)
	}

	out.Action = "updated"
	if input.Wait {
		if err := d.waitUpdate(ctx, input.StackName, input.Timeout, operationStarted, input.Progress); err != nil {
			return nil, err
		}
		return d.finalOutput(out, input.StackName)
	}
	out.Status = "UPDATE_IN_PROGRESS"
	return out, nil
}

func (d *Deployer) Destroy(ctx context.Context, input DestroyInput) (*DestroyOutput, error) {
	stackName := strings.TrimSpace(input.StackName)
	if stackName == "" {
		return nil, fmt.Errorf("stack name is required")
	}
	if input.Timeout == 0 {
		input.Timeout = DefaultTimeout
	}

	identity, err := d.sts.GetCallerIdentity()
	if err != nil {
		return nil, fmt.Errorf("check aliyun identity: %w", err)
	}

	stack, err := d.getStack(stackName)
	if err != nil {
		return nil, err
	}
	if stack == nil {
		return nil, fmt.Errorf("describe ROS stack %q: stack does not exist", stackName)
	}

	out := &DestroyOutput{
		Action:    "deleted",
		Region:    d.region,
		AccountID: tea.StringValue(identity.Body.AccountId),
		StackName: stackName,
		Status:    tea.StringValue(stack.Status),
	}

	operationStarted := time.Now().Add(-10 * time.Second)
	if _, err := d.ros.DeleteStack(&ros.DeleteStackRequest{
		RegionId: tea.String(d.region),
		StackId:  stack.StackId,
	}); err != nil {
		return nil, fmt.Errorf("delete ROS stack %q: %w", stackName, err)
	}

	if !input.Wait {
		out.Status = "DELETE_IN_PROGRESS"
		return out, nil
	}
	if err := d.waitDelete(ctx, stackName, input.Timeout, operationStarted, input.Progress); err != nil {
		return nil, err
	}
	out.Status = "DELETE_COMPLETE"
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

func (d *Deployer) getStack(stackName string) (*ros.GetStackResponseBody, error) {
	listOut, err := d.ros.ListStacks(&ros.ListStacksRequest{
		RegionId:  tea.String(d.region),
		StackName: []*string{tea.String(stackName)},
		PageSize:  tea.Int64(1),
	})
	if err != nil {
		return nil, fmt.Errorf("list ROS stack %q: %w", stackName, err)
	}
	if listOut.Body == nil || len(listOut.Body.Stacks) == 0 {
		return nil, nil
	}
	stackID := listOut.Body.Stacks[0].StackId
	if stackID == nil || tea.StringValue(stackID) == "" {
		return nil, nil
	}

	out, err := d.ros.GetStack(&ros.GetStackRequest{
		RegionId: tea.String(d.region),
		StackId:  stackID,
	})
	if err != nil {
		if isStackNotFoundError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get ROS stack %q: %w", stackName, err)
	}
	return out.Body, nil
}

func (d *Deployer) stackIDForName(stackName string) (*string, error) {
	stack, err := d.getStack(stackName)
	if err != nil {
		return nil, err
	}
	if stack == nil {
		return nil, nil
	}
	return stack.StackId, nil
}

func (d *Deployer) waitCreate(ctx context.Context, stackName string, timeout time.Duration, since time.Time, progress func(StackProgressEvent)) error {
	return d.waitStack(ctx, stackName, timeout, since, progress, "create", map[string]bool{
		"CREATE_COMPLETE": true,
	})
}

func (d *Deployer) waitUpdate(ctx context.Context, stackName string, timeout time.Duration, since time.Time, progress func(StackProgressEvent)) error {
	return d.waitStack(ctx, stackName, timeout, since, progress, "update", map[string]bool{
		"UPDATE_COMPLETE": true,
	})
}

func (d *Deployer) waitDelete(ctx context.Context, stackName string, timeout time.Duration, since time.Time, progress func(StackProgressEvent)) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	seenEvents := map[string]bool{}
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		if err := d.emitStackEvents(waitCtx, stackName, since, seenEvents, progress); err != nil {
			return err
		}
		stack, err := d.getStack(stackName)
		if err != nil {
			return err
		}
		if stack == nil {
			return nil
		}
		status := tea.StringValue(stack.Status)
		if status == "DELETE_COMPLETE" {
			return nil
		}
		if isFailedStackStatus(status) {
			reason := tea.StringValue(stack.StatusReason)
			if reason != "" {
				return fmt.Errorf("wait for ROS stack %q delete: stack status %s: %s", stackName, status, reason)
			}
			return fmt.Errorf("wait for ROS stack %q delete: stack status %s", stackName, status)
		}

		select {
		case <-waitCtx.Done():
			if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
				return fmt.Errorf("wait for ROS stack %q delete: timed out after %s", stackName, timeout)
			}
			return waitCtx.Err()
		case <-ticker.C:
		}
	}
}

func (d *Deployer) waitStack(ctx context.Context, stackName string, timeout time.Duration, since time.Time, progress func(StackProgressEvent), operation string, success map[string]bool) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	seenEvents := map[string]bool{}
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		if err := d.emitStackEvents(waitCtx, stackName, since, seenEvents, progress); err != nil {
			return err
		}
		stack, err := d.getStack(stackName)
		if err != nil {
			return err
		}
		if stack != nil {
			status := tea.StringValue(stack.Status)
			if success[status] {
				return nil
			}
			if isFailedStackStatus(status) {
				reason := tea.StringValue(stack.StatusReason)
				if reason != "" {
					return fmt.Errorf("wait for ROS stack %q %s: stack status %s: %s", stackName, operation, status, reason)
				}
				return fmt.Errorf("wait for ROS stack %q %s: stack status %s", stackName, operation, status)
			}
		}

		select {
		case <-waitCtx.Done():
			if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
				return fmt.Errorf("wait for ROS stack %q %s: timed out after %s", stackName, operation, timeout)
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
	if err := ctx.Err(); err != nil {
		return err
	}

	stackID, err := d.stackIDForName(stackName)
	if err != nil {
		return err
	}
	if stackID == nil {
		return nil
	}

	out, err := d.ros.ListStackEvents(&ros.ListStackEventsRequest{
		RegionId: tea.String(d.region),
		StackId:  stackID,
	})
	if err != nil {
		if isStackNotFoundError(err) {
			return nil
		}
		return fmt.Errorf("list ROS stack events %q: %w", stackName, err)
	}
	if out.Body == nil || out.Body.Events == nil {
		return nil
	}

	events := make([]*ros.ListStackEventsResponseBodyEvents, 0, len(out.Body.Events))
	for _, event := range out.Body.Events {
		eventID := tea.StringValue(event.EventId)
		if eventID == "" || seen[eventID] {
			continue
		}
		if ts := tea.StringValue(event.CreateTime); ts != "" {
			parsed, parseErr := time.Parse(time.RFC3339, ts)
			if parseErr == nil && parsed.Before(since) {
				continue
			}
		}
		events = append(events, event)
		seen[eventID] = true
	}
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		var ts time.Time
		if raw := tea.StringValue(event.CreateTime); raw != "" {
			if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
				ts = parsed
			}
		}
		progress(StackProgressEvent{
			Timestamp:            ts,
			LogicalResourceID:    tea.StringValue(event.LogicalResourceId),
			ResourceType:         tea.StringValue(event.ResourceType),
			ResourceStatus:       tea.StringValue(event.Status),
			ResourceStatusReason: tea.StringValue(event.StatusReason),
		})
	}
	return nil
}

func (d *Deployer) finalOutput(out *DeployOutput, stackName string) (*DeployOutput, error) {
	stack, err := d.getStack(stackName)
	if err != nil {
		return nil, err
	}
	if stack == nil {
		return out, nil
	}
	out.StackID = tea.StringValue(stack.StackId)
	out.Status = tea.StringValue(stack.Status)
	out.Outputs = stackOutputs(stack.Outputs)
	return out, nil
}

func rosParameters(values map[string]string) []*ros.CreateStackRequestParameters {
	keys := make([]string, 0, len(values))
	for key, value := range values {
		if strings.TrimSpace(key) == "" || value == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	params := make([]*ros.CreateStackRequestParameters, 0, len(keys))
	for _, key := range keys {
		value := values[key]
		params = append(params, &ros.CreateStackRequestParameters{
			ParameterKey:   tea.String(key),
			ParameterValue: tea.String(value),
		})
	}
	return params
}

func stackOutputs(outputs []map[string]interface{}) map[string]string {
	out := map[string]string{}
	for _, item := range outputs {
		key, _ := item["OutputKey"].(string)
		if key == "" {
			continue
		}
		switch value := item["OutputValue"].(type) {
		case string:
			out[key] = value
		default:
			out[key] = fmt.Sprint(value)
		}
	}
	return out
}

func cloudForgeTags(appID, appVersion string) []*ros.CreateStackRequestTags {
	tags := []*ros.CreateStackRequestTags{
		{Key: tea.String("cloud-forge:managed-by"), Value: tea.String("cloud-forge-cli")},
		{Key: tea.String("cloud-forge:app"), Value: tea.String(appID)},
	}
	if appVersion != "" {
		tags = append(tags, &ros.CreateStackRequestTags{
			Key:   tea.String("cloud-forge:app-version"),
			Value: tea.String(appVersion),
		})
	}
	return tags
}

func updateParameters(values map[string]string) []*ros.UpdateStackRequestParameters {
	keys := make([]string, 0, len(values))
	for key, value := range values {
		if strings.TrimSpace(key) == "" || value == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	params := make([]*ros.UpdateStackRequestParameters, 0, len(keys))
	for _, key := range keys {
		value := values[key]
		params = append(params, &ros.UpdateStackRequestParameters{
			ParameterKey:   tea.String(key),
			ParameterValue: tea.String(value),
		})
	}
	return params
}

func updateTags(appID, appVersion string) []*ros.UpdateStackRequestTags {
	tags := []*ros.UpdateStackRequestTags{
		{Key: tea.String("cloud-forge:managed-by"), Value: tea.String("cloud-forge-cli")},
		{Key: tea.String("cloud-forge:app"), Value: tea.String(appID)},
	}
	if appVersion != "" {
		tags = append(tags, &ros.UpdateStackRequestTags{
			Key:   tea.String("cloud-forge:app-version"),
			Value: tea.String(appVersion),
		})
	}
	return tags
}

func isStackNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "stacknotfound") ||
		strings.Contains(msg, "stack not found") ||
		strings.Contains(msg, "does not exist")
}

func isNoUpdatesError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no updates") || strings.Contains(msg, "not update")
}

func isFailedStackStatus(status string) bool {
	switch status {
	case "CREATE_FAILED",
		"ROLLBACK_IN_PROGRESS",
		"ROLLBACK_FAILED",
		"ROLLBACK_COMPLETE",
		"DELETE_FAILED",
		"UPDATE_FAILED",
		"UPDATE_ROLLBACK_IN_PROGRESS",
		"UPDATE_ROLLBACK_FAILED",
		"UPDATE_ROLLBACK_COMPLETE":
		return true
	default:
		return false
	}
}
