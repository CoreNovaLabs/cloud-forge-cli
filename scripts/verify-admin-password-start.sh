#!/usr/bin/env bash
# Verify CLI auto-generated AdminPassword can bootstrap and start catalog apps locally.
set -euo pipefail

CLI_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CAT_ROOT="${CLOUD_FORGE_CATALOG_ROOT:-$(cd "$CLI_ROOT/../cloud-forge-catalog" && pwd)}"
APPS=("$@")
if [[ ${#APPS[@]} -eq 0 ]]; then
  APPS=(code-server minio)
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "error: docker is required" >&2
  exit 1
fi

PW_FILE="$(mktemp)"
trap 'rm -f "$PW_FILE"' EXIT

echo "==> generating AdminPassword via cloud-forge-cli (configureAdminPassword)"
CF_ADMIN_PASSWORD_OUT="$PW_FILE" go test "$CLI_ROOT/internal/cli" -run '^TestExportGeneratedAdminPassword$' -count=1 >/dev/null
PW="$(<"$PW_FILE")"
if [[ ${#PW} -lt 8 ]]; then
  echo "error: generated password too short" >&2
  exit 1
fi
echo "  generated ${#PW}-char password"

echo "==> verifying deploy parameter wiring"
go test "$CLI_ROOT/internal/cli" -run '^TestGeneratedPasswordFlowsToDeployParameters$' -count=1 >/dev/null
echo "  OK deploy parameters include generated AdminPassword"

failed=0
for app in "${APPS[@]}"; do
  echo "==> local bootstrap + start: $app"
  if CLOUD_FORGE_SMOKE_ADMIN_PASSWORD="$PW" "$CAT_ROOT/scripts/local-smoke.sh" "$app"; then
    echo "PASS $app (CLI-generated password)"
  else
    echo "FAIL $app" >&2
    failed=1
  fi
done

exit "$failed"
