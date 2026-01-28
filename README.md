# YT Downloader

A self-contained desktop app for downloading, trimming, and exporting YouTube video clips.

## Background

I am teaching a class at the California Men's Colony this semester, and I needed a way to make my lectures entirely offline (I have no access to internet within the prison). I like to use videos wherever I can in my lectures, and had pieced together a number of scripts and hacky solutions make it work for me. Speaking with some of my colleagues, I got the impression that others would find such a tool useful as well, and so I set out to make this piece of software, which is cross-platform and self-contained. The app is designed to be easy to use and allow for easy download of youtube clips. You can have the audio for narration, but I find it especially useful in replacement of GIFs, where no audio is present, and I can lecture over a video or animation. The trimming tools in particular are helpful for this to get just the section you want for your slides. 

There are a variety of quality options available for export, but everything saves to mp4, which plays nice with PowerPoint. You can set the video to play on click, or automatically with looping behavior, replicating GIFs but at a higher quality and reduced size. 

I recommend using 720p, as most projectors max out at this resolution in my experience. Of course, if you have the resolution, you can go higher. 

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
3. Right-click (or Control-click) the app → select "Open" → click "Open" in the dialog (first time only)

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
