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
        
        # Bundle FFmpeg
        local DEST_FFMPEG="$DEST_APP/Contents/Resources/ffmpeg"
        cp "$FFMPEG_SOURCE" "$DEST_FFMPEG"
        chmod +x "$DEST_FFMPEG"
        
        echo "🔏 Code signing (ad-hoc)..."
        # 1. Sign the nested ffmpeg binary first
        codesign --force --sign - --options runtime "$DEST_FFMPEG"
        
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
