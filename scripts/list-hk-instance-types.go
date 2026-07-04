//go:build ignore

package main

import (
	"fmt"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	ecs "github.com/alibabacloud-go/ecs-20140526/v4/client"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/cloud-forge/cli/internal/aliyundeploy"
)

func main() {
	cfg, _ := aliyundeploy.LoadConfig(aliyundeploy.Config{Region: "cn-hongkong"})
	openCfg := &openapi.Config{AccessKeyId: tea.String(cfg.AccessKeyID), AccessKeySecret: tea.String(cfg.AccessKeySecret), RegionId: tea.String(cfg.Region)}
	client, _ := ecs.NewClient(openCfg)
	for _, zone := range []string{"cn-hongkong-b", "cn-hongkong-c", "cn-hongkong-d"} {
		out, err := client.DescribeAvailableResource(&ecs.DescribeAvailableResourceRequest{
			RegionId:            tea.String("cn-hongkong"),
			DestinationResource: tea.String("InstanceType"),
			IoOptimized:         tea.String("optimized"),
			ZoneId:              tea.String(zone),
			InstanceChargeType:  tea.String("PostPaid"),
		})
		if err != nil {
			fmt.Printf("zone %s error: %v\n", zone, err)
			continue
		}
		fmt.Printf("=== %s ===\n", zone)
		if out.Body != nil {
			for _, z := range out.Body.AvailableZones.AvailableZone {
				for _, r := range z.AvailableResources.AvailableResource {
					for _, t := range r.SupportedResources.SupportedResource {
						if tea.StringValue(t.Status) == "Available" {
							fmt.Println(tea.StringValue(t.Value))
						}
					}
				}
			}
		}
	}
}
