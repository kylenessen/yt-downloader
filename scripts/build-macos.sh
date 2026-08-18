#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
BUILD_DIR="$PROJECT_DIR/build/bin"
TOOLCHAIN_DIR="$PROJECT_DIR/build/toolchains"
TOOLCHAIN_ARM64="$TOOLCHAIN_DIR/toolchain-darwin-arm64.zip"
TOOLCHAIN_AMD64="$TOOLCHAIN_DIR/toolchain-darwin-amd64.zip"
SIGN_IDENTITY="${APPLE_SIGN_IDENTITY:--}"
NOTARY_PROFILE="${APPLE_NOTARY_PROFILE:-}"

echo "Building self-contained YT Downloader packages for macOS"
mkdir -p "$BUILD_DIR"

cd "$PROJECT_DIR"
wails build -clean -platform "darwin/amd64,darwin/arm64"

mkdir -p "$TOOLCHAIN_DIR"
"$SCRIPT_DIR/build-toolchain.sh" darwin-arm64 "$TOOLCHAIN_ARM64"
"$SCRIPT_DIR/build-toolchain.sh" darwin-amd64 "$TOOLCHAIN_AMD64"

package_app() {
    local arch="$1"
    local toolchain_archive="$2"
    local zip_name="$3"
    local source_app="$BUILD_DIR/yt-downloader-${arch}.app"
    local destination_app="$BUILD_DIR/YT Downloader.app"
    local destination_zip="$BUILD_DIR/$zip_name"

    if [ ! -d "$source_app" ]; then
        echo "Expected Wails output is missing: $source_app"
        exit 1
    fi

    rm -rf "$destination_app"
    mv "$source_app" "$destination_app"
    cp "$toolchain_archive" "$destination_app/Contents/Resources/toolchain.zip"
    cp "$PROJECT_DIR/THIRD_PARTY_NOTICES.md" "$destination_app/Contents/Resources/THIRD_PARTY_NOTICES.md"

    xattr -cr "$destination_app"
    echo "Signing $arch package with identity $SIGN_IDENTITY"
    if [ "$SIGN_IDENTITY" = "-" ]; then
        codesign --force --deep --sign - --options runtime "$destination_app"
    else
        codesign --force --deep --sign "$SIGN_IDENTITY" --options runtime --timestamp "$destination_app"
    fi
    codesign --verify --deep --strict --verbose=2 "$destination_app"

    if [ -n "$NOTARY_PROFILE" ]; then
        local submission_zip="$BUILD_DIR/.notary-${arch}.zip"
        rm -f "$submission_zip"
        ditto -c -k --keepParent "$destination_app" "$submission_zip"
        xcrun notarytool submit "$submission_zip" --keychain-profile "$NOTARY_PROFILE" --wait
        xcrun stapler staple "$destination_app"
        xcrun stapler validate "$destination_app"
        rm -f "$submission_zip"
    fi

    rm -f "$destination_zip"
    ditto -c -k --keepParent "$destination_app" "$destination_zip"
    rm -rf "$destination_app"
    echo "Created $destination_zip"
}

package_app arm64 "$TOOLCHAIN_ARM64" "YT-Downloader-macOS-Apple-Silicon.zip"
package_app amd64 "$TOOLCHAIN_AMD64" "YT-Downloader-macOS-Intel.zip"

ls -lh "$BUILD_DIR/"YT-Downloader-macOS-*.zip
