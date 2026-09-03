#!/usr/bin/env bash
# install.sh — installs pastebin binary to /usr/local/bin
# Usage: curl -sSL https://github.com/apimgr/pastebin/raw/main/scripts/install.sh | bash
set -euo pipefail

REPO="apimgr/pastebin"
BINARY="pastebin"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

detect_platform() {
    local os arch
    os="$(uname -s | tr '[:upper:]' '[:lower:]')"
    arch="$(uname -m)"
    case "${arch}" in
        x86_64)  arch="amd64" ;;
        aarch64) arch="arm64" ;;
        arm64)   arch="arm64" ;;
        *)       echo "Unsupported architecture: ${arch}" >&2; exit 1 ;;
    esac
    echo "${os}-${arch}"
}

PLATFORM="$(detect_platform)"
VERSION="${VERSION:-$(curl -sSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)}"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${BINARY}-${PLATFORM}"

echo "Installing ${BINARY} ${VERSION} for ${PLATFORM}..."
curl -sSL "${URL}" -o "/tmp/${BINARY}"
chmod +x "/tmp/${BINARY}"

if [ -w "${INSTALL_DIR}" ]; then
    mv "/tmp/${BINARY}" "${INSTALL_DIR}/${BINARY}"
else
    sudo mv "/tmp/${BINARY}" "${INSTALL_DIR}/${BINARY}"
fi

echo "Installed ${BINARY} to ${INSTALL_DIR}/${BINARY}"
"${INSTALL_DIR}/${BINARY}" --version
