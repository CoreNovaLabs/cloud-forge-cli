package aliyundeploy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	ecs "github.com/alibabacloud-go/ecs-20140526/v4/client"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/cloud-forge/cli/internal/awsdeploy"
)

type EnsureKeyPairInput struct {
	KeyPairName    string
	PrivateKeyPath string
	DryRun         bool
}

type EnsureKeyPairOutput struct {
	KeyPairName string
	Message     string
}

func DefaultPrivateKeyPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".cloud-forge", "keys", "aliyun", DefaultKeyPairName+".pem"), nil
}

func EnsureKeyPair(ctx context.Context, cfg Config, input EnsureKeyPairInput) (*EnsureKeyPairOutput, error) {
	_ = ctx
	keyName := strings.TrimSpace(input.KeyPairName)
	if keyName == "" {
		keyName = DefaultKeyPairName
	}

	privateKeyPath := strings.TrimSpace(input.PrivateKeyPath)
	if privateKeyPath == "" {
		var err error
		privateKeyPath, err = DefaultPrivateKeyPath()
		if err != nil {
			return nil, err
		}
	}

	_, ecsClient, err := newVPCAndECSClients(cfg)
	if err != nil {
		return nil, err
	}

	names, err := listKeyPairNames(ecsClient, cfg.Region)
	if err != nil {
		return nil, err
	}

	if selected := selectKeyPairName(names, keyName); selected != "" && selected != keyName {
		return &EnsureKeyPairOutput{
			KeyPairName: selected,
			Message:     fmt.Sprintf("Using KeyPair %s (pass --key to override)", selected),
		}, nil
	}

	remoteExists := containsString(names, keyName)
	if remoteExists {
		return &EnsureKeyPairOutput{KeyPairName: keyName}, nil
	}

	if input.DryRun {
		if len(names) > 0 {
			return &EnsureKeyPairOutput{
				KeyPairName: names[0],
				Message:     fmt.Sprintf("Dry-run: would use KeyPair %s", names[0]),
			}, nil
		}
		return nil, fmt.Errorf("no KeyPair found in %s; create one in the console or run deploy without --dry-run to import %s", cfg.Region, keyName)
	}

	publicKey, _, err := awsdeploy.LocalKeyMaterial(privateKeyPath, keyName)
	if err != nil {
		return nil, fmt.Errorf("prepare local SSH key: %w", err)
	}

	if _, err := ecsClient.ImportKeyPair(&ecs.ImportKeyPairRequest{
		RegionId:      tea.String(cfg.Region),
		KeyPairName:   tea.String(keyName),
		PublicKeyBody: tea.String(publicKey),
	}); err != nil && !isDuplicateKeyPairError(err) {
		return nil, fmt.Errorf("import KeyPair %q in %s: %w", keyName, cfg.Region, err)
	}

	msg := fmt.Sprintf("Using KeyPair %s (imported from %s)", keyName, privateKeyPath)
	return &EnsureKeyPairOutput{
		KeyPairName: keyName,
		Message:     msg,
	}, nil
}

func listKeyPairNames(client *ecs.Client, region string) ([]string, error) {
	out, err := client.DescribeKeyPairs(&ecs.DescribeKeyPairsRequest{
		RegionId: tea.String(region),
		PageSize: tea.Int32(50),
	})
	if err != nil {
		return nil, fmt.Errorf("list KeyPairs in %s: %w", region, err)
	}
	if out.Body == nil {
		return nil, nil
	}
	names := make([]string, 0, len(out.Body.KeyPairs.KeyPair))
	for _, kp := range out.Body.KeyPairs.KeyPair {
		if name := strings.TrimSpace(tea.StringValue(kp.KeyPairName)); name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

func selectKeyPairName(names []string, preferred string) string {
	if preferred != "" && preferred != DefaultKeyPairName {
		return ""
	}
	for _, name := range names {
		if name == DefaultKeyPairName {
			return name
		}
	}
	if len(names) == 1 {
		return names[0]
	}
	if len(names) > 1 {
		return names[0]
	}
	return ""
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func isDuplicateKeyPairError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate") || strings.Contains(msg, "already exist")
}
