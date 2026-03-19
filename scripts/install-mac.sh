#!/bin/bash
# YT Downloader - macOS Installer
# This script downloads, installs, and removes quarantine so you don't
# have to fight with Gatekeeper security prompts.

set -e

APP_NAME="YT Downloader"
INSTALL_DIR="/Applications"

# Detect architecture
ARCH=$(uname -m)
if [ "$ARCH" = "arm64" ]; then
    ZIP_NAME="YT-Downloader-macOS-Apple-Silicon.zip"
    echo "Detected Apple Silicon Mac"
elif [ "$ARCH" = "x86_64" ]; then
    ZIP_NAME="YT-Downloader-macOS-Intel.zip"
    echo "Detected Intel Mac"
else
    echo "Error: Unsupported architecture: $ARCH"
    exit 1
fi

DOWNLOAD_URL="https://github.com/kylenessen/yt-downloader/releases/latest/download/$ZIP_NAME"
TMP_DIR=$(mktemp -d)

cleanup() {
    rm -rf "$TMP_DIR"
}
trap cleanup EXIT

echo "Downloading $APP_NAME..."
curl -L -o "$TMP_DIR/$ZIP_NAME" "$DOWNLOAD_URL"

echo "Extracting..."
unzip -q "$TMP_DIR/$ZIP_NAME" -d "$TMP_DIR"

# Remove quarantine attributes (this is why we use a script - avoids Gatekeeper)
echo "Removing quarantine attributes..."
xattr -cr "$TMP_DIR/$APP_NAME.app"

# Move to Applications (may require password)
if [ -d "$INSTALL_DIR/$APP_NAME.app" ]; then
    echo "Removing existing installation..."
    rm -rf "$INSTALL_DIR/$APP_NAME.app"
fi

echo "Installing to $INSTALL_DIR..."
mv "$TMP_DIR/$APP_NAME.app" "$INSTALL_DIR/"

echo ""
echo "Done! $APP_NAME has been installed to $INSTALL_DIR."
echo "You can open it from your Applications folder or Launchpad."
echo ""

# Offer to open the app
read -p "Open $APP_NAME now? (y/n) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    open "$INSTALL_DIR/$APP_NAME.app"
fi
