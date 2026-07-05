package aliyundeploy

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	ecs "github.com/alibabacloud-go/ecs-20140526/v4/client"
	vpc "github.com/alibabacloud-go/vpc-20160428/v2/client"
	"github.com/alibabacloud-go/tea/tea"
)

const (
	DefaultKeyPairName = "cloud-forge-default"
)

// DeployDefaultsRequest controls auto-discovery for Aliyun deploy parameters.
type DeployDefaultsRequest struct {
	VpcID       string
	VSwitchID   string
	KeyPairName string
	AutoVpc     bool
	AutoVSwitch bool
	AutoKey     bool
	DryRun      bool
}

// DeployDefaultsResult holds resolved network and key parameters.
type DeployDefaultsResult struct {
	VpcID       string
	VSwitchID   string
	KeyPairName string
	Messages    []string
}

type networkPair struct {
	VpcID     string
	VpcName   string
	VSwitchID string
	VSwitchName string
	ZoneID    string
}

func ResolveDeployDefaults(ctx context.Context, cfg Config, req DeployDefaultsRequest) (DeployDefaultsResult, error) {
	loaded, err := LoadConfig(cfg)
	if err != nil {
		return DeployDefaultsResult{}, err
	}

	result := DeployDefaultsResult{
		VpcID:       strings.TrimSpace(req.VpcID),
		VSwitchID:   strings.TrimSpace(req.VSwitchID),
		KeyPairName: strings.TrimSpace(req.KeyPairName),
	}

	if envVpc := strings.TrimSpace(os.Getenv("ALIYUN_VPC_ID")); result.VpcID == "" && envVpc != "" {
		result.VpcID = envVpc
	}
	if envVsw := strings.TrimSpace(os.Getenv("ALIYUN_VSWITCH_ID")); result.VSwitchID == "" && envVsw != "" {
		result.VSwitchID = envVsw
	}
	if envKey := strings.TrimSpace(os.Getenv("ALIYUN_KEY_NAME")); result.KeyPairName == "" && envKey != "" {
		result.KeyPairName = envKey
	}

	needNetwork := (req.AutoVpc && result.VpcID == "") ||
		(req.AutoVSwitch && result.VSwitchID == "") ||
		(result.VpcID != "" && req.AutoVSwitch && result.VSwitchID == "")

	if needNetwork {
		pairs, err := listNetworkPairs(ctx, loaded)
		if err != nil {
			return DeployDefaultsResult{}, err
		}
		vpcID, vswID, note, err := selectNetworkPair(pairs, result.VpcID, result.VSwitchID)
		if err != nil {
			return DeployDefaultsResult{}, err
		}
		if req.AutoVpc && result.VpcID == "" {
			result.VpcID = vpcID
		}
		if req.AutoVSwitch && result.VSwitchID == "" {
			result.VSwitchID = vswID
		}
		if note != "" {
			result.Messages = append(result.Messages, note)
		}
	}

	if req.AutoKey && result.KeyPairName == "" {
		keyOut, err := EnsureKeyPair(ctx, loaded, EnsureKeyPairInput{
			DryRun: req.DryRun,
		})
		if err != nil {
			return DeployDefaultsResult{}, err
		}
		result.KeyPairName = keyOut.KeyPairName
		if keyOut.Message != "" {
			result.Messages = append(result.Messages, keyOut.Message)
		}
	}

	if result.VpcID == "" {
		return DeployDefaultsResult{}, fmt.Errorf("missing Aliyun VpcId: pass --vpc-id, set ALIYUN_VPC_ID, or create a VPC in %s", loaded.Region)
	}
	if result.VSwitchID == "" {
		return DeployDefaultsResult{}, fmt.Errorf("missing Aliyun VSwitchId: pass --vswitch-id, set ALIYUN_VSWITCH_ID, or create a VSwitch in %s", loaded.Region)
	}
	if result.KeyPairName == "" {
		return DeployDefaultsResult{}, fmt.Errorf("missing Aliyun KeyPairName: pass --key, set ALIYUN_KEY_NAME, or run cloud-forge auth aliyun after creating a key pair in %s", loaded.Region)
	}

	return result, nil
}

