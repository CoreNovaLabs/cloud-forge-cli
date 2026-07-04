package main

import (
	"fmt"
	"os"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	ecs "github.com/alibabacloud-go/ecs-20140526/v4/client"
	vpc "github.com/alibabacloud-go/vpc-20160428/v2/client"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/cloud-forge/cli/internal/aliyundeploy"
)

func main() {
	cfg, err := aliyundeploy.LoadConfig(aliyundeploy.Config{Region: "cn-hongkong"})
	if err != nil {
		panic(err)
	}
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
		panic(err)
	}
	vpcs, err := vpcClient.DescribeVpcs(&vpc.DescribeVpcsRequest{RegionId: tea.String(cfg.Region), PageSize: tea.Int32(10)})
	if err != nil {
		fmt.Fprintf(os.Stderr, "DescribeVpcs error: %v\n", err)
		os.Exit(1)
	}
	if vpcs.Body == nil || len(vpcs.Body.Vpcs.Vpc) == 0 {
		fmt.Println("NO_VPC")
		return
	}
	for _, v := range vpcs.Body.Vpcs.Vpc {
		vpcId := tea.StringValue(v.VpcId)
		fmt.Printf("VPC %s %s\n", vpcId, tea.StringValue(v.VpcName))
		vs, err := vpcClient.DescribeVSwitches(&vpc.DescribeVSwitchesRequest{RegionId: tea.String(cfg.Region), VpcId: tea.String(vpcId), PageSize: tea.Int32(10)})
		if err != nil {
			fmt.Fprintf(os.Stderr, "  VSwitch error: %v\n", err)
			continue
		}
		if vs.Body != nil {
			for _, s := range vs.Body.VSwitches.VSwitch {
				fmt.Printf("  VSW %s %s %s\n", tea.StringValue(s.VSwitchId), tea.StringValue(s.VSwitchName), tea.StringValue(s.ZoneId))
			}
		}
	}

	ecsClient, err := ecs.NewClient(openCfg)
	if err != nil {
		panic(err)
	}
	keys, err := ecsClient.DescribeKeyPairs(&ecs.DescribeKeyPairsRequest{RegionId: tea.String(cfg.Region), PageSize: tea.Int32(20)})
	if err != nil {
		fmt.Fprintf(os.Stderr, "DescribeKeyPairs error: %v\n", err)
		os.Exit(1)
	}
	if keys.Body != nil {
		for _, k := range keys.Body.KeyPairs.KeyPair {
			fmt.Printf("KEY %s\n", tea.StringValue(k.KeyPairName))
		}
	}
}
