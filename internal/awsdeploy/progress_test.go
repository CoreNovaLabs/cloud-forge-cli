package awsdeploy

import (
	"testing"

	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
)

func TestShouldResolveAWSEIPPublicIP(t *testing.T) {
	if !shouldResolveAWSEIPPublicIP("AWS::EC2::EIP", string(cfntypes.ResourceStatusCreateComplete)) {
		t.Fatal("expected EIP CREATE_COMPLETE to resolve public IP")
	}
	if !shouldResolveAWSEIPPublicIP("AWS::EC2::EIP", string(cfntypes.ResourceStatusUpdateComplete)) {
		t.Fatal("expected EIP UPDATE_COMPLETE to resolve public IP")
	}
	if shouldResolveAWSEIPPublicIP("AWS::EC2::Instance", string(cfntypes.ResourceStatusCreateComplete)) {
		t.Fatal("expected EC2 instance to skip public IP resolution")
	}
	if shouldResolveAWSEIPPublicIP("AWS::EC2::EIP", string(cfntypes.ResourceStatusCreateInProgress)) {
		t.Fatal("expected in-progress EIP to skip public IP resolution")
	}
}
