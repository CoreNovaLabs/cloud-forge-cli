<p align="center">
  <img src="assets/cloud-forge-logo.png" alt="Cloud Forge CLI logo" width="160" />
</p>

<h1 align="center">Cloud Forge CLI</h1>

<p align="center">
  Turn open-source apps into one-command cloud deployments. Supports AWS (CloudFormation + hardened AMIs) and Aliyun Hong Kong (ROS), with guided credential setup.
</p>

<p align="center">
  <a href="README.zh-CN.md">中文文档</a>
  ·
  <a href="#install">Install</a>
  ·
  <a href="#quick-start">Quick Start</a>
  ·
  <a href="#aws-deploy">AWS Deploy</a>
  ·
  <a href="#aliyun-deploy-hong-kong">Aliyun Deploy</a>
  ·
  <a href="#command-reference">Command Reference</a>
</p>

<p align="center">
  <img alt="AWS" src="https://img.shields.io/badge/AWS-deploy%20%7C%20delete-ff9900" />
  <img alt="Aliyun" src="https://img.shields.io/badge/Aliyun-deploy%20%7C%20delete-0089FF" />
  <a href="https://github.com/CoreNovaLabs/cloud-forge-cli/releases"><img alt="Release" src="https://img.shields.io/github/v/release/CoreNovaLabs/cloud-forge-cli?sort=semver" /></a>
  <a href="https://github.com/CoreNovaLabs/cloud-forge-cli/actions/workflows/test.yml"><img alt="Test CLI" src="https://github.com/CoreNovaLabs/cloud-forge-cli/actions/workflows/test.yml/badge.svg" /></a>
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/badge/license-MIT-green.svg" /></a>
  <a href="go.mod"><img alt="Go" src="https://img.shields.io/github/go-mod/go-version/CoreNovaLabs/cloud-forge-cli" /></a>
</p>

```bash
curl -fsSL https://cdn.jsdelivr.net/gh/CoreNovaLabs/cloud-forge-cli@main/scripts/install.sh | bash
cloud-forge auth aws
cloud-forge deploy hello-nginx --cloud aws
```

