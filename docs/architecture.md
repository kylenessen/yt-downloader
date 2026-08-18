# Architecture

YT Downloader keeps YouTube-specific behavior behind the official yt-dlp command line interface. The Go application does not parse YouTube pages or select private streaming clients.

The runtime pipeline is intentionally small.

1. The Wails frontend calls the application service.
2. The toolchain manager installs and verifies the bundled baseline archive.
3. The yt-dlp adapter reads metadata and downloads a WebKit-compatible MP4.
4. FFmpeg trims and encodes the selected clip.
5. A loopback-only HTTP server provides range requests for preview playback.

## Self-contained toolchain

Production does not search `PATH`, Homebrew, or common installation directories. The signed application contains `toolchain.zip` as inert data. On first launch the manager extracts yt-dlp, FFmpeg, ffprobe, and Deno into the user application support directory. Every extracted file is checked against the manifest inside the signed archive before it is executed.

Developers can supply explicit paths with `YT_DOWNLOADER_YTDLP`, `YT_DOWNLOADER_FFMPEG`, `YT_DOWNLOADER_FFPROBE`, and `YT_DOWNLOADER_DENO`. All four variables are required. A packaged archive can be tested with `YT_DOWNLOADER_TOOLCHAIN_ARCHIVE` and an isolated installation directory can be selected with `YT_DOWNLOADER_TOOLCHAIN_ROOT`.

## Updates and recovery

The app checks the official yt-dlp nightly channel no more than once per day. A new executable is downloaded to a temporary file, checked against the release SHA256 list, executed with `--version`, and then activated atomically. The previous working executable remains available for rollback.

When a high-quality download fails with an extractor, challenge, format, or HTTP 403 error, the app forces one update check and retries once. It uses a progressive stream only after that recovery attempt. If both the updated high-quality and progressive attempts fail, it rolls back and tries the previous progressive downloader once.

FFmpeg, ffprobe, and Deno change less frequently. Their baseline versions are pinned in `toolchain/manifest.json`. Updating that file changes future release packages without adding mutable URLs to the build.

## Release artifacts

`scripts/build-toolchain.sh` creates a platform archive from pinned URLs and SHA256 values. macOS packaging copies only the archive into the app bundle. It does not copy or re-sign nested PyInstaller executables. The outer app is then signed and optionally notarized.

Set `APPLE_SIGN_IDENTITY` to a Developer ID Application identity and `APPLE_NOTARY_PROFILE` to a `notarytool` keychain profile for a distributable macOS release. Without those values the script creates an ad hoc signed development package.

## Automated releases

The release workflow runs only for version tags. It builds each operating system package on its native GitHub-hosted runner. The macOS job refuses to proceed without the Apple signing and notarization secrets. This prevents an unsigned package from being published accidentally.

Configure these repository secrets before creating a release tag.

| Secret | Purpose |
| --- | --- |
| `APPLE_CERTIFICATE_P12` | Base64-encoded Developer ID Application certificate and private key |
| `APPLE_CERTIFICATE_PASSWORD` | Password used when the PKCS12 file was exported |
| `APPLE_KEYCHAIN_PASSWORD` | Ephemeral CI keychain password |
| `APPLE_SIGN_IDENTITY` | Full Developer ID Application identity name |
| `APPLE_ID` | Apple account used by the notary service |
| `APPLE_APP_PASSWORD` | App-specific password for that account |
| `APPLE_TEAM_ID` | Apple Developer team identifier |

Pushing a tag such as `v1.2.0` builds, signs, notarizes, verifies, and publishes the three download archives. Normal pushes and pull requests run source tests and execute the packaged Mac and Windows toolchains on clean runners.
