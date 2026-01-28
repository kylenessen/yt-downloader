#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
APP_PATH="$PROJECT_DIR/build/bin/yt-clipper.app"
RESOURCES_DIR="$APP_PATH/Contents/Resources"
FFMPEG_SOURCE="$PROJECT_DIR/build/darwin/Resources/ffmpeg"

echo "🔨 Building YouTube Clipper for macOS..."

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

# Build the Wails app
cd "$PROJECT_DIR"
wails build

# Copy FFmpeg into the app bundle
echo "📦 Bundling FFmpeg into app..."
cp "$FFMPEG_SOURCE" "$RESOURCES_DIR/ffmpeg"
chmod +x "$RESOURCES_DIR/ffmpeg"

# Verify
echo ""
echo "✅ Build complete!"
echo ""
echo "App location: $APP_PATH"
echo "App size: $(du -sh "$APP_PATH" | cut -f1)"
echo ""
echo "Contents:"
ls -lh "$RESOURCES_DIR/"
