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
	name := "cloud-forge-hello-nginx"
	if len(os.Args) > 1 {
		name = os.Args[1]
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
	out, err := client.ListStacks(&ros.ListStacksRequest{
		RegionId:  tea.String(cfg.Region),
		StackName: []*string{tea.String(name)},
		PageSize:  tea.Int64(10),
	})
	if err != nil {
		panic(err)
	}
	if out.Body == nil || len(out.Body.Stacks) == 0 {
		fmt.Println("NO_STACK")
		return
	}
	for _, s := range out.Body.Stacks {
		fmt.Printf("StackId=%s Status=%s\n", tea.StringValue(s.StackId), tea.StringValue(s.Status))
	}
}
