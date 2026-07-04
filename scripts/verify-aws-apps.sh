#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

export CLOUD_FORGE_TELEMETRY=0
export CLOUD_FORGE_STORE_URL="${CLOUD_FORGE_STORE_URL:-file:///Users/zhengyihui/代码/开源项目/cloud-forge/cloud-forge-catalog/index/apps.json}"

poll_health() {
  local url="$1"
  local host="${url#*://}"
  host="${host%%/*}"
  local probe
  local i
  for i in $(seq 1 40); do
    for probe in "${url%/}/health" "https://${host}/health" "http://${host}/health"; do
      if curl -sk --max-time 10 "$probe" 2>/dev/null | grep -q ok; then
        echo "HEALTH OK $probe"
        return 0
      fi
    done
    echo "  poll $i waiting..."
    sleep 15
  done
  echo "HEALTH FAIL $url (also tried http://${host}/health)"
  return 1
}

for app in hello-nginx gitea n8n uptime-kuma; do
  echo "======== AWS DEPLOY $app ========"
  out="$(mktemp)"
  if ! ./cloud-forge deploy "$app" --cloud aws --region us-east-1 \
    --store-url "$CLOUD_FORGE_STORE_URL" --cache-ttl 0 \
    --timeout 20m --progress plain 2>&1 | tee "$out"; then
    echo "DEPLOY FAIL $app"
    rm -f "$out"
    exit 1
  fi
  url="$(grep 'ServiceURL' "$out" | tail -1 | awk '{print $2}')"
  echo "ServiceURL=$url"
  if [[ -z "$url" ]]; then
    echo "missing ServiceURL in deploy output"
    rm -f "$out"
    exit 1
  fi
  poll_health "$url"
  echo "======== AWS DELETE $app ========"
  ./cloud-forge delete "cloud-forge-$app" --cloud aws --region us-east-1 --timeout 15m --progress none
  rm -f "$out"
done

echo "ALL AWS PASSED"
