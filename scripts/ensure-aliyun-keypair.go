//go:build ignore

package main

import (
	"fmt"
	"os"
	"strings"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	ecs "github.com/alibabacloud-go/ecs-20140526/v4/client"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/cloud-forge/cli/internal/aliyundeploy"
)

func main() {
	name := "cloud-forge-hello"
	if len(os.Args) > 1 {
		name = os.Args[1]
	}
	pubPath := os.ExpandEnv("$HOME/.ssh/id_ed25519.pub")
	if _, err := os.Stat(pubPath); err != nil {
		pubPath = os.ExpandEnv("$HOME/.ssh/id_rsa.pub")
	}
	pub, err := os.ReadFile(pubPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "no local ssh public key found\n")
		os.Exit(1)
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
	client, err := ecs.NewClient(openCfg)
	if err != nil {
		panic(err)
	}

	keys, err := client.DescribeKeyPairs(&ecs.DescribeKeyPairsRequest{
		RegionId:     tea.String(cfg.Region),
		KeyPairName:  tea.String(name),
	})
	if err == nil && keys.Body != nil && len(keys.Body.KeyPairs.KeyPair) > 0 {
		fmt.Printf("KeyPair already exists: %s\n", name)
		return
	}

	_, err = client.ImportKeyPair(&ecs.ImportKeyPairRequest{
		RegionId:       tea.String(cfg.Region),
		KeyPairName:    tea.String(name),
		PublicKeyBody:  tea.String(strings.TrimSpace(string(pub))),
	})
	if err != nil {
		panic(err)
	}
	fmt.Printf("Imported KeyPair: %s\n", name)
}
