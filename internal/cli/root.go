package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/cloud-forge/cli/internal/telemetry"
	"github.com/cloud-forge/cli/pkg/store"
)

const (
	Version                 = "0.1.0"
	defaultCatalogBaseURL   = "https://cdn.jsdelivr.net/gh/CoreNovaLabs/cloud-forge-catalog@main"
	fallbackCatalogBaseURL  = "https://raw.githubusercontent.com/CoreNovaLabs/cloud-forge-catalog/main"
	defaultStoreURL         = defaultCatalogBaseURL + "/index/apps.json"
	defaultStoreFallbackURL = fallbackCatalogBaseURL + "/index/apps.json"
)

type commonFlags struct {
	storeURL          string
	cacheDir          string
	cacheTTL          time.Duration
	telemetryEndpoint string
	cloud             string
	category          string
	tags              listFlag
}

type listFlag []string

func (f *listFlag) String() string {
	return strings.Join(*f, ",")
}

func (f *listFlag) Set(value string) error {
	if value == "" {
		return nil
	}
	*f = append(*f, value)
	return nil
}

// Run executes the CLI and returns a process exit code.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	switch args[0] {
	case "search":
		return runSearch(ctx, args[1:], stdout, stderr)
	case "show":
		return runShow(ctx, args[1:], stdout, stderr)
	case "template":
		return runTemplate(ctx, args[1:], stdout, stderr)
	case "version":
		fmt.Fprintf(stdout, "cloud-forge %s\n", Version)
		return 0
	case "help", "-h", "--help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func runSearch(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	started := time.Now()
	flags := newFlagSet("search", stderr)
	common := addCommonFlags(flags)
	positionals, err := parseInterspersed(flags, args)
	if err != nil {
		return 2
	}

	query := strings.Join(positionals, " ")
	apps, code := loadApps(ctx, common, store.Filter{
		Query:    query,
		Cloud:    common.cloud,
		Category: common.category,
		Tags:     []string(common.tags),
	}, stderr)
	if code != 0 {
		track(common, ctx, telemetry.Event{
			Event:      "search",
			Cloud:      common.cloud,
			Status:     "failed",
			DurationMS: durationMS(started),
			ErrorCode:  "load_apps",
		})
		return code
	}

	track(common, ctx, telemetry.Event{
		Event:      "search",
		Cloud:      common.cloud,
		Status:     "success",
		DurationMS: durationMS(started),
	})

	if len(apps) == 0 {
		fmt.Fprintln(stdout, "No apps found.")
		return 0
	}

	fmt.Fprintf(stdout, "%-18s %-18s %-12s %-15s %s\n", "ID", "NAME", "CATEGORY", "CLOUDS", "PRICE")
	for _, app := range apps {
		fmt.Fprintf(stdout, "%-18s %-18s %-12s %-15s %s\n",
			app.ID,
			app.Name,
			app.Category,
			strings.Join(app.Clouds, ","),
			app.Price,
		)
	}
	return 0
}

func runShow(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	started := time.Now()
	flags := newFlagSet("show", stderr)
	common := addCommonFlags(flags)
	positionals, err := parseInterspersed(flags, args)
	if err != nil {
		return 2
	}
	if len(positionals) != 1 {
		fmt.Fprintln(stderr, "usage: cloud-forge show <app> [flags]")
		return 2
	}

	st, code := loadStore(ctx, common, stderr)
	if code != 0 {
		track(common, ctx, telemetry.Event{
			Event:      "show",
			AppID:      positionals[0],
			Status:     "failed",
			DurationMS: durationMS(started),
			ErrorCode:  "load_store",
		})
		return code
	}

	app, err := st.Get(positionals[0])
	if err != nil {
		track(common, ctx, telemetry.Event{
			Event:      "show",
			AppID:      positionals[0],
			Status:     "failed",
			DurationMS: durationMS(started),
			ErrorCode:  "app_not_found",
		})
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}

	track(common, ctx, telemetry.Event{
		Event:      "show",
		AppID:      app.ID,
		AppVersion: app.Version,
		Status:     "success",
		DurationMS: durationMS(started),
	})

	fmt.Fprintf(stdout, "%s (%s)\n", app.Name, app.ID)
	fmt.Fprintf(stdout, "Description: %s\n", app.Desc)
	fmt.Fprintf(stdout, "Version:     %s\n", app.Version)
	fmt.Fprintf(stdout, "Category:    %s\n", app.Category)
	fmt.Fprintf(stdout, "Clouds:      %s\n", strings.Join(app.Clouds, ", "))
	if app.Price != "" {
		fmt.Fprintf(stdout, "Price:       %s\n", app.Price)
	}

	if len(app.Images) > 0 {
		fmt.Fprintln(stdout, "\nImages:")
		for _, cloud := range sortedKeys(app.Images) {
			fmt.Fprintf(stdout, "  %-8s %s\n", cloud, app.Images[cloud])
		}
	}

	if len(app.Params) > 0 {
		fmt.Fprintln(stdout, "\nParameters:")
		for _, name := range sortedKeys(app.Params) {
			param := app.Params[name]
			required := "optional"
			if param.Required || cloudRequired(param.AWS) || cloudRequired(param.Aliyun) {
				required = "required"
			}
			secret := ""
			if param.Secret {
				secret = " secret"
			}
			fmt.Fprintf(stdout, "  %-16s %-8s %s%s\n", name, required, param.Type, secret)
		}
	}

	return 0
}

