package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloud-forge/cli/internal/aliyunauth"
	"github.com/cloud-forge/cli/internal/awsauth"
)

var checkAWSCredentials = func(ctx context.Context, profile, region string) error {
	if _, err := awsauth.CheckIdentity(ctx, profile, region); err != nil {
		return newCredentialPreflightError("AWS", "aws", profile, region)
	}
	return nil
}

var checkAliyunCredentials = func(ctx context.Context, profile, region string) error {
	if _, err := aliyunauth.CheckIdentity(ctx, aliyunauth.Config{Profile: profile, Region: region}); err != nil {
		return newCredentialPreflightError("Aliyun", "aliyun", profile, region)
	}
	return nil
}

func newCredentialPreflightError(cloudName, authCloud, profile, region string) error {
	return fmt.Errorf("%s credentials are not configured or could not be verified.\n\nRun:\n  %s\n\nThen retry the browser launch or copied deploy command.", cloudName, cloudAuthCommand(authCloud, profile, region))
}

func cloudAuthCommand(cloud, profile, region string) string {
	parts := []string{"cloud-forge", "auth", cloud}
	if strings.TrimSpace(profile) != "" {
		parts = append(parts, "--profile", strings.TrimSpace(profile))
	}
	if cloud == "aws" && strings.TrimSpace(region) != "" && strings.TrimSpace(region) != defaultAWSRegion {
		parts = append(parts, "--region", strings.TrimSpace(region))
	}
	if cloud == "aliyun" && strings.TrimSpace(region) != "" && strings.TrimSpace(region) != defaultAliyunRegion {
		parts = append(parts, "--region", strings.TrimSpace(region))
	}
	return shellCommand(parts)
}
