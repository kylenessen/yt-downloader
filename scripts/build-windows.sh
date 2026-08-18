#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
BUILD_DIR="$PROJECT_DIR/build/bin"
TOOLCHAIN_DIR="$PROJECT_DIR/build/toolchains"
TOOLCHAIN_ARCHIVE="$TOOLCHAIN_DIR/toolchain-windows-amd64.zip"
DIST_DIR="$BUILD_DIR/YT-Downloader-Windows"
OUTPUT_ZIP="$BUILD_DIR/YT-Downloader-Windows.zip"

echo "Building self-contained YT Downloader package for Windows"
mkdir -p "$BUILD_DIR"

cd "$PROJECT_DIR"
wails build -clean -platform "windows/amd64"

mkdir -p "$TOOLCHAIN_DIR"
"$SCRIPT_DIR/build-toolchain.sh" windows-amd64 "$TOOLCHAIN_ARCHIVE"

rm -rf "$DIST_DIR"
mkdir -p "$DIST_DIR"

if [ -f "$BUILD_DIR/yt-downloader.exe" ]; then
    mv "$BUILD_DIR/yt-downloader.exe" "$DIST_DIR/YT Downloader.exe"
elif [ -f "$BUILD_DIR/yt-downloader-amd64.exe" ]; then
    mv "$BUILD_DIR/yt-downloader-amd64.exe" "$DIST_DIR/YT Downloader.exe"
else
    echo "Expected Wails Windows executable is missing"
    exit 1
fi

cp "$TOOLCHAIN_ARCHIVE" "$DIST_DIR/toolchain.zip"
cp "$PROJECT_DIR/THIRD_PARTY_NOTICES.md" "$DIST_DIR/THIRD_PARTY_NOTICES.md"

rm -f "$OUTPUT_ZIP"
(
    cd "$BUILD_DIR"
    zip -qr "$OUTPUT_ZIP" "$(basename "$DIST_DIR")"
)
rm -rf "$DIST_DIR"
echo "Created $OUTPUT_ZIP"
ls -lh "$OUTPUT_ZIP"