func listNetworkPairs(ctx context.Context, cfg Config) ([]networkPair, error) {
	_ = ctx
	vpcClient, ecsClient, err := newVPCAndECSClients(cfg)
	if err != nil {
		return nil, err
	}

	vpcsOut, err := vpcClient.DescribeVpcs(&vpc.DescribeVpcsRequest{
		RegionId: tea.String(cfg.Region),
		PageSize: tea.Int32(50),
	})
	if err != nil {
		return nil, fmt.Errorf("list Aliyun VPCs in %s: %w", cfg.Region, err)
	}
	if vpcsOut.Body == nil || len(vpcsOut.Body.Vpcs.Vpc) == 0 {
		return nil, fmt.Errorf("no VPC found in %s; create one in the Aliyun console or pass --vpc-id", cfg.Region)
	}

	var pairs []networkPair
	for _, v := range vpcsOut.Body.Vpcs.Vpc {
		vpcID := tea.StringValue(v.VpcId)
		vswOut, err := vpcClient.DescribeVSwitches(&vpc.DescribeVSwitchesRequest{
			RegionId: tea.String(cfg.Region),
			VpcId:    tea.String(vpcID),
			PageSize: tea.Int32(50),
		})
		if err != nil {
			return nil, fmt.Errorf("list VSwitches for VPC %s: %w", vpcID, err)
		}
		if vswOut.Body == nil {
			continue
		}
		for _, s := range vswOut.Body.VSwitches.VSwitch {
			pairs = append(pairs, networkPair{
				VpcID:       vpcID,
				VpcName:     tea.StringValue(v.VpcName),
				VSwitchID:   tea.StringValue(s.VSwitchId),
				VSwitchName: tea.StringValue(s.VSwitchName),
				ZoneID:      tea.StringValue(s.ZoneId),
			})
		}
	}
	_ = ecsClient
	return pairs, nil
}

func selectNetworkPair(pairs []networkPair, requestedVpc, requestedVSwitch string) (vpcID, vswID, note string, err error) {
	if len(pairs) == 0 {
		return "", "", "", fmt.Errorf("no VSwitch found in any VPC; create a VSwitch in the target region or pass --vswitch-id")
	}

	filtered := pairs
	if requestedVpc != "" {
		var matches []networkPair
		for _, p := range pairs {
			if p.VpcID == requestedVpc {
				matches = append(matches, p)
			}
		}
		if len(matches) == 0 {
			return "", "", "", fmt.Errorf("no VSwitch found in VPC %s", requestedVpc)
		}
		filtered = matches
	}

	if requestedVSwitch != "" {
		for _, p := range filtered {
			if p.VSwitchID == requestedVSwitch {
				return p.VpcID, p.VSwitchID, "", nil
			}
		}
		return "", "", "", fmt.Errorf("VSwitch %s not found in VPC %s", requestedVSwitch, requestedVpc)
	}

	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].VpcID != filtered[j].VpcID {
			return filtered[i].VpcID < filtered[j].VpcID
		}
		return filtered[i].VSwitchID < filtered[j].VSwitchID
	})

	chosen := filtered[0]
	if len(filtered) > 1 {
		note = fmt.Sprintf("Using VPC %s and VSwitch %s (%d matches in region; pass --vpc-id/--vswitch-id to override)",
			chosen.VpcID, chosen.VSwitchID, len(filtered))
	} else {
		note = fmt.Sprintf("Using VPC %s and VSwitch %s", chosen.VpcID, chosen.VSwitchID)
	}
	return chosen.VpcID, chosen.VSwitchID, note, nil
}

func newVPCAndECSClients(cfg Config) (*vpc.Client, *ecs.Client, error) {
	openCfg := &openapi.Config{
		AccessKeyId:     tea.String(cfg.AccessKeyID),
		AccessKeySecret: tea.String(cfg.AccessKeySecret),
		RegionId:        tea.String(cfg.Region),
	}
	if cfg.SecurityToken != "" {
		openCfg.SecurityToken = tea.String(cfg.SecurityToken)
	}
	vpcClient, err := vpc.NewClient(openCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("create VPC client: %w", err)
	}
	ecsClient, err := ecs.NewClient(openCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("create ECS client: %w", err)
	}
	return vpcClient, ecsClient, nil
}
