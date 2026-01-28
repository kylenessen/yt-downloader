# YouTube Clipper

A self-contained macOS app for downloading, trimming, and exporting YouTube video clips.

## Features

- 🎬 Download YouTube videos
- ✂️ Trim clips with a visual editor
- 🎚️ Quality selection (360p - 1080p or original)
- 🔇 Optional audio removal
- 📦 Self-contained - no external dependencies required

## Building for Distribution

To build a distributable app bundle with FFmpeg included:

```bash
./scripts/build-macos.sh
```

This will:
1. Download FFmpeg if not already present
2. Build the Wails app
3. Bundle FFmpeg into the `.app` package

The final app will be at `build/bin/yt-clipper.app` (~90MB).

## Development

### Live Development

To run in live development mode:

```bash
wails dev
```

This runs a Vite development server with hot reload for frontend changes.

### Quick Build (without bundled FFmpeg)

For development builds where you have FFmpeg installed via Homebrew:

```bash
wails build
```

Note: Users without FFmpeg installed will see a prompt to install it.

## Requirements

### For Development
- Go 1.21+
- Node.js 18+
- Wails CLI (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`)

### For End Users
- macOS 11+ (Big Sur or later)
- No additional dependencies when using the bundled build
