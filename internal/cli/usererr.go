package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/aws/smithy-go"
)

// formatUserError returns a concise, actionable message for common CLI failures.
func formatUserError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())

	switch {
	case strings.Contains(msg, "no valid credential"):
		return "AWS credentials are not configured. Run: cloud-forge auth aws"
	case strings.Contains(msg, "load aws config"):
		return "Could not load AWS configuration. Run: cloud-forge auth aws or set AWS_PROFILE / AWS_ACCESS_KEY_ID."
	case strings.Contains(msg, "aws region is required"):
		return "AWS region is required. Pass --region or set AWS_REGION / AWS_DEFAULT_REGION."
	case strings.Contains(msg, "check aws identity"):
		return "Could not verify AWS credentials. Run: cloud-forge auth aws status"
	case strings.Contains(msg, "accessdenied") || strings.Contains(msg, "not authorized"):
		return formatAccessDenied(err)
	case strings.Contains(msg, "invalidamiid") || strings.Contains(msg, "invalid amis"):
		return "The AMI ID is not available in this region. Pass --latest-ami-id or --image-id with a valid AMI."
	case strings.Contains(msg, "does not exist") && strings.Contains(msg, "stack"):
		return formatStackMissing(err)
	case strings.Contains(msg, "browser sign-in"):
		return "AWS browser sign-in failed. Try: cloud-forge auth aws --no-browser"
	case strings.Contains(msg, "authorization"):
		return "AWS authorization failed. Check your credentials and try: cloud-forge auth aws status"
	case strings.Contains(msg, "aliyun credentials are not configured"):
		return "Aliyun credentials are not configured. Run: cloud-forge auth aliyun"
	case strings.Contains(msg, "check aliyun identity"):
		return "Could not verify Aliyun credentials. Run: cloud-forge auth aliyun status"
	case strings.Contains(msg, "validate ros template"):
		return formatAliyunDeployError(err)
	case strings.Contains(msg, "create ros stack") || strings.Contains(msg, "update ros stack"):
		return formatAliyunDeployError(err)
	default:
		return err.Error()
	}
}

func formatAccessDenied(err error) string {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		action := inferIAMAction(apiErr.ErrorMessage())
		if action != "" {
			return fmt.Sprintf("Missing AWS permission (%s). Update the IAM policy for your user or role.", action)
		}
	}
	return "AWS denied the request. Check IAM permissions for CloudFormation, EC2, and STS."
}

func inferIAMAction(message string) string {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "cloudformation:createstack"):
		return "cloudformation:CreateStack"
	case strings.Contains(lower, "cloudformation:updatestack"):
		return "cloudformation:UpdateStack"
	case strings.Contains(lower, "cloudformation:deletestack"):
		return "cloudformation:DeleteStack"
	case strings.Contains(lower, "cloudformation:describestacks"):
		return "cloudformation:DescribeStacks"
	case strings.Contains(lower, "ec2:importkeypair"):
		return "ec2:ImportKeyPair"
	case strings.Contains(lower, "ec2:runinstances"):
		return "ec2:RunInstances"
	default:
		return ""
	}
}

func formatStackMissing(err error) string {
	return "CloudFormation stack not found. Check the stack name and region, or deploy the app first."
}

func formatAliyunDeployError(err error) string {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "ramrole") || strings.Contains(msg, "not authorized to create"):
		return "ROS needs a RAM service role to create ECS resources. Create the ROS service role in the Aliyun console, then retry."
	case strings.Contains(msg, "accessdenied") || strings.Contains(msg, "forbidden"):
		return "Aliyun denied the request. Check RAM permissions for ROS, ECS, and VPC."
	default:
		return err.Error()
	}
}

func printUserError(w io.Writer, err error) {
	fmt.Fprintln(w, formatUserError(err))
}
