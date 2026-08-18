#!/bin/bash
set -euo pipefail

if [ "$#" -ne 2 ]; then
    echo "Usage: $0 <darwin-arm64|darwin-amd64|windows-amd64> <output.zip>"
    exit 2
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
MANIFEST="$PROJECT_DIR/toolchain/manifest.json"
CACHE_DIR="${YT_DOWNLOADER_DOWNLOAD_CACHE:-$PROJECT_DIR/build/download-cache}"
PLATFORM="$1"
OUTPUT="$2"

for command_name in awk curl gzip jq shasum unzip zip; do
    if ! command -v "$command_name" >/dev/null 2>&1; then
        echo "Required build command is missing: $command_name"
        exit 1
    fi
done

if [ "$(jq -r --arg platform "$PLATFORM" '.platforms[$platform] != null' "$MANIFEST")" != "true" ]; then
    echo "Unknown toolchain platform: $PLATFORM"
    exit 2
fi

case "$OUTPUT" in
    /*) OUTPUT_PATH="$OUTPUT" ;;
    *) OUTPUT_PATH="$PWD/$OUTPUT" ;;
esac

WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/yt-downloader-toolchain.XXXXXX")"
cleanup() {
    rm -rf "$WORK_DIR"
}
trap cleanup EXIT
mkdir -p "$CACHE_DIR"

VERSION="$(jq -r '.version' "$MANIFEST")"
GOOS="$(jq -r --arg platform "$PLATFORM" '.platforms[$platform].goos' "$MANIFEST")"
GOARCH="$(jq -r --arg platform "$PLATFORM" '.platforms[$platform].goarch' "$MANIFEST")"

ARCHIVE_MANIFEST="$(jq -n \
    --arg version "$VERSION" \
    --arg goos "$GOOS" \
    --arg goarch "$GOARCH" \
    '{schemaVersion: 1, version: $version, goos: $goos, goarch: $goarch, tools: {}, files: {}}')"

for logical_name in yt-dlp ffmpeg ffprobe deno; do
    filename="$(jq -r --arg platform "$PLATFORM" --arg tool "$logical_name" '.platforms[$platform].tools[$tool].filename' "$MANIFEST")"
    url="$(jq -r --arg platform "$PLATFORM" --arg tool "$logical_name" '.platforms[$platform].tools[$tool].url' "$MANIFEST")"
    expected="$(jq -r --arg platform "$PLATFORM" --arg tool "$logical_name" '.platforms[$platform].tools[$tool].sha256' "$MANIFEST")"
    format="$(jq -r --arg platform "$PLATFORM" --arg tool "$logical_name" '.platforms[$platform].tools[$tool].format' "$MANIFEST")"
    entry="$(jq -r --arg platform "$PLATFORM" --arg tool "$logical_name" '.platforms[$platform].tools[$tool].entry // empty' "$MANIFEST")"
    download="$WORK_DIR/$logical_name.download"
    destination="$WORK_DIR/$filename"

    cached_download="$CACHE_DIR/$expected"
    if [ -f "$cached_download" ] && [ "$(shasum -a 256 "$cached_download" | awk '{print $1}')" = "$expected" ]; then
        cp "$cached_download" "$download"
    else
        cache_temp="$WORK_DIR/$expected.cache"
        echo "Downloading $logical_name for $PLATFORM"
        curl --fail --location --retry 3 --silent --show-error --output "$cache_temp" "$url"
        cache_sha="$(shasum -a 256 "$cache_temp" | awk '{print $1}')"
        if [ "$cache_sha" != "$expected" ]; then
            rm -f "$cache_temp"
            echo "Checksum mismatch for $logical_name. Expected $expected, got $cache_sha"
            exit 1
        fi
        mv "$cache_temp" "$cached_download"
        cp "$cached_download" "$download"
    fi
    actual="$(shasum -a 256 "$download" | awk '{print $1}')"
    if [ "$actual" != "$expected" ]; then
        echo "Checksum mismatch for $logical_name. Expected $expected, got $actual"
        exit 1
    fi

    case "$format" in
        raw)
            cp "$download" "$destination"
            ;;
        gzip)
            gzip -dc "$download" > "$destination"
            ;;
        zip)
            unzip -p "$download" "$entry" > "$destination"
            ;;
        *)
            echo "Unsupported toolchain format: $format"
            exit 1
            ;;
    esac
    chmod 0755 "$destination"
    installed_sha="$(shasum -a 256 "$destination" | awk '{print $1}')"
    ARCHIVE_MANIFEST="$(jq \
        --arg logical "$logical_name" \
        --arg filename "$filename" \
        --arg sha "$installed_sha" \
        '.tools[$logical] = $filename | .files[$filename] = {sha256: $sha}' \
        <<<"$ARCHIVE_MANIFEST")"
done

printf '%s\n' "$ARCHIVE_MANIFEST" > "$WORK_DIR/manifest.json"
mkdir -p "$(dirname "$OUTPUT_PATH")"
rm -f "$OUTPUT_PATH"
(
    cd "$WORK_DIR"
    zip -q -9 "$OUTPUT_PATH" manifest.json \
        "$(jq -r '.tools["yt-dlp"]' manifest.json)" \
        "$(jq -r '.tools.ffmpeg' manifest.json)" \
        "$(jq -r '.tools.ffprobe' manifest.json)" \
        "$(jq -r '.tools.deno' manifest.json)"
)
unzip -tq "$OUTPUT_PATH"
echo "Created $OUTPUT_PATH"
