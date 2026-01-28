#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
BUILD_DIR="$PROJECT_DIR/build/bin"
FFMPEG_SOURCE="$PROJECT_DIR/build/darwin/Resources/ffmpeg"

echo "🔨 Building YT Downloader for macOS (Intel + Apple Silicon)..."

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

# Rename and bundle FFmpeg into both app variants
echo "📦 Packaging apps..."

# Apple Silicon (arm64)
if [ -d "$BUILD_DIR/yt-downloader-arm64.app" ]; then
    rm -rf "$BUILD_DIR/YT Downloader.app" 2>/dev/null || true
    mv "$BUILD_DIR/yt-downloader-arm64.app" "$BUILD_DIR/YT Downloader.app"
    cp "$FFMPEG_SOURCE" "$BUILD_DIR/YT Downloader.app/Contents/Resources/ffmpeg"
    chmod +x "$BUILD_DIR/YT Downloader.app/Contents/Resources/ffmpeg"
    
    # Create zip for Apple Silicon
    echo "📦 Creating Apple Silicon zip..."
    cd "$BUILD_DIR"
    rm -f "YT-Downloader-macOS-Apple-Silicon.zip" 2>/dev/null || true
    zip -r "YT-Downloader-macOS-Apple-Silicon.zip" "YT Downloader.app"
    rm -rf "YT Downloader.app"
    cd "$PROJECT_DIR"
fi

# Intel (amd64)
if [ -d "$BUILD_DIR/yt-downloader-amd64.app" ]; then
    rm -rf "$BUILD_DIR/YT Downloader.app" 2>/dev/null || true
    mv "$BUILD_DIR/yt-downloader-amd64.app" "$BUILD_DIR/YT Downloader.app"
    cp "$FFMPEG_SOURCE" "$BUILD_DIR/YT Downloader.app/Contents/Resources/ffmpeg"
    chmod +x "$BUILD_DIR/YT Downloader.app/Contents/Resources/ffmpeg"
    
    # Create zip for Intel
    echo "📦 Creating Intel zip..."
    cd "$BUILD_DIR"
    rm -f "YT-Downloader-macOS-Intel.zip" 2>/dev/null || true
    zip -r "YT-Downloader-macOS-Intel.zip" "YT Downloader.app"
    rm -rf "YT Downloader.app"
    cd "$PROJECT_DIR"
fi

# Verify
echo ""
echo "✅ macOS builds complete!"
echo ""
echo "Built packages:"
ls -lh "$BUILD_DIR/"*.zip 2>/dev/null || echo "  No zip files found"
echo ""
