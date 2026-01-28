# YT Downloader

A self-contained desktop app for downloading, trimming, and exporting YouTube video clips.

## 📥 Download

| Platform | Download |
|----------|----------|
| **macOS (Apple Silicon)** | [Download for M1/M2/M3 Mac](https://github.com/kylenessen/yt-downloader/releases/latest/download/YT-Downloader-macOS-Apple-Silicon.zip) |
| **macOS (Intel)** | [Download for Intel Mac](https://github.com/kylenessen/yt-downloader/releases/latest/download/YT-Downloader-macOS-Intel.zip) |
| **Windows** | [Download for Windows](https://github.com/kylenessen/yt-downloader/releases/latest/download/YT-Downloader-Windows.zip) |

> **Not sure which Mac you have?** Click  → About This Mac. If it says "Apple M1/M2/M3", use Apple Silicon. If it says "Intel", use Intel.

### Installation

**macOS:**
1. Download and unzip
2. Drag `YT Downloader.app` to your Applications folder
3. **Important:** The app is not signed with an Apple Developer certificate, so macOS will show a warning. To open it:
   - **Option A (recommended):** Right-click (or Control-click) the app → select "Open" → click "Open" in the dialog
   - **Option B (if you see "damaged" error):** Open Terminal and run:
     ```bash
     xattr -cr "/Applications/YT Downloader.app"
     ```
     Then open the app normally.

**Windows:**
1. Download and unzip
2. Keep `YT Downloader.exe` and `ffmpeg.exe` in the same folder
3. Run `YT Downloader.exe`

## Features

- 🎬 Download YouTube videos
- ✂️ Trim clips with a visual editor
- 🎚️ Quality selection (360p - 1080p or original)
- 🔇 Optional audio removal
- 📦 Self-contained - FFmpeg is bundled, no external dependencies required

## Building from Source

### Prerequisites
- Go 1.21+
- Node.js 18+
- Wails CLI (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`)

### Build All Platforms

```bash
./scripts/build-all.sh
```

### Build Individual Platforms

```bash
./scripts/build-macos.sh   # macOS Intel + Apple Silicon
./scripts/build-windows.sh # Windows x64
```

### Development

```bash
wails dev  # Live development with hot reload
```

## System Requirements

| Platform | Requirements |
|----------|-------------|
| macOS | macOS 11+ (Big Sur or later) |
| Windows | Windows 10+ (64-bit) |
