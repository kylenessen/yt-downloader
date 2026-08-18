#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "Building YT Downloader for all platforms"
"$SCRIPT_DIR/build-macos.sh"
"$SCRIPT_DIR/build-windows.sh"
echo "All packages created"
