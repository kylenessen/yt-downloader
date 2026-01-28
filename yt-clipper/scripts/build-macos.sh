#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
BUILD_DIR="$PROJECT_DIR/build/bin"
FFMPEG_SOURCE="$PROJECT_DIR/build/darwin/Resources/ffmpeg"

echo "🔨 Building YouTube Clipper for macOS (Intel + Apple Silicon)..."

# Check if FFmpeg binary exists in build resources
if [ ! -f "$FFMPEG_SOURCE" ]; then
    echo "⬇️  FFmpeg not found. Downloading..."
    mkdir -p "$PROJECT_DIR/build/darwin/Resources"
    curl -L -o /tmp/ffmpeg.zip "https://evermeet.cx/ffmpeg/getrelease/zip"
    unzip -o /tmp/ffmpeg.zip -d "$PROJECT_DIR/build/darwin/Resources/"
    chmod +x "$FFMPEG_SOURCE"
    rm /tmp/ffmpeg.zip
    echo "✅ FFmpeg downloaded"
fi

# Build the Wails app for both architectures
cd "$PROJECT_DIR"
wails build -platform "darwin/amd64,darwin/arm64"

# Bundle FFmpeg into both app variants
for ARCH in amd64 arm64; do
    APP_PATH="$BUILD_DIR/yt-clipper-${ARCH}.app"
    RESOURCES_DIR="$APP_PATH/Contents/Resources"
    
    if [ -d "$APP_PATH" ]; then
        echo "📦 Bundling FFmpeg into ${ARCH} app..."
        cp "$FFMPEG_SOURCE" "$RESOURCES_DIR/ffmpeg"
        chmod +x "$RESOURCES_DIR/ffmpeg"
    fi
done

# Verify
echo ""
echo "✅ macOS builds complete!"
echo ""
echo "Built applications:"
ls -lh "$BUILD_DIR/"*.app 2>/dev/null | while read line; do
    echo "  $line"
done
echo ""
echo "Intel (amd64) size: $(du -sh "$BUILD_DIR/yt-clipper-amd64.app" 2>/dev/null | cut -f1)"
echo "Apple Silicon (arm64) size: $(du -sh "$BUILD_DIR/yt-clipper-arm64.app" 2>/dev/null | cut -f1)"
