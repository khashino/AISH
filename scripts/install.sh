#!/bin/sh
set -eu

REPO="${AISH_REPO:-khashino/AISH}"
REQUESTED_VERSION="${AISH_VERSION:-latest}"
INSTALL_DIR="${AISH_INSTALL_DIR:-$HOME/.local/bin}"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$OS" in
  linux|darwin)
    ;;
  *)
    echo "Unsupported operating system: $OS" >&2
    exit 1
    ;;
esac

case "$ARCH" in
  x86_64|amd64)
    ARCH="amd64"
    ;;
  aarch64|arm64)
    ARCH="arm64"
    ;;
  *)
    echo "Unsupported architecture: $ARCH" >&2
    exit 1
    ;;
esac

if [ "$REQUESTED_VERSION" = "latest" ]; then
  echo "Finding latest AISH release..."
  LATEST_URL="$(curl -fsSL -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest")"
  VERSION="${LATEST_URL##*/}"
else
  VERSION="$REQUESTED_VERSION"
fi

case "$VERSION" in
  v*) ;;
  *) VERSION="v$VERSION" ;;
esac

VERSION_NUMBER="${VERSION#v}"
ASSET="aish-v${VERSION_NUMBER}-${OS}-${ARCH}"
DOWNLOAD_URL="https://github.com/$REPO/releases/download/$VERSION/$ASSET"

mkdir -p "$INSTALL_DIR"

TMP_FILE="$(mktemp)"
trap 'rm -f "$TMP_FILE"' EXIT INT TERM

echo "Downloading $ASSET..."

curl -fL --progress-bar "$DOWNLOAD_URL" -o "$TMP_FILE"

chmod +x "$TMP_FILE"
mv "$TMP_FILE" "$INSTALL_DIR/aish"
trap - EXIT INT TERM

echo "Installed AISH $VERSION to $INSTALL_DIR/aish"

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    echo "Add this to your shell configuration:"
    echo "export PATH=\"$INSTALL_DIR:\$PATH\""
    ;;
esac