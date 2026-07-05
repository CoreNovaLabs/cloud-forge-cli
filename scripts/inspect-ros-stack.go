//go:build ignore

package main

import (
	"fmt"
	"os"

	openapi "github.com/alibabacloud-go/darabonba-openapi/client"
	ros "github.com/alibabacloud-go/ros-20190910/v3/client"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/cloud-forge/cli/internal/aliyundeploy"
)

func main() {
	stackID := "ddd74db3-44ae-4a62-a9f5-2aaec1676661"
	if len(os.Args) > 1 {
		stackID = os.Args[1]
	}
	cfg, err := aliyundeploy.LoadConfig(aliyundeploy.Config{Region: "cn-hongkong"})
	if err != nil {
		panic(err)
	}
	openCfg := &openapi.Config{
		AccessKeyId:     tea.String(cfg.AccessKeyID),
		AccessKeySecret: tea.String(cfg.AccessKeySecret),
		RegionId:        tea.String(cfg.Region),
	}
	client, err := ros.NewClient(openCfg)
	if err != nil {
		panic(err)
	}
	stack, err := client.GetStack(&ros.GetStackRequest{
		RegionId: tea.String("cn-hongkong"),
		StackId:  tea.String(stackID),
	})
	if err != nil {
		panic(err)
	}
	fmt.Println("Parameters:")
	for _, p := range stack.Body.Parameters {
		fmt.Printf("  %s = %s\n", tea.StringValue(p.ParameterKey), tea.StringValue(p.ParameterValue))
	}
	fmt.Println("Outputs:")
	for _, o := range stack.Body.Outputs {
		fmt.Printf("  %v\n", o)
	}
	resources, err := client.ListStackResources(&ros.ListStackResourcesRequest{
		RegionId: tea.String("cn-hongkong"),
		StackId:  tea.String(stackID),
	})
	if err != nil {
		panic(err)
	}
	fmt.Println("Resources:")
	for _, r := range resources.Body.Resources {
		fmt.Printf("  %s %s %s\n", tea.StringValue(r.LogicalResourceId), tea.StringValue(r.ResourceType), tea.StringValue(r.Status))
	}
}
