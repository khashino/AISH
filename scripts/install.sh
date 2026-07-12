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

case "$VERSION" in
v[0-9]*)
;;
*)
echo "Unable to determine the latest release version." >&2
echo "GitHub returned: $LATEST_URL" >&2
exit 1
;;
esac
else
VERSION="$REQUESTED_VERSION"

case "$VERSION" in
v*)
;;
*)
VERSION="v$VERSION"
;;
esac
fi

VERSION_NUMBER="${VERSION#v}"
ASSET="aish-v${VERSION_NUMBER}-${OS}-${ARCH}"
DOWNLOAD_URL="https://github.com/$REPO/releases/download/$VERSION/$ASSET"

mkdir -p "$INSTALL_DIR"

TMP_FILE="$(mktemp)"
trap 'rm -f "$TMP_FILE"' EXIT INT TERM

echo "Downloading AISH $VERSION..."
echo "Asset: $ASSET"
echo "URL: $DOWNLOAD_URL"

if ! curl -fL --progress-bar "$DOWNLOAD_URL" -o "$TMP_FILE"; then
echo >&2
echo "Download failed." >&2
echo "Expected release asset:" >&2
echo "  $DOWNLOAD_URL" >&2
echo >&2
echo "Check that the release and asset exist on GitHub." >&2
exit 1
fi

chmod +x "$TMP_FILE"
mv "$TMP_FILE" "$INSTALL_DIR/aish"
trap - EXIT INT TERM

echo
echo "AISH $VERSION installed successfully:"
echo "  $INSTALL_DIR/aish"

case ":$PATH:" in
*":$INSTALL_DIR:"*)
;;
*)
echo
echo "Add AISH to PATH for this terminal:"
echo "  export PATH="$INSTALL_DIR:$PATH""
echo
echo "Make it permanent:"
echo "  echo 'export PATH="$INSTALL_DIR:$PATH"' >> ~/.bashrc"
;;
esac

echo
echo "Get started:"
echo "  aish setup"
echo "  aish doctor"
