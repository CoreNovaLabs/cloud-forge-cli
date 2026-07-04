<p align="center">
  <img src="assets/cloud-forge-logo.png" alt="Cloud Forge CLI logo" width="160" />
</p>

<h1 align="center">Cloud Forge CLI</h1>

<p align="center">
  Turn open-source apps into one-command cloud deployments. The current AWS path uses CloudFormation, hardened AMIs, and guided credential setup.
</p>

<p align="center">
  <a href="README.zh-CN.md">中文文档</a>
  ·
  <a href="#quick-start">Quick Start</a>
  ·
  <a href="#aws-credentials">AWS Credentials</a>
  ·
  <a href="#cleanup">Cleanup</a>
</p>

<p align="center">
  <a href="https://github.com/CoreNovaLabs/cloud-forge-cli/actions/workflows/test.yml"><img alt="Test CLI" src="https://github.com/CoreNovaLabs/cloud-forge-cli/actions/workflows/test.yml/badge.svg" /></a>
  <a href="https://github.com/CoreNovaLabs/cloud-forge-cli/releases"><img alt="Release" src="https://img.shields.io/github/v/release/CoreNovaLabs/cloud-forge-cli?sort=semver" /></a>
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/badge/license-MIT-green.svg" /></a>
  <a href="go.mod"><img alt="Go" src="https://img.shields.io/github/go-mod/go-version/CoreNovaLabs/cloud-forge-cli" /></a>
  <img alt="AWS" src="https://img.shields.io/badge/AWS-deploy%20%7C%20delete-ff9900" />
</p>

```bash
curl -fsSL https://cdn.jsdelivr.net/gh/CoreNovaLabs/cloud-forge-cli@main/scripts/install.sh | bash
cloud-forge auth aws
cloud-forge deploy hello-nginx --cloud aws --allowed-ip <YOUR_IP>/32
```

Cloud Forge is building a catalog of open-source apps that can be deployed with a single command. Cloud Forge CLI is the command-line entry point for that catalog: use it to find apps, inspect templates, deploy AWS CloudFormation stacks, and remove the stacks when you are done.

| Capability | What it does |
| --- | --- |
| Catalog search | Browse the growing app catalog, including `hello-nginx`, `gitea`, `n8n`, and `uptime-kuma`. |
| AWS deploy | Create or update CloudFormation stacks and follow progress in the terminal. |
| Built-in auth | Sign in through a browser or configure access keys without installing the AWS CLI. |
| Cleanup | Delete CloudFormation stacks and release the AWS resources they created. |

**Current status:** AWS and Aliyun (`cn-hongkong`) deploy and delete are available for `hello-nginx`, `gitea`, `n8n`, and `uptime-kuma`. Aliyun v1 uses public OS images with UserData bootstrap; first boot may take 8–15 minutes.

## What It Does

Cloud Forge CLI turns catalog entries into a repeatable deployment workflow. The long-term goal is a broad open-source app catalog; the current CLI focuses on AWS deployment first.

For AWS, it can:

- search the catalog
- show app metadata
- render the CloudFormation template for an app
- create or update a CloudFormation stack
- delete a CloudFormation stack
- show CloudFormation resource progress during deploy and delete
- reuse a local SSH key for EC2 access
- print outputs such as service URL, public IP, instance ID, AMI ID, and region when the template provides them

AWS deploys default to `us-east-1`.

## Install

Recommended one-line install:

```bash
curl -fsSL https://cdn.jsdelivr.net/gh/CoreNovaLabs/cloud-forge-cli@main/scripts/install.sh | bash
```

The installer puts `cloud-forge` in `~/.local/bin`. Add that directory to your `PATH` if your shell cannot find the command.

If the CDN is unavailable in your network, use the GitHub raw URL instead:

```bash
curl -fsSL https://raw.githubusercontent.com/CoreNovaLabs/cloud-forge-cli/main/scripts/install.sh | bash
```

Manual install from GitHub Releases:

```text
https://github.com/CoreNovaLabs/cloud-forge-cli/releases
```

Unpack the archive and move the `cloud-forge` binary to a directory on your `PATH`.

Verify the install:

```bash
cloud-forge version
```

