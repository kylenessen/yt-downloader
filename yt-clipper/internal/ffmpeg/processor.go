package ffmpeg

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// TrimOptions specifies options for video trimming
type TrimOptions struct {
	InputPath   string
	OutputPath  string
	StartTime   float64 // in seconds
	EndTime     float64 // in seconds
	RemoveAudio bool
	// Export tuning (optional)
	MaxHeight    int    // If >0, scales to min(MaxHeight, input height)
	CRF          int    // Default 23
	Preset       string // Default "medium"
	AudioBitrate string // Default "128k" (ignored if RemoveAudio)
}

// Processor handles video processing with FFmpeg
type Processor struct {
	ffmpegPath string
}

// NewProcessor creates a new video processor
func NewProcessor(ffmpegPath string) *Processor {
	return &Processor{ffmpegPath: ffmpegPath}
}

// TrimVideo extracts a clip from the video
func (p *Processor) TrimVideo(ctx context.Context, opts TrimOptions) error {
	if p.ffmpegPath == "" {
		return fmt.Errorf("ffmpeg path not set")
	}

	// Validate input
	if _, err := os.Stat(opts.InputPath); os.IsNotExist(err) {
		return fmt.Errorf("input file does not exist: %s", opts.InputPath)
	}

	if opts.EndTime <= opts.StartTime {
		return fmt.Errorf("end time must be greater than start time")
	}

	// Ensure output directory exists
	outputDir := filepath.Dir(opts.OutputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Build ffmpeg command
	args := []string{
		"-y", // Overwrite output
		"-ss", formatTime(opts.StartTime), // Seek to start (before -i for faster seeking)
		"-i", opts.InputPath,
		"-t", formatTime(opts.EndTime - opts.StartTime), // Duration
	}

	if opts.RemoveAudio {
		args = append(args, "-an") // Remove audio
	} else {
		audioBitrate := opts.AudioBitrate
		if audioBitrate == "" {
			audioBitrate = "128k"
		}
		args = append(args, "-c:a", "aac", "-b:a", audioBitrate) // Re-encode audio to AAC
	}

	if opts.MaxHeight > 0 {
		// Avoid upscaling: clamp output height to input height.
		args = append(args, "-vf", fmt.Sprintf("scale=-2:min(%d,ih)", opts.MaxHeight))
	}

	preset := opts.Preset
	if preset == "" {
		preset = "medium"
	}
	crf := opts.CRF
	if crf == 0 {
		crf = 23
	}

	// Video encoding settings
	args = append(args,
		"-c:v", "libx264", // H.264 codec
		"-preset", preset, // Balance between speed and quality
		"-crf", strconv.Itoa(crf), // Quality (lower = better)
		"-movflags", "+faststart", // Enable progressive download
		opts.OutputPath,
	)

	cmd := exec.CommandContext(ctx, p.ffmpegPath, args...)

	// Capture stderr for error messages
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg error: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// TrimVideoWithProgress trims video and reports progress
func (p *Processor) TrimVideoWithProgress(ctx context.Context, opts TrimOptions, progressCb func(float64)) error {
	if p.ffmpegPath == "" {
		return fmt.Errorf("ffmpeg path not set")
	}

	// Validate input
	if _, err := os.Stat(opts.InputPath); os.IsNotExist(err) {
		return fmt.Errorf("input file does not exist: %s", opts.InputPath)
	}

	if opts.EndTime <= opts.StartTime {
		return fmt.Errorf("end time must be greater than start time")
	}

	// Ensure output directory exists
	outputDir := filepath.Dir(opts.OutputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	duration := opts.EndTime - opts.StartTime

	// Build ffmpeg command with progress output
	args := []string{
		"-y",
		"-progress", "pipe:1", // Output progress to stdout
		"-ss", formatTime(opts.StartTime),
		"-i", opts.InputPath,
		"-t", formatTime(duration),
	}

	if opts.RemoveAudio {
		args = append(args, "-an")
	} else {
		audioBitrate := opts.AudioBitrate
		if audioBitrate == "" {
			audioBitrate = "128k"
		}
		args = append(args, "-c:a", "aac", "-b:a", audioBitrate)
	}

	if opts.MaxHeight > 0 {
		args = append(args, "-vf", fmt.Sprintf("scale=-2:min(%d,ih)", opts.MaxHeight))
	}

	preset := opts.Preset
	if preset == "" {
		preset = "medium"
	}
	crf := opts.CRF
	if crf == 0 {
		crf = 23
	}

	args = append(args,
		"-c:v", "libx264",
		"-preset", preset,
		"-crf", strconv.Itoa(crf),
		"-movflags", "+faststart",
		opts.OutputPath,
	)

	cmd := exec.CommandContext(ctx, p.ffmpegPath, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	// Parse progress output
	go func() {
		buf := make([]byte, 1024)
		var currentTime float64

		for {
			n, err := stdout.Read(buf)
			if err != nil {
				break
			}

			output := string(buf[:n])
			lines := strings.Split(output, "\n")

			for _, line := range lines {
				if strings.HasPrefix(line, "out_time_ms=") {
					timeMs := strings.TrimPrefix(line, "out_time_ms=")
					if ms, err := strconv.ParseInt(timeMs, 10, 64); err == nil {
						currentTime = float64(ms) / 1000000.0
						if progressCb != nil && duration > 0 {
							progress := currentTime / duration
							if progress > 1.0 {
								progress = 1.0
							}
							progressCb(progress)
						}
					}
				}
			}
		}
	}()

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("ffmpeg error: %w", err)
	}

	if progressCb != nil {
		progressCb(1.0)
	}

	return nil
}

// GetVideoDuration returns the duration of a video in seconds
func (p *Processor) GetVideoDuration(ctx context.Context, inputPath string) (float64, error) {
	// Use ffprobe if available, otherwise parse ffmpeg output
	ffprobePath := strings.Replace(p.ffmpegPath, "ffmpeg", "ffprobe", 1)

	var cmd *exec.Cmd
	if _, err := os.Stat(ffprobePath); err == nil {
		cmd = exec.CommandContext(ctx, ffprobePath,
			"-v", "error",
			"-show_entries", "format=duration",
			"-of", "default=noprint_wrappers=1:nokey=1",
			inputPath,
		)
	} else {
		// Fallback: use ffmpeg with null output
		cmd = exec.CommandContext(ctx, p.ffmpegPath,
			"-i", inputPath,
			"-f", "null", "-",
		)
	}

	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to get duration: %w", err)
	}

	duration, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse duration: %w", err)
	}

	return duration, nil
}

// formatTime converts seconds to HH:MM:SS.mmm format
func formatTime(seconds float64) string {
	hours := int(seconds) / 3600
	minutes := (int(seconds) % 3600) / 60
	secs := seconds - float64(hours*3600) - float64(minutes*60)
	return fmt.Sprintf("%02d:%02d:%06.3f", hours, minutes, secs)
}
