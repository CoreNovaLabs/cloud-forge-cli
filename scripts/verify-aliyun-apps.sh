#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CATALOG_ROOT="$(cd "$ROOT/../cloud-forge-catalog" && pwd)"
cd "$ROOT"

export CLOUD_FORGE_TELEMETRY=0
export CLOUD_FORGE_STORE_URL="${CLOUD_FORGE_STORE_URL:-file://${CATALOG_ROOT}/index/apps.json}"

REGION="${ALIYUN_REGION:-cn-hongkong}"
VPC_ID="${ALIYUN_VPC_ID:-}"
VSWITCH_ID="${ALIYUN_VSWITCH_ID:-}"
KEY_NAME="${ALIYUN_KEY_NAME:-}"

discover_resources() {
  local line
  while IFS= read -r line; do
    case "$line" in
      VPC\ *)
        VPC_ID="${line#VPC }"
        VPC_ID="${VPC_ID%% *}"
        ;;
      VSW\ *)
        if [[ -z "$VSWITCH_ID" ]]; then
          VSWITCH_ID="$(echo "$line" | awk '{print $2}')"
        fi
        ;;
      KEY\ *)
        if [[ -z "$KEY_NAME" ]]; then
          KEY_NAME="${line#KEY }"
        fi
        ;;
    esac
  done < <(go run scripts/list-aliyun-hk-resources.go 2>/dev/null || true)
}

if [[ -z "$VPC_ID" || -z "$VSWITCH_ID" || -z "$KEY_NAME" ]]; then
  discover_resources
fi

for var in VPC_ID VSWITCH_ID KEY_NAME; do
  if [[ -z "${!var}" ]]; then
    echo "error: missing Aliyun $var (set ALIYUN_${var} or ensure cn-hongkong VPC/VSwitch/KeyPair exist)" >&2
    exit 1
  fi
done

echo "Aliyun verify: region=$REGION vpc=$VPC_ID vswitch=$VSWITCH_ID key=$KEY_NAME"

poll_health() {
  local url="$1"
  local host="${url#*://}"
  host="${host%%/*}"
  local probe
  local i
  for i in $(seq 1 60); do
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
  echo "======== ALIYUN DEPLOY $app ========"
  out="$(mktemp)"
  if ! ./cloud-forge deploy "$app" --cloud aliyun --region "$REGION" \
    --vpc-id "$VPC_ID" --vswitch-id "$VSWITCH_ID" --key "$KEY_NAME" \
    --store-url "$CLOUD_FORGE_STORE_URL" --cache-ttl 0 \
    --timeout 25m --progress plain 2>&1 | tee "$out"; then
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
  echo "======== ALIYUN DELETE $app ========"
  ./cloud-forge delete "cloud-forge-$app" --cloud aliyun --region "$REGION" --timeout 20m --progress none
  rm -f "$out"
done

echo "ALL ALIYUN PASSED"
