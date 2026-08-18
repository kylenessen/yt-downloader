# Third-party software

YT Downloader distributes the following third-party executable components inside its self-contained toolchain archive.

yt-dlp is distributed from the official yt-dlp nightly releases. The macOS and Windows standalone executables contain components under GPLv3 and other licenses. See the [yt-dlp release documentation](https://github.com/yt-dlp/yt-dlp#release-files) and [third-party notices](https://github.com/yt-dlp/yt-dlp/blob/master/THIRD_PARTY_LICENSES.txt).

FFmpeg and ffprobe are distributed from the `ffmpeg-static` project. The bundled builds and their license information are available from the pinned [b6.1.1 release](https://github.com/eugeneware/ffmpeg-static/releases/tag/b6.1.1).

Deno is licensed under the MIT License and is distributed from the pinned [v2.9.5 release](https://github.com/denoland/deno/releases/tag/v2.9.5).

The exact versions, source URLs, and SHA256 checksums used for every supported platform are recorded in `toolchain/manifest.json`.