You can also build from source. See [Build From Source](#build-from-source).

## AWS Credentials

Cloud Forge CLI calls AWS through the AWS SDK for Go v2. It also includes a browser-based AWS sign-in flow, so the CLI can set up credentials without shelling out to the AWS CLI.

You do not need to install the AWS CLI, but you do need an AWS identity or access keys.

Use the built-in AWS browser sign-in:

```bash
cloud-forge auth aws
```

By default, `cloud-forge auth aws` opens an AWS sign-in page. After authorization, it writes a local profile with temporary credentials. This flow uses AWS Sign-In OAuth with PKCE inside Cloud Forge and does not require the AWS CLI.

If the browser does not open, the CLI prints a sign-in URL that you can copy into a browser. Use `--no-browser` when you want to print the URL and paste the authorization code manually.

If `AWS_PROFILE` is set, the auth command uses that profile by default. Use `--profile NAME` to check or write a specific profile.

Check current auth status:

```bash
cloud-forge auth aws status
```

The status output shows the AWS account, ARN, region, profile, and the credential source selected by the AWS SDK.

Supported credential sources include:

- `~/.aws/credentials`
- `~/.aws/config`
- `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`
- `AWS_PROFILE`
- AWS SSO or assume-role profiles supported by the AWS SDK
- EC2/ECS instance or task roles

Use an AWS profile:

```bash
export AWS_PROFILE=default
```

Use environment variables:

```bash
export AWS_ACCESS_KEY_ID="..."
export AWS_SECRET_ACCESS_KEY="..."
```

AWS deploys default to `us-east-1`. Override it when needed:

```bash
cloud-forge deploy hello-nginx --cloud aws --region us-west-2
```

For production use, prefer a least-privilege IAM user or role. Avoid using AWS account root credentials.

Browser sign-in is the default; this explicit form is equivalent:

```bash
cloud-forge auth aws --method browser
```

Print the sign-in URL without opening a browser:

```bash
cloud-forge auth aws --method browser --no-browser
```

Force manual access key configuration:

```bash
cloud-forge auth aws --method access-key
```

## Quick Start

Find an app:

```bash
cloud-forge search nginx --cloud aws
```

Show app details:

```bash
cloud-forge show hello-nginx
```

Preview the AWS template:

```bash
cloud-forge template hello-nginx --cloud aws
```

Validate the deployment without creating resources:

```bash
cloud-forge deploy hello-nginx --cloud aws --dry-run
```

Deploy to AWS:

```bash
cloud-forge deploy hello-nginx --cloud aws \
  --stack-name cloud-forge-hello-nginx \
  --instance-type t3.micro \
  --allowed-ip 1.2.3.4/32
```

During deployment, the CLI prints CloudFormation progress:

```text
[12:01:08] AWS::EC2::SecurityGroup HelloSecurityGroup CREATE_COMPLETE
[12:01:15] AWS::EC2::Instance HelloInstance CREATE_IN_PROGRESS
```

When the stack finishes, the CLI prints the template outputs and a cleanup command:

```text
To remove later: cloud-forge delete cloud-forge-hello-nginx --cloud aws --region us-east-1
```

## Cleanup

Delete a stack created by Cloud Forge:

```bash
cloud-forge delete cloud-forge-hello-nginx --cloud aws --region us-east-1
```

Wait for deletion to finish (default):

```bash
cloud-forge delete cloud-forge-gitea --cloud aws --wait
```

Start deletion without waiting:

```bash
cloud-forge delete cloud-forge-n8n --cloud aws --no-wait
```

Deleting the stack removes the EC2 instance, Elastic IP, security group, and related resources created by the template.

The reusable local private key stays on disk:

```text
~/.cloud-forge/keys/aws/cloud-forge-default.pem
```

## SSH Key Behavior

By default, AWS deploys use a reusable local SSH key:

```text
~/.cloud-forge/keys/aws/cloud-forge-default.pem
```

On first use, the CLI creates this private key with `0600` permissions. If the target AWS region does not already have a `cloud-forge-default` key pair, the CLI imports the matching public key into EC2. The same local private key is reused across regions.

Use an existing EC2 key pair instead:

```bash
cloud-forge deploy hello-nginx --cloud aws \
  --key-name my-key
```

Disable SSH key injection:

```bash
cloud-forge deploy hello-nginx --cloud aws \
  --ssh-key none
```

Use a custom local private key path:

```bash
cloud-forge deploy hello-nginx --cloud aws \
  --ssh-key-path ~/.cloud-forge/keys/aws/custom.pem
```

## Common Options

```bash
cloud-forge deploy hello-nginx --cloud aws \
  --region us-east-1 \
  --stack-name cloud-forge-hello-nginx \
  --instance-type t3.micro \
  --allowed-ip 1.2.3.4/32 \
  --progress plain
```

Disable progress output:

```bash
cloud-forge deploy hello-nginx --cloud aws \
  --progress none
```

Template parameters can be supplied with dedicated flags or repeated `--param` flags:

```bash
cloud-forge deploy gitea --cloud aws \
  --region us-east-1 \
  --param KeyName=my-key \
  --param ImageId=ami-0123456789abcdef0
```

Dedicated AWS parameter flags include `--instance-type`, `--key`, `--key-name`, `--ssh-key`, `--ssh-key-path`, `--progress`, `--domain`, `--hosted-zone-id`, `--disk-size`, `--vpc`, `--subnet`, `--allowed-ip`, `--image-id`, `--latest-ami-id`, and `--caddy-tls-mode`.

## Catalog Reference

By default, the CLI reads the catalog index from:

```text
https://cdn.jsdelivr.net/gh/CoreNovaLabs/cloud-forge-catalog@main/index/apps.json
```

If the default mirror is unavailable, it falls back to the GitHub raw catalog URL.

For local development:

```bash
export CLOUD_FORGE_STORE_URL="file:///absolute/path/to/cloud-forge-catalog/index/apps.json"
```

## Command Reference

```bash
cloud-forge search hello --cloud aws
cloud-forge auth aws status
cloud-forge show hello-nginx
cloud-forge template hello-nginx --cloud aws
cloud-forge deploy hello-nginx --cloud aws --dry-run
cloud-forge deploy hello-nginx --cloud aliyun --region cn-hongkong \
  --vpc-id vpc-xxx --vswitch-id vsw-xxx --key my-key --dry-run
cloud-forge delete cloud-forge-hello-nginx --cloud aliyun --region cn-hongkong
cloud-forge help deploy
```

Aliyun v1 supports **`cn-hongkong` only**. Configure AccessKey credentials with `cloud-forge auth aliyun` before deploy.

## AWS Deploy

AWS deployment uses the AWS SDK for Go v2 and CloudFormation. It does not call the AWS CLI underneath.

Credentials are loaded from the standard AWS SDK credential chain. AWS deploys default to `us-east-1`; override with `--region` when needed.

```bash
export AWS_PROFILE=default
```

Validate a template without creating resources:

```bash
cloud-forge deploy hello-nginx --cloud aws --dry-run
```

Create or update a stack:

```bash
cloud-forge deploy hello-nginx --cloud aws \
  --stack-name cloud-forge-hello-nginx \
  --instance-type t3.micro \
  --allowed-ip 1.2.3.4/32
```

By default, deploy waits print CloudFormation resource events as plain progress lines:

```text
[12:01:08] AWS::EC2::SecurityGroup HelloSecurityGroup CREATE_COMPLETE
[12:01:15] AWS::EC2::Instance HelloInstance CREATE_IN_PROGRESS
```

Disable progress output:

```bash
cloud-forge deploy hello-nginx --cloud aws \
  --progress none
```

After the stack reaches `CREATE_COMPLETE`, `deploy` keeps polling `ServiceURL` (`/health` and `/`) until the app responds or `--timeout` is reached. The first boot still needs to pull images and obtain a TLS certificate, so this bridges the gap between stack completion and a reachable endpoint. Pass `--no-wait-ready` to return right after the stack is created.

## Aliyun deploy (Hong Kong)

Aliyun v1 uses ROS to create ECS + EIP, then bootstraps Docker/Caddy and the app container via UserData on first boot. Unlike AWS pre-baked AMIs, expect **8–15 minutes** before the service is reachable.

**Default behavior:** after ROS `CREATE_COMPLETE`, `deploy` keeps polling `ServiceURL` (`/health` and `/`) until the app responds or `--timeout` is reached, the same as AWS. Pass `--no-wait-ready` to return right after the stack is created.

Only **`cn-hongkong`** is supported. You need a VPC, VSwitch, and SSH KeyPair in that region.

```bash
cloud-forge auth aliyun
cloud-forge auth aliyun status

cloud-forge deploy hello-nginx --cloud aliyun --region cn-hongkong \
  --vpc-id vpc-xxx \
  --vswitch-id vsw-xxx \
  --key my-key \
  --allowed-ip <YOUR_IP>/32 \
  --timeout 20m

# Stack only — do not wait for bootstrap
cloud-forge deploy hello-nginx --cloud aliyun --region cn-hongkong \
  --vpc-id vpc-xxx --vswitch-id vsw-xxx --key my-key --no-wait-ready

cloud-forge delete cloud-forge-hello-nginx --cloud aliyun --region cn-hongkong
```

Container images use Docker Hub short names (reachable from Hong Kong ECS without ACR).

## Telemetry

By default, the CLI sends anonymous, non-blocking usage events to:

```text
https://telemetry.corenovacloud.com/v1/events
```

Telemetry never includes cloud credentials, account IDs, domains, local paths, or template parameter values.

Disable telemetry when needed:

```bash
export CLOUD_FORGE_TELEMETRY=0
```

Use a different endpoint for local testing:

```bash
export CLOUD_FORGE_TELEMETRY_ENDPOINT="http://127.0.0.1:8787/v1/events"
```

## Build From Source

```bash
go build ./cmd/cloud-forge
```

## Development

```bash
go test ./...
```
