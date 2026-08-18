#!/bin/bash
set -euo pipefail

for variable_name in APPLE_CERTIFICATE_P12 APPLE_CERTIFICATE_PASSWORD APPLE_KEYCHAIN_PASSWORD; do
    if [ -z "${!variable_name:-}" ]; then
        echo "Required signing secret is missing: $variable_name"
        exit 1
    fi
done

certificate_file="$(mktemp "${TMPDIR:-/tmp}/yt-downloader-certificate.XXXXXX")"
keychain_file="$RUNNER_TEMP/yt-downloader-signing.keychain-db"

cleanup() {
    rm -f "$certificate_file"
}
trap cleanup EXIT

printf '%s' "$APPLE_CERTIFICATE_P12" | base64 --decode > "$certificate_file"
security create-keychain -p "$APPLE_KEYCHAIN_PASSWORD" "$keychain_file"
security set-keychain-settings -lut 21600 "$keychain_file"
security unlock-keychain -p "$APPLE_KEYCHAIN_PASSWORD" "$keychain_file"
security import "$certificate_file" -P "$APPLE_CERTIFICATE_PASSWORD" -A -t cert -f pkcs12 -k "$keychain_file"
security set-key-partition-list -S apple-tool:,apple: -s -k "$APPLE_KEYCHAIN_PASSWORD" "$keychain_file"
security list-keychains -d user -s "$keychain_file" login.keychain-db
security default-keychain -s "$keychain_file"
security find-identity -v -p codesigning "$keychain_file"
