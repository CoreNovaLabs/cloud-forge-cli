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
	case "auth", "auth aws":
		printAuthHelp(w)
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
  cloud-forge deploy <app> --cloud aws [flags]
  cloud-forge delete <stack> --cloud aws [flags]
  cloud-forge auth aws [status] [flags]
  cloud-forge version
  cloud-forge help [command]

Deploy and delete currently support AWS only. Search and template also support Aliyun.

Exit codes:
  0  success
  1  runtime error
  2  usage or validation error

Environment:
  CLOUD_FORGE_STORE_URL           Catalog index URL or local file path
  CLOUD_FORGE_TELEMETRY           Set to 0, false, off, or disabled to disable telemetry
  CLOUD_FORGE_TELEMETRY_ENDPOINT  Override telemetry endpoint URL
  AWS_PROFILE                     Default AWS profile for auth and deploy
  AWS_REGION / AWS_DEFAULT_REGION Default AWS region

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
  --cache-ttl <duration> Catalog cache TTL

`)
}

func printShowHelp(w io.Writer) {
	fmt.Fprintf(w, `Usage:
  cloud-forge show <app> [flags]

Show app metadata, supported clouds, pricing, and deploy parameters.

Flags:
  --store-url <url>      Catalog index URL or local file path
  --cache-dir <path>     Catalog cache directory
  --cache-ttl <duration> Catalog cache TTL

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
  --cache-ttl <duration> Catalog cache TTL

`)
}

func printDeployHelp(w io.Writer) {
	fmt.Fprintf(w, `Usage:
  cloud-forge deploy <app> --cloud aws [flags]

Create or update an AWS CloudFormation stack for a catalog app.

Flags:
  --cloud <aws>              Cloud provider (deploy supports aws only)
  --region <name>            AWS region (default: us-east-1)
  --profile <name>           AWS shared config profile
  --stack-name <name>        CloudFormation stack name (default: cloud-forge-<app>)
  --instance-type <type>     CloudFormation InstanceType parameter
  --allowed-ip <cidr>        Restrict SSH to this CIDR (default from catalog, often 0.0.0.0/0)
  --dry-run                  Validate template and parameters without creating resources
  --no-wait                  Return after starting the stack operation
  --timeout <duration>       Maximum wait time (default: 30m)
  --progress <plain|none>    Show CloudFormation progress events (default: plain)
  --ssh-key <auto|none>      Manage a local SSH key automatically (default: auto)
  --ssh-key-path <path>      Private key path for --ssh-key auto
  --key, --key-name <name>   Use an existing AWS EC2 key pair
  --domain <name>            CloudFormation DomainName parameter
  --hosted-zone-id <id>      CloudFormation HostedZoneId parameter
  --disk-size <gb>           CloudFormation DiskSize parameter
  --vpc, --vpc-id <id>       CloudFormation VpcId parameter
  --subnet, --subnet-id <id> CloudFormation SubnetId parameter
  --image-id <ami>           CloudFormation ImageId parameter override
  --latest-ami-id <ami>      CloudFormation LatestAmiId parameter override
  --param, --parameter <k=v> CloudFormation parameter override (repeatable)
  --store-url <url>          Catalog index URL or local file path
  --cache-dir <path>         Catalog cache directory
  --cache-ttl <duration>     Catalog cache TTL

Examples:
  cloud-forge deploy hello-nginx --cloud aws --dry-run
  cloud-forge deploy gitea --cloud aws --allowed-ip 203.0.113.10/32

`)
}

func printDeleteHelp(w io.Writer) {
	fmt.Fprintf(w, `Usage:
  cloud-forge delete <stack-name> --cloud aws [flags]

Delete an AWS CloudFormation stack created by Cloud Forge.

Flags:
  --cloud <aws>              Cloud provider (delete supports aws only)
  --region <name>            AWS region (default: us-east-1)
  --profile <name>           AWS shared config profile
  --wait                     Wait until stack deletion completes (default: true)
  --no-wait                  Return immediately after starting deletion
  --timeout <duration>       Maximum wait time (default: 30m)
  --progress <plain|none>    Show CloudFormation progress events (default: plain)
  --store-url <url>          Catalog index URL or local file path (for telemetry only)
  --cache-dir <path>         Cache directory
  --telemetry-endpoint <url> Telemetry endpoint URL

Examples:
  cloud-forge delete cloud-forge-hello-nginx --cloud aws
  cloud-forge delete cloud-forge-gitea --cloud aws --region us-west-2 --no-wait

`)
}

func printAuthHelp(w io.Writer) {
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
