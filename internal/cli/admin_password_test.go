package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/cloud-forge/cli/pkg/store"
)

func TestConfigureAdminPasswordGeneratesWhenMissing(t *testing.T) {
	app := &store.App{
		Params: map[string]store.ParamDefinition{
			adminPasswordParam: {Type: "string", Secret: true},
		},
	}
	deploy := &deployFlags{parameters: keyValueFlag{}}

	cfg, err := configureAdminPassword(app, deploy)
	if err != nil {
		t.Fatal(err)
	}
	if cfg == nil || !cfg.generated {
		t.Fatal("expected generated admin password")
	}
	if len(cfg.password) < adminPasswordMin {
		t.Fatalf("password too short: %q", cfg.password)
	}
	if deploy.parameters[adminPasswordParam] != cfg.password {
		t.Fatalf("parameter not set: %q", deploy.parameters[adminPasswordParam])
	}
}

func TestConfigureAdminPasswordUsesManualValue(t *testing.T) {
	app := &store.App{
		Params: map[string]store.ParamDefinition{
			adminPasswordParam: {Type: "string", Secret: true},
		},
	}
	deploy := &deployFlags{
		adminPassword: "ManualPass123",
		parameters:    keyValueFlag{},
	}

	cfg, err := configureAdminPassword(app, deploy)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.generated {
		t.Fatal("expected manual password")
	}
	if cfg.password != "ManualPass123" {
		t.Fatalf("unexpected password %q", cfg.password)
	}
}

func TestConfigureAdminPasswordRejectsConflict(t *testing.T) {
	app := &store.App{
		Params: map[string]store.ParamDefinition{
			adminPasswordParam: {Type: "string", Secret: true},
		},
	}
	deploy := &deployFlags{
		adminPassword: "one",
		parameters:    keyValueFlag{adminPasswordParam: "two"},
	}

	if _, err := configureAdminPassword(app, deploy); err == nil || !strings.Contains(err.Error(), "must match") {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestConfigureAdminPasswordSkipsWhenNotRequired(t *testing.T) {
	app := &store.App{Params: map[string]store.ParamDefinition{}}
	deploy := &deployFlags{parameters: keyValueFlag{}}

	cfg, err := configureAdminPassword(app, deploy)
	if err != nil {
		t.Fatal(err)
	}
	if cfg != nil {
		t.Fatal("expected nil config")
	}
}

func TestExportGeneratedAdminPassword(t *testing.T) {
	out := os.Getenv("CF_ADMIN_PASSWORD_OUT")
	if out == "" {
		t.Skip("CF_ADMIN_PASSWORD_OUT not set")
	}
	app := &store.App{
		Params: map[string]store.ParamDefinition{
			adminPasswordParam: {Type: "string", Secret: true},
		},
	}
	deploy := &deployFlags{parameters: keyValueFlag{}}
	cfg, err := configureAdminPassword(app, deploy)
	if err != nil {
		t.Fatal(err)
	}
	if cfg == nil || !cfg.generated {
		t.Fatal("expected generated password")
	}
	if err := os.WriteFile(out, []byte(cfg.password), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestGeneratedPasswordFlowsToDeployParameters(t *testing.T) {
	app := &store.App{
		Params: map[string]store.ParamDefinition{
			adminPasswordParam: {Type: "string", Secret: true},
			"InstanceType": {
				Type: "string",
				AWS:  &store.CloudParam{Default: "t3.small"},
			},
			"AllowedIP": {Type: "string", Default: "0.0.0.0/0", AWS: &store.CloudParam{Default: "0.0.0.0/0"}},
		},
	}
	deploy := &deployFlags{parameters: keyValueFlag{}}
	cfg, err := configureAdminPassword(app, deploy)
	if err != nil {
		t.Fatal(err)
	}
	params, err := buildAWSDeployParameters(app, deploy)
	if err != nil {
		t.Fatal(err)
	}
	if params[adminPasswordParam] != cfg.password {
		t.Fatalf("deploy params password %q != generated %q", params[adminPasswordParam], cfg.password)
	}
}
