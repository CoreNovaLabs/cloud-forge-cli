#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CATALOG_ROOT="$(cd "$ROOT/../cloud-forge-catalog" && pwd)"
cd "$ROOT"

export CLOUD_FORGE_TELEMETRY=0
export CLOUD_FORGE_STORE_URL="${CLOUD_FORGE_STORE_URL:-file://${CATALOG_ROOT}/index/apps.json}"

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

APPS=($("$CATALOG_ROOT/scripts/list-verify-apps.sh"))
if [[ ${#APPS[@]} -eq 0 ]]; then
  echo "error: no apps selected for cloud verify (check CLOUD_FORGE_VERIFY_TIERS)" >&2
  exit 1
fi

echo "AWS verify tiers=${CLOUD_FORGE_VERIFY_TIERS:-certified} sample=${CLOUD_FORGE_VERIFY_SAMPLE:-0} apps=${APPS[*]}"

for app in "${APPS[@]}"; do
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
