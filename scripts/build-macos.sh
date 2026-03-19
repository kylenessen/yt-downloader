#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
BUILD_DIR="$PROJECT_DIR/build/bin"
RESOURCES_DIR="$PROJECT_DIR/build/darwin/Resources"

# FFmpeg static builds from GitHub (architecture-specific)
FFMPEG_ARM64_URL="https://github.com/eugeneware/ffmpeg-static/releases/download/b6.1.1/ffmpeg-darwin-arm64.gz"
FFMPEG_AMD64_URL="https://github.com/eugeneware/ffmpeg-static/releases/download/b6.1.1/ffmpeg-darwin-x64.gz"

echo "🔨 Building YT Downloader for macOS (Intel + Apple Silicon)..."

mkdir -p "$RESOURCES_DIR"

# Download architecture-specific FFmpeg binaries
if [ ! -f "$RESOURCES_DIR/ffmpeg-arm64" ]; then
    echo "⬇️  FFmpeg (ARM64) not found. Downloading..."
    curl -L -o /tmp/ffmpeg-arm64.gz "$FFMPEG_ARM64_URL"
    gunzip -c /tmp/ffmpeg-arm64.gz > "$RESOURCES_DIR/ffmpeg-arm64"
    chmod +x "$RESOURCES_DIR/ffmpeg-arm64"
    rm /tmp/ffmpeg-arm64.gz
    echo "✅ FFmpeg (ARM64) downloaded"
fi

if [ ! -f "$RESOURCES_DIR/ffmpeg-amd64" ]; then
    echo "⬇️  FFmpeg (Intel) not found. Downloading..."
    curl -L -o /tmp/ffmpeg-amd64.gz "$FFMPEG_AMD64_URL"
    gunzip -c /tmp/ffmpeg-amd64.gz > "$RESOURCES_DIR/ffmpeg-amd64"
    chmod +x "$RESOURCES_DIR/ffmpeg-amd64"
    rm /tmp/ffmpeg-amd64.gz
    echo "✅ FFmpeg (Intel) downloaded"
fi

# Check if yt-dlp binary exists (universal macOS binary)
YTDLP_SOURCE="$RESOURCES_DIR/yt-dlp"
if [ ! -f "$YTDLP_SOURCE" ]; then
    echo "⬇️  yt-dlp not found. Downloading..."
    curl -L -o "$YTDLP_SOURCE" "https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp_macos"
    chmod +x "$YTDLP_SOURCE"
    echo "✅ yt-dlp downloaded"
fi

# Build the Wails app for both architectures
cd "$PROJECT_DIR"
wails build -platform "darwin/amd64,darwin/arm64"

# Function to package and sign an app
package_app() {
    local ARCH=$1
    local ZIP_NAME=$2
    local SOURCE_APP="$BUILD_DIR/yt-downloader-${ARCH}.app"
    local DEST_APP="$BUILD_DIR/YT Downloader.app"

    if [ -d "$SOURCE_APP" ]; then
        echo "📦 Packaging ${ARCH} build..."

        # Remove any existing destination
        rm -rf "$DEST_APP" 2>/dev/null || true

        # Rename the app
        mv "$SOURCE_APP" "$DEST_APP"

        # Bundle architecture-specific FFmpeg
        local DEST_FFMPEG="$DEST_APP/Contents/Resources/ffmpeg"
        cp "$RESOURCES_DIR/ffmpeg-${ARCH}" "$DEST_FFMPEG"
        chmod +x "$DEST_FFMPEG"

        # Bundle yt-dlp
        local DEST_YTDLP="$DEST_APP/Contents/Resources/yt-dlp"
        cp "$YTDLP_SOURCE" "$DEST_YTDLP"
        chmod +x "$DEST_YTDLP"

        echo "🔏 Code signing (ad-hoc)..."
        # 1. Sign the nested binaries first
        codesign --force --sign - --options runtime "$DEST_FFMPEG"
        codesign --force --sign - --options runtime "$DEST_YTDLP"
        
        # 2. Sign the main app bundle
        codesign --force --deep --sign - --options runtime "$DEST_APP"
        
        # Verify the signature locally
        echo "🔍 Verifying signature..."
        codesign --verify --deep --strict "$DEST_APP" || echo "⚠️ Warning: Signature verification failed"
        
        # Remove any quarantine attributes (for local testing)
        xattr -cr "$DEST_APP"
        
        # Create zip
        echo "📦 Creating ${ZIP_NAME}..."
        cd "$BUILD_DIR"
        rm -f "$ZIP_NAME" 2>/dev/null || true
        zip -qr "$ZIP_NAME" "YT Downloader.app"
        rm -rf "YT Downloader.app"
        cd "$PROJECT_DIR"
        
        echo "✅ Created $ZIP_NAME"
    fi
}

# Package both architectures
package_app "arm64" "YT-Downloader-macOS-Apple-Silicon.zip"
package_app "amd64" "YT-Downloader-macOS-Intel.zip"

# Verify
echo ""
echo "✅ macOS builds complete!"
echo ""
echo "Built packages:"
ls -lh "$BUILD_DIR/"*.zip 2>/dev/null || echo "  No zip files found"
echo ""
