import './style.css';
import { LoadVideo, SelectOutputDirectory, ExportClip, CheckFFmpeg, InstallFFmpeg } from '../wailsjs/go/main/App';
import { EventsOn } from '../wailsjs/runtime/runtime';

// State
let videoInfo = null;
let startTime = 0;
let endTime = 0;
let duration = 0;
let outputDir = '';
let ffmpegInstalled = false;

// Initialize the app
document.querySelector('#app').innerHTML = `
    <h1>YouTube Clipper</h1>

    <!-- FFmpeg Install Banner -->
    <div class="ffmpeg-banner" id="ffmpegBanner">
        <p>FFmpeg is required for video processing but is not installed.</p>
        <button class="btn" id="installFfmpeg">Install FFmpeg</button>
        <div class="progress-container" id="ffmpegProgress">
            <div class="progress-bar">
                <div class="progress-fill" id="ffmpegProgressFill"></div>
            </div>
            <div class="progress-text" id="ffmpegProgressText">Installing...</div>
        </div>
    </div>

    <!-- URL Input Section -->
    <div class="container">
        <div class="url-section">
            <input type="text" class="url-input" id="urlInput" placeholder="Paste YouTube URL here..." />
            <button class="btn" id="loadBtn">Load Video</button>
        </div>
        <div class="progress-container" id="downloadProgress">
            <div class="progress-bar">
                <div class="progress-fill" id="downloadProgressFill"></div>
            </div>
            <div class="progress-text" id="downloadProgressText">Downloading...</div>
        </div>
    </div>

    <!-- Video Section -->
    <div class="video-section" id="videoSection">
        <div class="container">
            <div class="video-info">
                <div class="video-title" id="videoTitle"></div>
                <div class="video-author" id="videoAuthor"></div>
            </div>
            <div class="video-container">
                <video id="videoPlayer"></video>
                <div class="player-controls" id="playerControls">
                    <div class="player-controls-row">
                        <button class="btn btn-secondary player-btn" id="skipBackBtn" title="Back 1s">-1s</button>
                        <button class="btn player-btn" id="playPauseBtn" title="Play/Pause">Play</button>
                        <button class="btn btn-secondary player-btn" id="skipForwardBtn" title="Forward 1s">+1s</button>
                        <div class="player-time" id="playbackTime">00:00:00 / 00:00:00</div>
                    </div>
                    <input type="range" class="player-scrub" id="playbackSlider" min="0" max="0" value="0" step="0.01" />
                </div>
            </div>

            <!-- Trim Controls -->
            <div class="trim-section">
                <div class="trim-label">Trim Selection</div>
                <div class="time-display">
                    <span class="time-value" id="startTimeDisplay">00:00:00</span>
                    <span class="time-value" id="endTimeDisplay">00:00:00</span>
                </div>
                <div class="slider-container">
                    <div class="slider-track"></div>
                    <div class="slider-range" id="sliderRange"></div>
                    <input type="range" id="startSlider" min="0" max="100" value="0" step="0.1" />
                    <input type="range" id="endSlider" min="0" max="100" value="100" step="0.1" />
                </div>
                <div class="duration-info">
                    Clip duration: <span class="clip-duration" id="clipDuration">0:00</span>
                </div>
                <div class="trim-buttons">
                    <button class="btn btn-secondary" id="setStartBtn">Set Start</button>
                    <button class="btn btn-secondary" id="setEndBtn">Set End</button>
                    <button class="btn btn-secondary" id="previewBtn">Preview Clip</button>
                </div>
            </div>
        </div>
    </div>

    <!-- Export Section -->
    <div class="export-section" id="exportSection">
        <div class="container">
            <div class="form-group">
                <label>Filename</label>
                <input type="text" id="filenameInput" placeholder="Enter filename (without extension)" />
            </div>
            <div class="form-group">
                <label>Output Directory</label>
                <div class="directory-select">
                    <input type="text" id="outputDirInput" readonly placeholder="Select output directory..." />
                    <button class="btn btn-secondary" id="selectDirBtn">Browse</button>
                </div>
            </div>
            <div class="form-group checkbox-group">
                <input type="checkbox" id="removeAudioCheck" />
                <label for="removeAudioCheck">Remove audio</label>
            </div>
            <button class="btn export-btn" id="exportBtn">Export Clip</button>
            <div class="progress-container" id="exportProgress">
                <div class="progress-bar">
                    <div class="progress-fill" id="exportProgressFill"></div>
                </div>
                <div class="progress-text" id="exportProgressText">Exporting...</div>
            </div>
        </div>
    </div>

    <!-- Status Messages -->
    <div class="status" id="statusMessage"></div>
`;

