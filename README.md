<p align="center">
  <img src="assets/cloud-forge-logo.png" alt="Cloud Forge CLI logo" width="160" />
</p>

<h1 align="center">Cloud Forge CLI</h1>

Cloud Forge CLI is the command-line client for the Cloud Forge catalog. The current foundation supports catalog search, app inspection, template download, and AWS CloudFormation deployment.

## Build

```bash
go build ./cmd/cloud-forge
```

## Catalog

By default the CLI reads:

```text
https://cdn.jsdelivr.net/gh/CoreNovaLabs/cloud-forge-catalog@main/index/apps.json
```

If the default mirror is unavailable, the CLI falls back to the GitHub raw catalog URL.

For local development:

```bash
export CLOUD_FORGE_STORE_URL="file:///absolute/path/to/cloud-forge-catalog/index/apps.json"
```

## Commands

```bash
cloud-forge search hello --cloud aws
cloud-forge show hello-nginx
cloud-forge template hello-nginx --cloud aws
cloud-forge deploy hello-nginx --cloud aws --dry-run
```

## AWS Deploy

AWS deployment uses the AWS SDK for Go v2 and CloudFormation. It does not shell out to the AWS CLI.

Credentials are loaded from the normal AWS SDK chain. AWS deploys default to `us-east-1`; override with `--region` when needed.

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

By default, AWS deploys use a local reusable SSH key:

```text
~/.cloud-forge/keys/aws/cloud-forge-default.pem
```

The CLI creates this private key on first use with `0600` permissions and imports the public key into EC2 as `cloud-forge-default` when the target AWS region does not already have that key pair. The same local private key is reused across regions.

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

Template parameters can be passed either with dedicated flags or repeated `--param` flags:

```bash
cloud-forge deploy gitea --cloud aws \
  --region us-east-1 \
  --param KeyName=my-key \
  --param ImageId=ami-0123456789abcdef0
```

Supported dedicated AWS parameter flags include `--instance-type`, `--key`, `--key-name`, `--ssh-key`, `--ssh-key-path`, `--progress`, `--domain`, `--hosted-zone-id`, `--disk-size`, `--vpc`, `--subnet`, `--allowed-ip`, `--image-id`, and `--latest-ami-id`.

## Telemetry

The CLI sends anonymous, non-blocking usage events to:

```text
https://telemetry.corenovacloud.com/v1/events
```

Telemetry does not include cloud credentials, account IDs, domains, local paths, or template parameter values.

Disable it when needed:

```bash
export CLOUD_FORGE_TELEMETRY=0
```

Use a different endpoint for local testing:

```bash
export CLOUD_FORGE_TELEMETRY_ENDPOINT="http://127.0.0.1:8787/v1/events"
```

## Development

```bash
go test ./...
```
