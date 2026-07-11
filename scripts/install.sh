#!/bin/sh
set -eu
REPO=${AISH_REPO:-khashino/AISH}
VERSION=${AISH_VERSION:-latest}
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in x86_64|amd64) ARCH=amd64;; aarch64|arm64) ARCH=arm64;; *) echo "unsupported arch: $ARCH"; exit 1;; esac
ASSET="aish-${OS}-${ARCH}"
[ "$OS" = windows ] && ASSET="$ASSET.exe"
BASE="https://github.com/$REPO/releases"
if [ "$VERSION" = latest ]; then URL="$BASE/latest/download/$ASSET"; else URL="$BASE/download/$VERSION/$ASSET"; fi
DEST=${AISH_INSTALL_DIR:-$HOME/.local/bin}
mkdir -p "$DEST"
curl -fL "$URL" -o "$DEST/aish"
chmod +x "$DEST/aish"
echo "Installed AISH to $DEST/aish"
