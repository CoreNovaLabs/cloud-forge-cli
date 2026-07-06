package cli

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
)

const launchURLScheme = "cloud-forge"

func runLaunchURL(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := newFlagSet("launch-url", stderr)
	printCommand := flags.Bool("print-command", false, "print the equivalent cloud-forge command without running it")
	positionals, err := parseInterspersed(flags, args)
	if err != nil {
		return 2
	}
	if len(positionals) != 1 {
		fmt.Fprintln(stderr, "usage: cloud-forge launch-url <cloud-forge://deploy?...>")
		return 2
	}

	deployArgs, err := deployArgsFromLaunchURL(positionals[0])
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 2
	}
	if *printCommand {
		fmt.Fprintln(stdout, shellCommand(append([]string{"cloud-forge"}, deployArgs...)))
		return 0
	}

	fmt.Fprintln(stdout, "Opening Cloud Forge deploy from browser launcher.")
	return RunWithIO(ctx, deployArgs, stdin, stdout, stderr)
}

func deployArgsFromLaunchURL(raw string) ([]string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("parse launch URL: %w", err)
	}
	if u.Scheme != launchURLScheme {
		return nil, fmt.Errorf("launch URL must use %s://, got %q", launchURLScheme, u.Scheme)
	}
	action := u.Host
	if action == "" {
		action = strings.Trim(strings.TrimPrefix(u.Path, "/"), "/")
	}
	if action != "deploy" {
		return nil, fmt.Errorf("unsupported launch action %q; expected deploy", action)
	}

	query := u.Query()
	appID := strings.TrimSpace(query.Get("app"))
	if appID == "" {
		pathApp := strings.Trim(strings.TrimPrefix(u.Path, "/"), "/")
		if u.Host == "deploy" && pathApp != "" {
			appID = pathApp
		}
	}
	if appID == "" {
		return nil, fmt.Errorf("launch URL is missing app")
	}
	if strings.ContainsAny(appID, " \t\r\n") {
		return nil, fmt.Errorf("launch URL app contains whitespace: %q", appID)
	}

	cloud := strings.TrimSpace(query.Get("cloud"))
	if cloud == "" {
		cloud = "aws"
	}
	switch cloud {
	case "aws", "aliyun":
	default:
		return nil, fmt.Errorf("unsupported cloud %q", cloud)
	}

	deployArgs := []string{"deploy", appID, "--cloud", cloud}
	appendQueryFlag := func(keys []string, flag string) {
		for _, key := range keys {
			if value := strings.TrimSpace(query.Get(key)); value != "" {
				deployArgs = append(deployArgs, flag, value)
				return
			}
		}
	}
	appendQueryFlag([]string{"region"}, "--region")
	appendQueryFlag([]string{"profile"}, "--profile")
	appendQueryFlag([]string{"stackName", "stack-name"}, "--stack-name")
	appendBoolFlag(query, &deployArgs, "dryRun", "--dry-run")
	appendBoolFlag(query, &deployArgs, "noWait", "--no-wait")
	appendBoolFlag(query, &deployArgs, "noWaitReady", "--no-wait-ready")

	for _, name := range launchURLParamNames(query) {
		for _, value := range query["param."+name] {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			deployArgs = append(deployArgs, "--param", name+"="+value)
		}
	}
	return deployArgs, nil
}

func appendBoolFlag(query url.Values, args *[]string, key, flag string) {
	switch strings.ToLower(strings.TrimSpace(query.Get(key))) {
	case "1", "true", "yes", "on":
		*args = append(*args, flag)
	}
}

func launchURLParamNames(query url.Values) []string {
	names := make([]string, 0)
	for key := range query {
		name, ok := strings.CutPrefix(key, "param.")
		if !ok || strings.TrimSpace(name) == "" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func shellCommand(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, shellQuote(arg))
	}
	return strings.Join(quoted, " ")
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if strings.IndexFunc(value, func(r rune) bool {
		return !(r == '-' || r == '_' || r == '.' || r == '/' || r == ':' || r == '@' || r == '%' || r == '+' || r == '=' || r == ',' || r == '~' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'))
	}) == -1 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
