package cli

import (
	"fmt"
	"io"
)

func printCommandHelp(w io.Writer, command string) {
	switch command {
	case "search":
		printSearchHelp(w)
	case "show":
		printShowHelp(w)
	case "template":
		printTemplateHelp(w)
	case "deploy":
		printDeployHelp(w)
	case "delete", "destroy":
		printDeleteHelp(w)
	case "auth":
		printAuthHelp(w)
	case "auth aws":
		printAuthAWSHelp(w)
	case "auth aliyun":
		printAuthAliyunHelp(w)
	case "launch-url":
		printLaunchURLHelp(w)
	case "install-protocol":
		printInstallProtocolHelp(w)
	default:
		fmt.Fprintf(w, "Unknown help topic %q\n\n", command)
		printUsage(w)
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintf(w, `cloud-forge %s

Usage:
  cloud-forge search [query] [flags]
  cloud-forge show <app> [flags]
  cloud-forge template <app> --cloud aws|aliyun [flags]
  cloud-forge deploy <app> --cloud aws|aliyun [flags]
  cloud-forge delete <stack> --cloud aws|aliyun [flags]
  cloud-forge auth aws|aliyun [status] [flags]
  cloud-forge launch-url <cloud-forge://deploy?...>
  cloud-forge install-protocol
  cloud-forge version
  cloud-forge help [command]

Aliyun deploy defaults to cn-hongkong; use --region for other regions. Mainland China regions may fail bootstrap (Docker Hub / catalog CDN). First bootstrap may take 8-15 minutes.

Exit codes:
  0  success
  1  runtime error
  2  usage or validation error

Environment:
  CLOUD_FORGE_STORE_URL              Catalog index URL or local file path
  CLOUD_FORGE_TELEMETRY              Set to 0, false, off, or disabled to disable telemetry
  CLOUD_FORGE_TELEMETRY_ENDPOINT     Override telemetry endpoint URL
  AWS_PROFILE                        Default AWS profile for auth and deploy
  AWS_REGION / AWS_DEFAULT_REGION    Default AWS region
  ALIBABA_CLOUD_ACCESS_KEY_ID        Aliyun access key (alternative to auth aliyun)
  ALIBABA_CLOUD_ACCESS_KEY_SECRET    Aliyun secret key

Documentation:
  https://github.com/CoreNovaLabs/cloud-forge-cli

Run "cloud-forge help <command>" for command-specific flags.

`, Version)
}

func printSearchHelp(w io.Writer) {
	fmt.Fprintf(w, `Usage:
  cloud-forge search [query] [flags]

Search the Cloud Forge catalog.

Flags:
  --cloud <aws|aliyun>   Filter by cloud provider
  --category <name>      Filter by category
  --tag <name>           Filter by tag (repeatable)
  --store-url <url>      Catalog index URL or local file path
  --cache-dir <path>     Catalog cache directory
  --cache-ttl <duration> Catalog cache TTL; 0 forces refresh

`)
}

func printShowHelp(w io.Writer) {
	fmt.Fprintf(w, `Usage:
  cloud-forge show <app> [flags]

Show app metadata, supported clouds, pricing, and deploy parameters.

Flags:
  --store-url <url>      Catalog index URL or local file path
  --cache-dir <path>     Catalog cache directory
  --cache-ttl <duration> Catalog cache TTL; 0 forces refresh

`)
}

func printTemplateHelp(w io.Writer) {
	fmt.Fprintf(w, `Usage:
  cloud-forge template <app> --cloud aws|aliyun [flags]

Download the IaC template for an app.

Flags:
  --cloud <aws|aliyun>   Cloud provider (default: aws)
  --store-url <url>      Catalog index URL or local file path
  --cache-dir <path>     Catalog cache directory
  --cache-ttl <duration> Catalog cache TTL; 0 forces refresh

`)
}

func printDeployHelp(w io.Writer) {
	fmt.Fprintf(w, `Usage:
  cloud-forge deploy <app> --cloud aws|aliyun [flags]

Create or update a CloudFormation (AWS) or ROS (Aliyun) stack for a catalog app.

Flags:
  --cloud <aws|aliyun>         Cloud provider (default: aws)
  --region <name>              AWS region (default: us-east-1) or Aliyun region (default: cn-hongkong)
  --profile <name>             AWS shared config profile or Aliyun credentials profile
  --stack-name <name>          Stack name (default: cloud-forge-<app>)
  --instance-type <type>       InstanceType parameter
  --allowed-ip <cidr>          Restrict SSH to this CIDR
  --dry-run                    Validate template and parameters without creating resources
  --no-wait                    Return after starting the stack operation
  --wait-ready                 Wait for app bootstrap after stack completes (default true)
  --no-wait-ready              Return after stack completes without waiting for the endpoint
  --timeout <duration>         Maximum wait time for stack completion and app bootstrap (default: 30m)
  --progress <plain|none>      Show stack progress events (default: plain)
  --ssh-key <auto|none>        Manage a local SSH key automatically (AWS only, default: auto)
  --ssh-key-path <path>        Private key path for --ssh-key auto (AWS only)
  --key, --key-name <name>     AWS EC2 key pair or Aliyun KeyPairName
  --domain <name>              DomainName parameter (custom HTTPS endpoint)
  --hosted-zone-id <id>        Route53 hosted zone ID (AWS; with --domain)
  --dns-domain <name>          Aliyun DNS root domain (with --domain for automatic A records)
  --disk-size <gb>             DiskSize parameter
  --vpc, --vpc-id <id>         VpcId parameter
  --subnet, --subnet-id <id>   SubnetId parameter (AWS)
  --vswitch-id <id>            VSwitchId parameter (Aliyun; auto-discovered when omitted)
  --image-id <id>              ImageId override (Aliyun) or LatestAmiId alias (AWS)
  --latest-ami-id <ami>        LatestAmiId parameter override (AWS)
  --caddy-tls-mode <mode>      CaddyTlsMode parameter (auto, ip-letsencrypt, http, internal)
  --caddy-email <email>        ACME contact email for Caddy public certificates
  --admin-password <password>  AdminPassword for apps that require it (auto-generated when omitted)
  --param, --parameter <k=v>   Parameter override (repeatable)
  --store-url <url>            Catalog index URL or local file path
  --cache-dir <path>           Catalog cache directory
  --cache-ttl <duration>       Catalog cache TTL; 0 forces refresh

Examples:
  cloud-forge deploy hello-nginx --cloud aws --dry-run
  cloud-forge deploy hello-nginx --cloud aliyun --region cn-hongkong

`)
}

func printDeleteHelp(w io.Writer) {
	fmt.Fprintf(w, `Usage:
  cloud-forge delete <stack-name> --cloud aws|aliyun [flags]

Delete a stack created by Cloud Forge.

Flags:
  --cloud <aws|aliyun>       Cloud provider (default: aws)
  --region <name>            AWS region (default: us-east-1) or Aliyun region (default: cn-hongkong)
  --profile <name>           AWS shared config profile or Aliyun credentials profile
  --no-wait                  Return immediately after starting deletion
  --timeout <duration>       Maximum wait time (default: 30m)
  --progress <plain|none>    Show stack progress events (default: plain)

Examples:
  cloud-forge delete cloud-forge-hello-nginx --cloud aws
  cloud-forge delete cloud-forge-hello-nginx --cloud aliyun --region cn-hongkong

`)
}

func printAuthHelp(w io.Writer) {
	fmt.Fprintf(w, `Usage:
  cloud-forge auth aws [status] [flags]
  cloud-forge auth aliyun [status] [flags]

Configure or inspect cloud credentials.

Run "cloud-forge help auth aws" or "cloud-forge help auth aliyun" for details.

`)
}

func printAuthAWSHelp(w io.Writer) {
	fmt.Fprintf(w, `Usage:
  cloud-forge auth aws [status] [flags]

Configure or inspect AWS credentials.

Subcommands:
  status                   Show current AWS auth status

Flags:
  --profile <name>         AWS profile to check or write (default: AWS_PROFILE or cloud-forge)
  --region <name>          Default AWS region for the profile (default: us-east-1)
  --method <auto|browser|access-key>  Auth method (default: browser)
  --no-browser             Print the sign-in URL without opening a browser

Examples:
  cloud-forge auth aws
  cloud-forge auth aws status
  cloud-forge auth aws --method access-key

`)
}

func printAuthAliyunHelp(w io.Writer) {
	fmt.Fprintf(w, `Usage:
  cloud-forge auth aliyun [status] [flags]

Configure or inspect Aliyun AccessKey credentials for ROS deploy.

Subcommands:
  status                   Show current Aliyun auth status

Flags:
  --profile <name>         Credentials profile (default: default)
  --region <name>          Default region (default: cn-hongkong)

Credentials are stored in ~/.cloud-forge/aliyun/credentials

Examples:
  cloud-forge auth aliyun
  cloud-forge auth aliyun status

`)
}

func printLaunchURLHelp(w io.Writer) {
	fmt.Fprintf(w, `Usage:
  cloud-forge launch-url <cloud-forge://deploy?...> [flags]

Open a deployment plan generated by the Cloud Forge browser launcher.

Flags:
  --print-command        Print the equivalent cloud-forge command without running it

Example:
  cloud-forge launch-url 'cloud-forge://deploy?app=actual-budget&cloud=aws&region=us-east-1&stackName=cloud-forge-actual-budget'

`)
}

func printInstallProtocolHelp(w io.Writer) {
	fmt.Fprintf(w, `Usage:
  cloud-forge install-protocol

Register cloud-forge:// browser launch links on this computer.

After registration, the web launcher can open the local Cloud Forge CLI.
Credentials are still configured and stored locally by the CLI.

`)
}

func wantsHelp(args []string) bool {
	for _, arg := range args {
		switch arg {
		case "-h", "--help", "help":
			return true
		}
	}
	return false
}

func helpTopic(args []string) string {
	for i, arg := range args {
		if arg == "help" && i+1 < len(args) {
			return args[i+1]
		}
	}
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			return ""
		}
	}
	return ""
}
