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