// DOM Elements
const urlInput = document.getElementById('urlInput');
const loadBtn = document.getElementById('loadBtn');
const downloadProgress = document.getElementById('downloadProgress');
const downloadProgressFill = document.getElementById('downloadProgressFill');
const downloadProgressText = document.getElementById('downloadProgressText');

const ffmpegBanner = document.getElementById('ffmpegBanner');
const installFfmpegBtn = document.getElementById('installFfmpeg');
const ffmpegProgress = document.getElementById('ffmpegProgress');
const ffmpegProgressFill = document.getElementById('ffmpegProgressFill');
const ffmpegProgressText = document.getElementById('ffmpegProgressText');

const videoSection = document.getElementById('videoSection');
const videoPlayer = document.getElementById('videoPlayer');
const playPauseBtn = document.getElementById('playPauseBtn');
const skipBackBtn = document.getElementById('skipBackBtn');
const skipForwardBtn = document.getElementById('skipForwardBtn');
const playbackSlider = document.getElementById('playbackSlider');
const playbackTime = document.getElementById('playbackTime');
const videoTitle = document.getElementById('videoTitle');
const videoAuthor = document.getElementById('videoAuthor');

const startSlider = document.getElementById('startSlider');
const endSlider = document.getElementById('endSlider');
const sliderRange = document.getElementById('sliderRange');
const startTimeDisplay = document.getElementById('startTimeDisplay');
const endTimeDisplay = document.getElementById('endTimeDisplay');
const clipDuration = document.getElementById('clipDuration');

const setStartBtn = document.getElementById('setStartBtn');
const setEndBtn = document.getElementById('setEndBtn');
const previewBtn = document.getElementById('previewBtn');

const exportSection = document.getElementById('exportSection');
const filenameInput = document.getElementById('filenameInput');
const outputDirInput = document.getElementById('outputDirInput');
const selectDirBtn = document.getElementById('selectDirBtn');
const removeAudioCheck = document.getElementById('removeAudioCheck');
const exportBtn = document.getElementById('exportBtn');
const exportProgress = document.getElementById('exportProgress');
const exportProgressFill = document.getElementById('exportProgressFill');
const exportProgressText = document.getElementById('exportProgressText');

const statusMessage = document.getElementById('statusMessage');

// Format time as HH:MM:SS
function formatTime(seconds) {
    const h = Math.floor(seconds / 3600);
    const m = Math.floor((seconds % 3600) / 60);
    const s = Math.floor(seconds % 60);
    return `${h.toString().padStart(2, '0')}:${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`;
}

// Format duration as M:SS
function formatDuration(seconds) {
    const m = Math.floor(seconds / 60);
    const s = Math.floor(seconds % 60);
    return `${m}:${s.toString().padStart(2, '0')}`;
}

function updatePlaybackControls() {
    if (!videoPlayer.src) {
        playPauseBtn.textContent = 'Play';
        playbackTime.textContent = '00:00:00 / 00:00:00';
        playbackSlider.max = 0;
        playbackSlider.value = 0;
        return;
    }

    playPauseBtn.textContent = videoPlayer.paused ? 'Play' : 'Pause';
    const current = Number.isFinite(videoPlayer.currentTime) ? videoPlayer.currentTime : 0;
    const total = Number.isFinite(duration) ? duration : 0;
    playbackTime.textContent = `${formatTime(current)} / ${formatTime(total)}`;

    playbackSlider.max = total;
    playbackSlider.value = Math.min(current, total);
}

// Update slider range display
function updateSliderRange() {
    const startPercent = (startTime / duration) * 100;
    const endPercent = (endTime / duration) * 100;
    sliderRange.style.left = `${startPercent}%`;
    sliderRange.style.width = `${endPercent - startPercent}%`;

    startTimeDisplay.textContent = formatTime(startTime);
    endTimeDisplay.textContent = formatTime(endTime);
    clipDuration.textContent = formatDuration(endTime - startTime);
}

// Seek helper (debounced for slider scrubbing)
let seekTimer = null;
function seekPreview(time) {
    if (!videoPlayer.src || !Number.isFinite(time)) {
        return;
    }

    const clamped = Math.max(0, Math.min(time, duration || time));

    // Pause so the user can land on an exact frame.
    if (!videoPlayer.paused) {
        videoPlayer.pause();
    }

    if (seekTimer) {
        clearTimeout(seekTimer);
    }
    seekTimer = setTimeout(() => {
        try {
            videoPlayer.currentTime = clamped;
            updatePlaybackControls();
        } catch (e) {
            // ignore seek errors (e.g. not yet ready)
        }
    }, 75);
}

