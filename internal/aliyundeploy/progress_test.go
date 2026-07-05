package aliyundeploy

import "testing"

func TestShouldResolveEIPPublicIP(t *testing.T) {
	if shouldResolveEIPPublicIP("ALIYUN::VPC::EIP", "CREATE_COMPLETE") != true {
		t.Fatal("expected EIP CREATE_COMPLETE to resolve public IP")
	}
	if shouldResolveEIPPublicIP("ALIYUN::ECS::Instance", "CREATE_COMPLETE") {
		t.Fatal("expected ECS instance to skip public IP resolution")
	}
	if shouldResolveEIPPublicIP("ALIYUN::VPC::EIP", "CREATE_IN_PROGRESS") {
		t.Fatal("expected in-progress EIP to skip public IP resolution")
	}
}

func TestResourceAttributeString(t *testing.T) {
	ip := resourceAttributeString([]map[string]interface{}{
		{"AllocationId": "eip-123"},
		{"EipAddress": "203.0.113.10"},
	}, "EipAddress")
	if ip != "203.0.113.10" {
		t.Fatalf("unexpected ip: %q", ip)
	}
}
