# Cloud Forge CLI — Smoke Test Checklist

Use this checklist before tagging a release. Live AWS tests require an account with CloudFormation and EC2 permissions.

## Local (no AWS)

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
done
```

## Live AWS (manual)

```bash
cloud-forge auth aws
cloud-forge auth aws status

for app in hello-nginx gitea n8n uptime-kuma; do
  cloud-forge deploy "$app" --cloud aws --allowed-ip <YOUR_IP>/32
  # open ServiceURL from deploy output
  cloud-forge delete "cloud-forge-$app" --cloud aws --wait
done
```

Record the AWS account, region, date, and result for each app when completing live verification.
