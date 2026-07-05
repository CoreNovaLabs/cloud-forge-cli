package cli

import (
	"crypto/rand"
	"fmt"
	"io"
	"math/big"
	"strings"

	"github.com/cloud-forge/cli/pkg/store"
)

const (
	adminPasswordParam = "AdminPassword"
	adminPasswordMin   = 8
	adminPasswordChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
)

type adminPasswordConfig struct {
	password  string
	generated bool
}

func appHasParam(app *store.App, name string) bool {
	if app == nil || app.Params == nil {
		return false
	}
	_, ok := app.Params[name]
	return ok
}

func configureAdminPassword(app *store.App, deploy *deployFlags) (*adminPasswordConfig, error) {
	if !appHasParam(app, adminPasswordParam) {
		return nil, nil
	}

	fromFlag := strings.TrimSpace(deploy.adminPassword)
	fromParam := ""
	if deploy.parameters != nil {
		fromParam = strings.TrimSpace(deploy.parameters[adminPasswordParam])
	}
	if fromFlag != "" && fromParam != "" && fromFlag != fromParam {
		return nil, fmt.Errorf("--admin-password and --param AdminPassword=... must match when both are set")
	}

	password := fromFlag
	if password == "" {
		password = fromParam
	}

	generated := false
	if password == "" {
		var err error
		password, err = generateAdminPassword(24)
		if err != nil {
			return nil, err
		}
		generated = true
	}
	if len(password) < adminPasswordMin {
		return nil, fmt.Errorf("AdminPassword must be at least %d characters", adminPasswordMin)
	}

	if deploy.parameters == nil {
		deploy.parameters = keyValueFlag{}
	}
	deploy.parameters[adminPasswordParam] = password

	return &adminPasswordConfig{
		password:  password,
		generated: generated,
	}, nil
}

func generateAdminPassword(length int) (string, error) {
	if length < adminPasswordMin {
		return "", fmt.Errorf("password length must be at least %d", adminPasswordMin)
	}
	max := big.NewInt(int64(len(adminPasswordChars)))
	out := make([]byte, length)
	for i := range out {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("generate admin password: %w", err)
		}
		out[i] = adminPasswordChars[n.Int64()]
	}
	return string(out), nil
}

func printAdminPassword(stdout io.Writer, cfg *adminPasswordConfig, dryRun bool) {
	if cfg == nil || !cfg.generated {
		return
	}
	if dryRun {
		fmt.Fprintln(stdout, "\nAdminPassword: (will be auto-generated on deploy, minimum 8 characters)")
		return
	}
	fmt.Fprintln(stdout, "\nAdminPassword: (auto-generated — save this, shown once)")
	fmt.Fprintf(stdout, "  %s\n", cfg.password)
}
