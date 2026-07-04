# Cloud Forge CLI — Smoke Test Checklist

Use this checklist before tagging a release. Live cloud tests require accounts with CloudFormation/ROS and compute permissions.

## Local (no cloud)

```bash
cd cloud-forge-cli
go test ./...

cd ../cloud-forge-catalog
make validate
make index
git diff --exit-code index/apps.json

export CLOUD_FORGE_STORE_URL="file://$(pwd)/index/apps.json"
export CLOUD_FORGE_TELEMETRY=0

for app in hello-nginx gitea n8n uptime-kuma; do
  cloud-forge show "$app"
  cloud-forge deploy "$app" --cloud aws --dry-run
  cloud-forge deploy "$app" --cloud aliyun --region cn-hongkong \
    --vpc-id vpc-test --vswitch-id vsw-test --key test-key --dry-run
done
```

## Live AWS (manual)

```bash
cloud-forge auth aws
cloud-forge auth aws status

for app in hello-nginx gitea n8n uptime-kuma; do
  cloud-forge deploy "$app" --cloud aws --allowed-ip <YOUR_IP>/32
  # open ServiceURL from deploy output
  cloud-forge delete "cloud-forge-$app" --cloud aws
done
```

## Live Aliyun Hong Kong (manual)

Prerequisites in `cn-hongkong`:

- VPC with at least one VSwitch
- ECS KeyPair for SSH
- RAM user/role with ROS, ECS, VPC, and EIP permissions
- ROS service-linked role configured (CLI prints guidance if missing)

```bash
cloud-forge auth aliyun
cloud-forge auth aliyun status

cloud-forge deploy hello-nginx --cloud aliyun --region cn-hongkong \
  --vpc-id vpc-xxx \
  --vswitch-id vsw-xxx \
  --key my-key \
  --allowed-ip <YOUR_IP>/32 \
  --timeout 20m

# CLI waits for /health by default; use --no-wait-ready to skip bootstrap polling

cloud-forge delete cloud-forge-hello-nginx --cloud aliyun --region cn-hongkong
```

Repeat for `gitea`, `n8n`, and `uptime-kuma` when validating the full catalog.

Record the cloud account, region, date, and result for each app when completing live verification.