// Show status message
function showStatus(message, type = 'success') {
    statusMessage.textContent = message;
    statusMessage.className = `status visible ${type}`;
    setTimeout(() => {
        statusMessage.classList.remove('visible');
    }, 5000);
}

// Check FFmpeg installation
async function checkFfmpeg() {
    try {
        ffmpegInstalled = await CheckFFmpeg();
        if (!ffmpegInstalled) {
            ffmpegBanner.classList.add('visible');
        } else {
            ffmpegBanner.classList.remove('visible');
        }
    } catch (err) {
        console.error('Failed to check FFmpeg:', err);
        ffmpegBanner.classList.add('visible');
    }
}

// Install FFmpeg
installFfmpegBtn.addEventListener('click', async () => {
    try {
        installFfmpegBtn.disabled = true;
        ffmpegProgress.classList.add('visible');
        await InstallFFmpeg();
        ffmpegInstalled = true;
        ffmpegBanner.classList.remove('visible');
        showStatus('FFmpeg installed successfully!', 'success');
    } catch (err) {
        showStatus(`Failed to install FFmpeg: ${err}`, 'error');
        installFfmpegBtn.disabled = false;
    } finally {
        ffmpegProgress.classList.remove('visible');
    }
});

// Load video
loadBtn.addEventListener('click', async () => {
    const url = urlInput.value.trim();
    if (!url) {
        showStatus('Please enter a YouTube URL', 'error');
        return;
    }

    try {
        loadBtn.disabled = true;
        downloadProgress.classList.add('visible');
        downloadProgressFill.style.width = '0%';
        downloadProgressText.textContent = 'Downloading...';

        videoInfo = await LoadVideo(url);

        // Update video player
        videoPlayer.src = videoInfo.videoUrl;
        videoPlayer.load();
        videoTitle.textContent = videoInfo.title;
        videoAuthor.textContent = videoInfo.author;
        filenameInput.value = videoInfo.title;

        // Reset trim controls
        duration = videoInfo.duration;
        startTime = 0;
        endTime = duration;

        startSlider.max = duration;
        endSlider.max = duration;
        startSlider.value = 0;
        endSlider.value = duration;

        updateSliderRange();
        updatePlaybackControls();

        // Show sections
        videoSection.classList.add('visible');
        exportSection.classList.add('visible');

        showStatus('Video loaded successfully!', 'success');
    } catch (err) {
        showStatus(`Failed to load video: ${err}`, 'error');
    } finally {
        loadBtn.disabled = false;
        downloadProgress.classList.remove('visible');
    }
});

// Video player events
videoPlayer.addEventListener('loadedmetadata', () => {
    // Update duration from actual video if different
    if (videoPlayer.duration && videoPlayer.duration !== Infinity) {
        duration = videoPlayer.duration;
        endTime = duration;
        startSlider.max = duration;
        endSlider.max = duration;
        endSlider.value = duration;
        updateSliderRange();
        updatePlaybackControls();
    }
});

videoPlayer.addEventListener('timeupdate', () => {
    updatePlaybackControls();
});

videoPlayer.addEventListener('play', updatePlaybackControls);
videoPlayer.addEventListener('pause', updatePlaybackControls);

videoPlayer.addEventListener('error', () => {
    const err = videoPlayer.error;
    const code = err ? err.code : 'unknown';
    showStatus(`Video playback failed (code ${code}).`, 'error');
});

// Playback controls
playPauseBtn.addEventListener('click', async () => {
    if (!videoPlayer.src) return;
    if (videoPlayer.paused) {
        try {
            await videoPlayer.play();
        } catch (e) {
            showStatus('Unable to play video.', 'error');
        }
    } else {
        videoPlayer.pause();
    }
});

skipBackBtn.addEventListener('click', () => {
    if (!videoPlayer.src) return;
    seekPreview(Math.max(0, videoPlayer.currentTime - 1));
});

skipForwardBtn.addEventListener('click', () => {
    if (!videoPlayer.src) return;
    seekPreview(Math.min(duration, videoPlayer.currentTime + 1));
});