func runTemplate(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	started := time.Now()
	flags := newFlagSet("template", stderr)
	common := addCommonFlags(flags)
	positionals, err := parseInterspersed(flags, args)
	if err != nil {
		return 2
	}
	if len(positionals) != 1 {
		fmt.Fprintln(stderr, "usage: cloud-forge template <app> --cloud <aws|aliyun> [flags]")
		return 2
	}
	if common.cloud == "" {
		common.cloud = "aws"
	}

	st, code := loadStore(ctx, common, stderr)
	if code != 0 {
		track(common, ctx, telemetry.Event{
			Event:      "template_fetch",
			AppID:      positionals[0],
			Cloud:      common.cloud,
			Status:     "failed",
			DurationMS: durationMS(started),
			ErrorCode:  "load_store",
		})
		return code
	}

	app, appErr := st.Get(positionals[0])
	body, err := st.GetTemplate(ctx, positionals[0], common.cloud)
	if err != nil {
		track(common, ctx, telemetry.Event{
			Event:      "template_fetch",
			AppID:      positionals[0],
			Cloud:      common.cloud,
			Status:     "failed",
			DurationMS: durationMS(started),
			ErrorCode:  "template_fetch",
		})
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	event := telemetry.Event{
		Event:      "template_fetch",
		AppID:      positionals[0],
		Cloud:      common.cloud,
		Status:     "success",
		DurationMS: durationMS(started),
	}
	if appErr == nil && app != nil {
		event.AppID = app.ID
		event.AppVersion = app.Version
	}
	track(common, ctx, event)

	fmt.Fprint(stdout, body)
	if !strings.HasSuffix(body, "\n") {
		fmt.Fprintln(stdout)
	}
	return 0
}

func addCommonFlags(flags *flag.FlagSet) *commonFlags {
	common := &commonFlags{
		storeURL: defaultStoreURL,
		cacheTTL: 24 * time.Hour,
	}
	if envURL := os.Getenv("CLOUD_FORGE_STORE_URL"); envURL != "" {
		common.storeURL = envURL
	}
	flags.StringVar(&common.storeURL, "store-url", common.storeURL, "catalog index URL or local path")
	flags.StringVar(&common.cacheDir, "cache-dir", "", "cache directory")
	flags.DurationVar(&common.cacheTTL, "cache-ttl", common.cacheTTL, "catalog cache TTL")
	flags.StringVar(&common.telemetryEndpoint, "telemetry-endpoint", telemetryEndpointFromEnv(), "telemetry endpoint URL")
	flags.StringVar(&common.cloud, "cloud", "", "cloud provider filter")
	flags.StringVar(&common.category, "category", "", "category filter")
	flags.Var(&common.tags, "tag", "tag filter; may be repeated")
	return common
}

func track(common *commonFlags, ctx context.Context, event telemetry.Event) {
	client := telemetry.NewClient(telemetry.Config{
		CacheDir:   common.cacheDir,
		Endpoint:   common.telemetryEndpoint,
		CLIVersion: Version,
	})
	client.Track(ctx, event)
}

func telemetryEndpointFromEnv() string {
	if endpoint := os.Getenv("CLOUD_FORGE_TELEMETRY_ENDPOINT"); endpoint != "" {
		return endpoint
	}
	return telemetry.DefaultEndpoint
}

func durationMS(started time.Time) int64 {
	return time.Since(started).Milliseconds()
}

func loadApps(ctx context.Context, common *commonFlags, filter store.Filter, stderr io.Writer) ([]store.App, int) {
	st, code := loadStore(ctx, common, stderr)
	if code != 0 {
		return nil, code
	}

	apps, err := st.List(filter)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return nil, 1
	}
	return apps, 0
}

func loadStore(ctx context.Context, common *commonFlags, stderr io.Writer) (store.Store, int) {
	cfg := store.Config{
		IndexURL: common.storeURL,
		CacheDir: common.cacheDir,
		CacheTTL: common.cacheTTL,
	}
	if common.storeURL == defaultStoreURL {
		cfg.IndexFallbackURLs = []string{defaultStoreFallbackURL}
		cfg.TemplateBaseURLs = []string{defaultCatalogBaseURL, fallbackCatalogBaseURL}
	}

	st := store.NewHTTPStore(cfg)
	if err := st.Sync(ctx); err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return nil, 1
	}
	return st, 0
}

func newFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	return flags
}

func parseInterspersed(flags *flag.FlagSet, args []string) ([]string, error) {
	flagArgs := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			flagArgs = append(flagArgs, arg)
			if !strings.Contains(arg, "=") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				flagArgs = append(flagArgs, args[i+1])
				i++
			}
			continue
		}
		positionals = append(positionals, arg)
	}

	if err := flags.Parse(flagArgs); err != nil {
		return nil, err
	}
	return positionals, nil
}

func printUsage(w io.Writer) {
	fmt.Fprintf(w, `cloud-forge %s

Usage:
  cloud-forge search [query] [--cloud aws|aliyun] [--category name]
  cloud-forge show <app>
  cloud-forge template <app> --cloud aws|aliyun
  cloud-forge version

Environment:
  CLOUD_FORGE_STORE_URL  Catalog index URL or local file path.
  CLOUD_FORGE_TELEMETRY  Set to 0, false, off, or disabled to disable telemetry.

`, Version)
}

func cloudRequired(param *store.CloudParam) bool {
	return param != nil && param.Required
}

func sortedKeys[V any](items map[string]V) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
