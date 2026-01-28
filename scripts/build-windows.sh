#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
BUILD_DIR="$PROJECT_DIR/build/bin"
FFMPEG_DIR="$PROJECT_DIR/build/windows/Resources"
FFMPEG_SOURCE="$FFMPEG_DIR/ffmpeg.exe"

echo "🔨 Building YT Downloader for Windows..."

# Check if FFmpeg binary exists in build resources
if [ ! -f "$FFMPEG_SOURCE" ]; then
    echo "⬇️  FFmpeg for Windows not found. Downloading..."
    mkdir -p "$FFMPEG_DIR"
    
    # Download FFmpeg essentials build for Windows
    curl -L -o /tmp/ffmpeg-win.zip "https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip"
    
    # Extract and find ffmpeg.exe
    unzip -o /tmp/ffmpeg-win.zip -d /tmp/ffmpeg-win/
    
    # Find and copy ffmpeg.exe (it's in a versioned subfolder)
    FFMPEG_EXE=$(find /tmp/ffmpeg-win -name "ffmpeg.exe" | head -1)
    if [ -n "$FFMPEG_EXE" ]; then
        cp "$FFMPEG_EXE" "$FFMPEG_SOURCE"
        echo "✅ FFmpeg for Windows downloaded"
    else
        echo "❌ Failed to find ffmpeg.exe in download"
        exit 1
    fi
    
    # Cleanup
    rm -rf /tmp/ffmpeg-win /tmp/ffmpeg-win.zip
fi

# Build the Wails app for Windows
cd "$PROJECT_DIR"
wails build -platform "windows/amd64"

# Create distribution folder with exe and ffmpeg
DIST_DIR="$BUILD_DIR/YT-Downloader-Windows"
rm -rf "$DIST_DIR" 2>/dev/null || true
mkdir -p "$DIST_DIR"

echo "📦 Creating Windows distribution..."

# Wails creates yt-downloader.exe when building for single platform
if [ -f "$BUILD_DIR/yt-downloader.exe" ]; then
    mv "$BUILD_DIR/yt-downloader.exe" "$DIST_DIR/YT Downloader.exe"
elif [ -f "$BUILD_DIR/yt-downloader-amd64.exe" ]; then
    mv "$BUILD_DIR/yt-downloader-amd64.exe" "$DIST_DIR/YT Downloader.exe"
else
    echo "❌ Could not find Windows exe"
    exit 1
fi

cp "$FFMPEG_SOURCE" "$DIST_DIR/ffmpeg.exe"

# Create zip
echo "📦 Creating Windows zip..."
cd "$BUILD_DIR"
rm -f "YT-Downloader-Windows.zip" 2>/dev/null || true
zip -r "YT-Downloader-Windows.zip" "YT-Downloader-Windows"
rm -rf "YT-Downloader-Windows"
cd "$PROJECT_DIR"

# Verify
echo ""
echo "✅ Windows build complete!"
echo ""
echo "Built package:"
ls -lh "$BUILD_DIR/YT-Downloader-Windows.zip" 2>/dev/null || echo "  No zip file found"
echo ""