let scrubWasPlaying = false;
playbackSlider.addEventListener('pointerdown', () => {
    scrubWasPlaying = !videoPlayer.paused;
    videoPlayer.pause();
});
playbackSlider.addEventListener('input', () => {
    const t = parseFloat(playbackSlider.value);
    // scrub should feel immediate
    if (Number.isFinite(t)) {
        try {
            videoPlayer.currentTime = t;
        } catch (e) {
            // ignore
        }
    }
    updatePlaybackControls();
});
playbackSlider.addEventListener('pointerup', async () => {
    if (scrubWasPlaying) {
        try {
            await videoPlayer.play();
        } catch (e) {
            // ignore
        }
    }
    scrubWasPlaying = false;
});

// Click video to toggle play/pause (keeps the video itself clean of overlays)
videoPlayer.addEventListener('click', () => {
    if (!videoPlayer.src) return;
    if (videoPlayer.paused) {
        videoPlayer.play().catch(() => {});
    } else {
        videoPlayer.pause();
    }
});

// Slider events
startSlider.addEventListener('input', () => {
    startTime = parseFloat(startSlider.value);
    if (startTime >= endTime) {
        startTime = endTime - 0.1;
        startSlider.value = startTime;
    }
    updateSliderRange();
    seekPreview(startTime);
});

endSlider.addEventListener('input', () => {
    endTime = parseFloat(endSlider.value);
    if (endTime <= startTime) {
        endTime = startTime + 0.1;
        endSlider.value = endTime;
    }
    updateSliderRange();
    seekPreview(endTime);
});

// Trim buttons
setStartBtn.addEventListener('click', () => {
    startTime = videoPlayer.currentTime;
    startSlider.value = startTime;
    if (startTime >= endTime) {
        endTime = Math.min(startTime + 1, duration);
        endSlider.value = endTime;
    }
    updateSliderRange();
});

setEndBtn.addEventListener('click', () => {
    endTime = videoPlayer.currentTime;
    endSlider.value = endTime;
    if (endTime <= startTime) {
        startTime = Math.max(endTime - 1, 0);
        startSlider.value = startTime;
    }
    updateSliderRange();
});

previewBtn.addEventListener('click', () => {
    videoPlayer.currentTime = startTime;
    videoPlayer.play();

    // Stop at end time
    const checkEnd = () => {
        if (videoPlayer.currentTime >= endTime) {
            videoPlayer.pause();
            videoPlayer.removeEventListener('timeupdate', checkEnd);
        }
    };
    videoPlayer.addEventListener('timeupdate', checkEnd);
});

// Select output directory
selectDirBtn.addEventListener('click', async () => {
    try {
        const dir = await SelectOutputDirectory();
        if (dir) {
            outputDir = dir;
            outputDirInput.value = dir;
        }
    } catch (err) {
        showStatus(`Failed to select directory: ${err}`, 'error');
    }
});

// Export clip
exportBtn.addEventListener('click', async () => {
    if (!videoInfo) {
        showStatus('Please load a video first', 'error');
        return;
    }

    if (!outputDir) {
        showStatus('Please select an output directory', 'error');
        return;
    }

    if (!ffmpegInstalled) {
        showStatus('FFmpeg is not installed. Please install it first.', 'error');
        return;
    }

    const filename = filenameInput.value.trim() || 'clip';

    try {
        exportBtn.disabled = true;
        exportProgress.classList.add('visible');
        exportProgressFill.style.width = '0%';

        await ExportClip({
            startTime: startTime,
            endTime: endTime,
            removeAudio: removeAudioCheck.checked,
            filename: filename,
            outputDir: outputDir
        });

        showStatus('Clip exported successfully!', 'success');
    } catch (err) {
        showStatus(`Failed to export clip: ${err}`, 'error');
    } finally {
        exportBtn.disabled = false;
        exportProgress.classList.remove('visible');
    }
});

// Event listeners for progress updates
EventsOn('download:progress', (progress) => {
    const percent = Math.round(progress * 100);
    downloadProgressFill.style.width = `${percent}%`;
    downloadProgressText.textContent = `Downloading: ${percent}%`;
});

EventsOn('export:progress', (progress) => {
    const percent = Math.round(progress * 100);
    exportProgressFill.style.width = `${percent}%`;
    exportProgressText.textContent = `Exporting: ${percent}%`;
});

EventsOn('ffmpeg:progress', (data) => {
    const percent = Math.round(data.progress * 100);
    ffmpegProgressFill.style.width = `${percent}%`;
    ffmpegProgressText.textContent = data.status;
});

// Initialize
checkFfmpeg();
