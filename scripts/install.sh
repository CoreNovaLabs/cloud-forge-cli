#!/usr/bin/env bash
set -euo pipefail

REPO="${CLOUD_FORGE_REPO:-CoreNovaLabs/cloud-forge-cli}"
INSTALL_DIR="${CLOUD_FORGE_INSTALL_DIR:-${HOME}/.local/bin}"
VERSION="${CLOUD_FORGE_VERSION:-latest}"
TMPDIR_CLOUD_FORGE_INSTALL=""

cleanup() {
  if [[ -n "${TMPDIR_CLOUD_FORGE_INSTALL:-}" ]]; then
    rm -rf "${TMPDIR_CLOUD_FORGE_INSTALL}"
  fi
}

usage() {
  cat <<EOF
Cloud Forge CLI installer

Usage:
  curl -fsSL https://cdn.jsdelivr.net/gh/${REPO}@main/scripts/install.sh | bash
  CLOUD_FORGE_VERSION=v0.2.0 bash install.sh

Windows PowerShell:
  irm https://cdn.jsdelivr.net/gh/${REPO}@main/scripts/install.ps1 | iex

Environment:
  CLOUD_FORGE_REPO          GitHub repository (default: CoreNovaLabs/cloud-forge-cli)
  CLOUD_FORGE_VERSION       Release tag or "latest" (default: latest)
  CLOUD_FORGE_INSTALL_DIR   Install directory (default: ~/.local/bin)
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

detect_platform() {
  local os arch
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m)"
  case "${os}" in
    darwin) os="darwin" ;;
    linux) os="linux" ;;
    *)
      echo "Unsupported operating system: ${os}" >&2
      echo "Download a release manually: https://github.com/${REPO}/releases" >&2
      exit 1
      ;;
  esac
  case "${arch}" in
    x86_64|amd64) arch="amd64" ;;
    arm64|aarch64) arch="arm64" ;;
    *)
      echo "Unsupported architecture: ${arch}" >&2
      echo "Download a release manually: https://github.com/${REPO}/releases" >&2
      exit 1
      ;;
  esac
  echo "${os}_${arch}"
}

resolve_version() {
  if [[ "${VERSION}" != "latest" ]]; then
    echo "${VERSION}"
    return
  fi
  curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1
}

main() {
  need_cmd() {
    if ! command -v "$1" >/dev/null 2>&1; then
      echo "Missing required command: $1" >&2
      exit 1
    fi
  }

  need_cmd curl
  need_cmd tar
  need_cmd install

  local platform resolved_version archive_name download_url
  platform="$(detect_platform)"
  resolved_version="$(resolve_version)"
  if [[ -z "${resolved_version}" ]]; then
    echo "Could not resolve latest release for ${REPO}" >&2
    exit 1
  fi

  local version_tag="${resolved_version#v}"
  archive_name="cloud-forge_${version_tag}_${platform}.tar.gz"
  download_url="https://github.com/${REPO}/releases/download/${resolved_version}/${archive_name}"

  TMPDIR_CLOUD_FORGE_INSTALL="$(mktemp -d)"
  trap cleanup EXIT

  echo "Installing cloud-forge ${resolved_version} for ${platform}"
  curl -fsSL "${download_url}" -o "${TMPDIR_CLOUD_FORGE_INSTALL}/${archive_name}"
  tar -xzf "${TMPDIR_CLOUD_FORGE_INSTALL}/${archive_name}" -C "${TMPDIR_CLOUD_FORGE_INSTALL}"

  mkdir -p "${INSTALL_DIR}"
  install -m 0755 "${TMPDIR_CLOUD_FORGE_INSTALL}/cloud-forge" "${INSTALL_DIR}/cloud-forge"

  echo "Installed cloud-forge to ${INSTALL_DIR}/cloud-forge"
  if ! echo ":${PATH}:" | grep -q ":${INSTALL_DIR}:"; then
    echo "Add ${INSTALL_DIR} to your PATH, for example:"
    echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
  fi
  "${INSTALL_DIR}/cloud-forge" version
}

main "$@"