Cloud Forge CLI is the command-line entry point for the [Cloud Forge Catalog](https://github.com/CoreNovaLabs/cloud-forge-catalog): find apps, inspect templates, deploy stacks on AWS or Aliyun, and remove them when you are done.

| Capability | What it does |
| --- | --- |
| Catalog search | Browse apps with `search` / `show`; parameters and cloud support come from each manifest. |
| AWS deploy | Create or update CloudFormation stacks and follow progress in the terminal. |
| Aliyun deploy | Deploy to Hong Kong (`cn-hongkong`) via ROS with ECS + EIP and container bootstrap. |
| Built-in auth | AWS browser sign-in or access keys; Aliyun AccessKey setup. |
| Cleanup | Delete Cloud Forge stacks and release associated cloud resources. |

**Catalog notes**

- Index: [cloud-forge-catalog/index/apps.json](https://github.com/CoreNovaLabs/cloud-forge-catalog/blob/main/index/apps.json)
- Any app with `aws` or `aliyun` in `clouds` can be deployed; run `cloud-forge show <app>` for parameters
- `certified` apps have fuller cloud verification; `community` apps iterate faster
- Aliyun v1 uses public OS images with UserData bootstrap; first boot may take 8–15 minutes

Default regions: AWS `us-east-1`, Aliyun `cn-hongkong`.

## Install

The one-liner above installs `cloud-forge` into `~/.local/bin`. Add that directory to your `PATH` if your shell cannot find the command.

If the CDN is unavailable in your network, use the GitHub raw URL instead:

```bash
curl -fsSL https://raw.githubusercontent.com/CoreNovaLabs/cloud-forge-cli/main/scripts/install.sh | bash
```

Manual install from [GitHub Releases](https://github.com/CoreNovaLabs/cloud-forge-cli/releases): unpack the archive and move the binary to a directory on your `PATH`.

Verify:

```bash
cloud-forge version
```

See [Build From Source](#build-from-source) to compile locally.

## AWS Credentials

Cloud Forge CLI calls AWS through the AWS SDK for Go v2. It includes a browser-based sign-in flow, so you do not need the AWS CLI installed—but you do need an AWS identity or access keys.

```bash
cloud-forge auth aws
cloud-forge auth aws status
```

By default, `cloud-forge auth aws` opens an AWS sign-in page and writes temporary credentials to a local profile (AWS Sign-In OAuth with PKCE). Use `--no-browser` to print the URL and paste the authorization code manually. Use `--profile NAME` to target a specific profile.

Other supported credential sources: `~/.aws/credentials`, `~/.aws/config`, `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY`, `AWS_PROFILE`, AWS SSO or assume-role profiles, and EC2/ECS instance or task roles.

```bash
export AWS_PROFILE=default
# or
export AWS_ACCESS_KEY_ID="..."
export AWS_SECRET_ACCESS_KEY="..."
```

Override the default deploy region when needed:

```bash
cloud-forge deploy hello-nginx --cloud aws --region us-west-2
```

Auth method variants:

```bash
cloud-forge auth aws --method browser          # default
cloud-forge auth aws --method browser --no-browser
cloud-forge auth aws --method access-key
```

For production use, prefer a least-privilege IAM user or role. Avoid AWS account root credentials.

## Quick Start

```bash
cloud-forge search nginx --cloud aws
cloud-forge show hello-nginx
cloud-forge template hello-nginx --cloud aws
cloud-forge deploy hello-nginx --cloud aws --dry-run
cloud-forge deploy hello-nginx --cloud aws
```

Without `--allowed-ip`, SSH defaults to `0.0.0.0/0` and the CLI prints a security warning. Restrict access with `--allowed-ip <your-ip>/32` when needed.

When the stack finishes, the CLI prints outputs and a cleanup hint:

```text
To remove later: cloud-forge delete cloud-forge-hello-nginx --cloud aws --region us-east-1
```

For Aliyun, see [Aliyun Deploy (Hong Kong)](#aliyun-deploy-hong-kong).

## Cleanup

```bash
cloud-forge delete cloud-forge-hello-nginx --cloud aws --region us-east-1
cloud-forge delete cloud-forge-<app-id> --cloud aws --wait      # default: wait for completion
cloud-forge delete cloud-forge-<app-id> --cloud aws --no-wait   # return immediately
```

Deleting the stack removes the EC2 instance, Elastic IP, security group, and related resources created by the template.

## AWS Deploy

AWS deployment uses the AWS SDK for Go v2 and CloudFormation—not the AWS CLI underneath. Credentials come from the standard AWS SDK chain. Default region is `us-east-1` (`--region` to override).

Create or update a stack:

```bash
cloud-forge deploy hello-nginx --cloud aws \
  --stack-name cloud-forge-hello-nginx \
  --instance-type t3.micro
```

By default, deploy prints CloudFormation resource events:

```text
[12:01:08] AWS::EC2::SecurityGroup HelloSecurityGroup CREATE_COMPLETE
[12:01:15] AWS::EC2::Instance HelloInstance CREATE_IN_PROGRESS
```

```bash
cloud-forge deploy hello-nginx --cloud aws --progress none   # disable progress lines
```

After `CREATE_COMPLETE`, `deploy` polls `ServiceURL` (`/health` and `/`) until the app responds or `--timeout` is reached. First boot still pulls images and obtains TLS, so this bridges stack completion and a reachable endpoint. Pass `--no-wait-ready` to return right after the stack is created.

### SSH Key Behavior

By default, AWS deploys use a reusable local key at `~/.cloud-forge/keys/aws/cloud-forge-default.pem`. On first use the CLI creates it with `0600` permissions and imports the public key into EC2 as `cloud-forge-default` when missing in the target region. The same local key is reused across regions. Deleting a stack does not remove this file.

```bash
cloud-forge deploy hello-nginx --cloud aws --key-name my-key
cloud-forge deploy hello-nginx --cloud aws --ssh-key none
cloud-forge deploy hello-nginx --cloud aws --ssh-key-path ~/.cloud-forge/keys/aws/custom.pem
```

## Aliyun Deploy (Hong Kong)

Aliyun v1 uses ROS to create ECS + EIP, then bootstraps Docker/Caddy and the app container via UserData. Unlike AWS pre-baked AMIs, expect **8–15 minutes** before the service is reachable.

Only **`cn-hongkong`** is supported. You need a VPC, VSwitch, and SSH KeyPair in that region.

```bash
cloud-forge auth aliyun
cloud-forge auth aliyun status

cloud-forge deploy hello-nginx --cloud aliyun --region cn-hongkong \
  --vpc-id vpc-xxx \
  --vswitch-id vsw-xxx \
  --key my-key \
  --timeout 20m

# Stack only — do not wait for bootstrap
cloud-forge deploy hello-nginx --cloud aliyun --region cn-hongkong \
  --vpc-id vpc-xxx --vswitch-id vsw-xxx --key my-key --no-wait-ready

cloud-forge delete cloud-forge-hello-nginx --cloud aliyun --region cn-hongkong
```

After ROS `CREATE_COMPLETE`, `deploy` polls `ServiceURL` the same way as AWS. Pass `--no-wait-ready` to skip app readiness waiting. Container images use Docker Hub short names (reachable from Hong Kong ECS without ACR).

## Common Options

```bash
cloud-forge deploy hello-nginx --cloud aws \
  --region us-east-1 \
  --stack-name cloud-forge-hello-nginx \
  --instance-type t3.micro \
  --allowed-ip 1.2.3.4/32 \
  --progress plain
```

Template parameters can use dedicated flags or repeated `--param`:

```bash
cloud-forge deploy hello-nginx --cloud aws \
  --param KeyName=my-key \
  --param InstanceType=t3.micro
```

Common deploy flags (each app may expose more—run `cloud-forge show <app>`):

- **Instance:** `--instance-type`, `--disk-size`, `--image-id`, `--latest-ami-id`
- **Network:** `--vpc` / `--vpc-id`, `--subnet` / `--subnet-id`, `--vswitch-id`, `--allowed-ip`
- **SSH / keys:** `--key` / `--key-name`, `--ssh-key`, `--ssh-key-path`
- **DNS / TLS:** `--domain`, `--hosted-zone-id`, `--caddy-tls-mode`
- **Other:** `--progress`, `--admin-password`

## Admin Password

Some apps (for example `code-server`, `minio`) require an admin password. `cloud-forge show <app>` lists `AdminPassword optional secret` when applicable.

```bash
cloud-forge deploy minio --cloud aws --admin-password 'MyStr0ngPass'
# or
cloud-forge deploy minio --cloud aws --param AdminPassword='MyStr0ngPass'
```

If omitted, the CLI generates a random 24-character password, passes it into IaC parameters, and **prints it once after a successful deploy** (`--dry-run` only notes that a password will be generated). Passwords are not written to stack outputs or telemetry.

## Catalog Reference

```text
https://cdn.jsdelivr.net/gh/CoreNovaLabs/cloud-forge-catalog@main/index/apps.json
```

If the default CDN mirror is unavailable, the CLI falls back to the GitHub raw catalog URL.

Local development:

```bash
export CLOUD_FORGE_STORE_URL="file:///absolute/path/to/cloud-forge-catalog/index/apps.json"
```

## Command Reference

```bash
cloud-forge search hello --cloud aws
cloud-forge show hello-nginx
cloud-forge template hello-nginx --cloud aws
cloud-forge deploy hello-nginx --cloud aws --dry-run
cloud-forge deploy hello-nginx --cloud aliyun --region cn-hongkong \
  --vpc-id vpc-xxx --vswitch-id vsw-xxx --key my-key --dry-run
cloud-forge auth aws status
cloud-forge auth aliyun status
cloud-forge delete cloud-forge-hello-nginx --cloud aws --region us-east-1
cloud-forge delete cloud-forge-hello-nginx --cloud aliyun --region cn-hongkong
cloud-forge help deploy
```

## Telemetry

By default, the CLI sends anonymous, non-blocking usage events to `https://telemetry.corenovacloud.com/v1/events`. Telemetry never includes cloud credentials, account IDs, domains, local paths, or template parameter values.

```bash
export CLOUD_FORGE_TELEMETRY=0
export CLOUD_FORGE_TELEMETRY_ENDPOINT="http://127.0.0.1:8787/v1/events"   # local testing
```

## Build From Source

```bash
go build ./cmd/cloud-forge
```

## Development

```bash
go test ./...
```
