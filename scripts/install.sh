#!/usr/bin/env bash
# opsmate installer
# Usage: curl -fsSL https://raw.githubusercontent.com/paffin/opsmate/main/scripts/install.sh | bash

set -euo pipefail

REPO="paffin/opsmate"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

# Detect OS and arch
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
  x86_64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

# Get latest version
VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')

if [ -z "$VERSION" ]; then
  echo "Failed to get latest version"
  exit 1
fi

echo "Installing opsmate ${VERSION} for ${OS}/${ARCH}..."

# Download
URL="https://github.com/${REPO}/releases/download/${VERSION}/opsmate_${VERSION#v}_${OS}_${ARCH}.tar.gz"
TMP=$(mktemp -d)
trap "rm -rf $TMP" EXIT

curl -fsSL "$URL" -o "$TMP/opsmate.tar.gz"
tar -xzf "$TMP/opsmate.tar.gz" -C "$TMP"

# Install
if [ -w "$INSTALL_DIR" ]; then
  mv "$TMP/opsmate" "$INSTALL_DIR/opsmate"
else
  sudo mv "$TMP/opsmate" "$INSTALL_DIR/opsmate"
fi

chmod +x "$INSTALL_DIR/opsmate"

echo "opsmate ${VERSION} installed to ${INSTALL_DIR}/opsmate"
echo ""
echo "Get started:"
echo "  opsmate          # Launch with DevOps superpowers"
echo "  opsmate status   # Check infrastructure connectivity"
