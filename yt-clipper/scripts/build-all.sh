#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "🚀 Building YouTube Clipper for all platforms..."
echo ""

# Build macOS
echo "=========================================="
"$SCRIPT_DIR/build-macos.sh"
echo ""

# Build Windows
echo "=========================================="
"$SCRIPT_DIR/build-windows.sh"
echo ""

echo "=========================================="
echo "🎉 All builds complete!"
echo "=========================================="
