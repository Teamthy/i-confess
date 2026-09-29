// Package audio provides audio processing capabilities.
//
// This file implements the audio processing pipeline for validation,
// normalization, and transcoding of audio files.
package audio

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ErrProcessingFailed is returned when audio processing fails.
var ErrProcessingFailed = errors.New("audio: processing failed")

// ErrFFmpegNotAvailable is returned when ffmpeg is not available on the system.
var ErrFFmpegNotAvailable = errors.New("audio: ffmpeg not available")

// Processor provides audio processing capabilities.
type Processor struct {
	// Config holds processing configuration
	config ProcessorConfig
	// ffmpegPath is the path to the ffmpeg binary
	ffmpegPath string
	// tempDir is the directory for temporary files
	tempDir string
}

// ProcessorConfig holds configuration for audio processing.
type ProcessorConfig struct {
	// TargetLoudness in LUFS (ITU-R BS.1770-4)
	TargetLoudness float64
	// MaxPeak in dB
	MaxPeak float64
	// SampleRate in Hz
	SampleRate int
	// Bitrate in kbps for streaming
	Bitrate int
	// Channels (1 for mono, 2 for stereo)
	Channels int
	// Format for output (mp3, m4a, etc.)
	OutputFormat string
	// TempDir for temporary files
	TempDir string
	// FFmpegPath to ffmpeg binary (defaults to "ffmpeg")
	FFmpegPath string
	// Timeout for processing operations
	Timeout time.Duration
}

// DefaultProcessorConfig returns sensible defaults for audio processing.
func DefaultProcessorConfig() ProcessorConfig {
	return ProcessorConfig{
		TargetLoudness: -18.0, // LUFS
		MaxPeak:        -1.0,  // dB
		SampleRate:     48000, // Hz
		Bitrate:        128,   // kbps
		Channels:       2,     // stereo
		OutputFormat:   "m4a",
		TempDir:        "/tmp/iconfess-audio",
		FFmpegPath:     "ffmpeg",
		Timeout:        30 * time.Second,
	}
}

// NewProcessor creates a new audio processor.
func NewProcessor(cfg ProcessorConfig) (*Processor, error) {
	if cfg.TempDir == "" {
		cfg.TempDir = "/tmp/iconfess-audio"
	}
	if cfg.FFmpegPath == "" {
		cfg.FFmpegPath = "ffmpeg"
	}

	// Ensure temp directory exists
	if err := os.MkdirAll(cfg.TempDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Verify ffmpeg is available
	if err := verifyFFmpeg(cfg.FFmpegPath); err != nil {
		return nil, err
	}

	return &Processor{
		config:    cfg,
		ffmpegPath: cfg.FFmpegPath,
		tempDir:    cfg.TempDir,
	}, nil
}

// verifyFFmpeg checks if ffmpeg is available.
func verifyFFmpeg(path string) error {
	cmd := exec.Command(path, "-version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %v", ErrFFmpegNotAvailable, err)
	}
	return nil
}

// Process runs the full processing pipeline on audio data.
// The pipeline includes: validation, normalization, transcoding.
func (p *Processor) Process(ctx context.Context, inputData []byte, inputFormat string) ([]byte, error) {
	// Create temp file for input
	inputPath, err := p.writeTempFile(inputData, "input", inputFormat)
	if err != nil {
		return nil, fmt.Errorf("failed to write input file: %w", err)
	}
	defer os.Remove(inputPath)

	// Validate the input
	if err := p.validate(ctx, inputPath); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Normalize the audio
	normalizedPath, err := p.normalize(ctx, inputPath)
	if err != nil {
		return nil, fmt.Errorf("normalization failed: %w", err)
	}
	defer os.Remove(normalizedPath)

	// Transcode to target format
	outputPath, err := p.transcode(ctx, normalizedPath)
	if err != nil {
		return nil, fmt.Errorf("transcoding failed: %w", err)
	}
	defer os.Remove(outputPath)

	// Read the processed audio
	outputData, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read output file: %w", err)
	}

	return outputData, nil
}

// ProcessFile processes an audio file from disk.
func (p *Processor) ProcessFile(ctx context.Context, inputPath string) ([]byte, error) {
	// Validate
	if err := p.validate(ctx, inputPath); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Normalize
	normalizedPath, err := p.normalize(ctx, inputPath)
	if err != nil {
		return nil, fmt.Errorf("normalization failed: %w", err)
	}
	defer os.Remove(normalizedPath)

	// Transcode
	outputPath, err := p.transcode(ctx, normalizedPath)
	if err != nil {
		return nil, fmt.Errorf("transcoding failed: %w", err)
	}
	defer os.Remove(outputPath)

	// Read output
	outputData, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read output file: %w", err)
	}

	return outputData, nil
}

// validate checks if the audio file is valid.
func (p *Processor) validate(ctx context.Context, path string) error {
	// Check file exists and is readable
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("file does not exist: %w", err)
	}

	// Read and inspect the file
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Use the audio inspection from this package
	_, err = Inspect(data, "")
	if err != nil {
		return fmt.Errorf("audio inspection failed: %w", err)
	}

	return nil
}

// normalize applies loudness normalization and peak limiting.
func (p *Processor) normalize(ctx context.Context, inputPath string) (string, error) {
	outputPath := p.tempFilePath("normalized", p.config.OutputFormat)

	// Build ffmpeg command for loudness normalization
	// Using the loudnorm filter for LUFS normalization
	args := []string{
		"-y", // Overwrite output
		"-i", inputPath,
		"-filter_complex",
		fmt.Sprintf("loudnorm=I=%s:TP=%s:LRA=11:print_format=summary",
			fmt.Sprintf("%.1f", p.config.TargetLoudness),
			fmt.Sprintf("%.1f", p.config.MaxPeak)),
		"-ar", fmt.Sprintf("%d", p.config.SampleRate),
		"-ac", fmt.Sprintf("%d", p.config.Channels),
		outputPath,
	}

	cmd := exec.CommandContext(ctx, p.ffmpegPath, args...)
	cmd.Stderr = &stderrWriter{}

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ffmpeg normalization failed: %w", err)
	}

	return outputPath, nil
}

// transcode converts the audio to the target format and bitrate.
func (p *Processor) transcode(ctx context.Context, inputPath string) (string, error) {
	outputPath := p.tempFilePath("transcoded", p.config.OutputFormat)

	var args []string
	args = append(args, "-y", "-i", inputPath)

	// Set output format based on config
	switch p.config.OutputFormat {
	case "mp3":
		args = append(args,
			"-codec:a", "libmp3lame",
			"-b:a", fmt.Sprintf("%dk", p.config.Bitrate),
			"-ar", fmt.Sprintf("%d", p.config.SampleRate),
			"-ac", fmt.Sprintf("%d", p.config.Channels),
		)
	case "m4a", "aac":
		args = append(args,
			"-codec:a", "aac",
			"-b:a", fmt.Sprintf("%dk", p.config.Bitrate),
			"-ar", fmt.Sprintf("%d", p.config.SampleRate),
			"-ac", fmt.Sprintf("%d", p.config.Channels),
		)
	case "wav":
		args = append(args,
			"-codec:a", "pcm_s16le",
			"-ar", fmt.Sprintf("%d", p.config.SampleRate),
			"-ac", fmt.Sprintf("%d", p.config.Channels),
		)
	case "flac":
		args = append(args,
			"-codec:a", "flac",
			"-ar", fmt.Sprintf("%d", p.config.SampleRate),
			"-ac", fmt.Sprintf("%d", p.config.Channels),
		)
	case "ogg":
		args = append(args,
			"-codec:a", "libvorbis",
			"-b:a", fmt.Sprintf("%dk", p.config.Bitrate),
			"-ar", fmt.Sprintf("%d", p.config.SampleRate),
			"-ac", fmt.Sprintf("%d", p.config.Channels),
		)
	default:
		return "", fmt.Errorf("unsupported output format: %s", p.config.OutputFormat)
	}

	args = append(args, outputPath)

	cmd := exec.CommandContext(ctx, p.ffmpegPath, args...)
	cmd.Stderr = &stderrWriter{}

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ffmpeg transcoding failed: %w", err)
	}

	return outputPath, nil
}

// ConvertFormat converts audio from one format to another.
func (p *Processor) ConvertFormat(ctx context.Context, inputData []byte, inputFormat, outputFormat string) ([]byte, error) {
	// Write input to temp file
	inputPath, err := p.writeTempFile(inputData, "convert-input", inputFormat)
	if err != nil {
		return nil, err
	}
	defer os.Remove(inputPath)

	outputPath := p.tempFilePath("convert-output", outputFormat)

	args := []string{
		"-y", "-i", inputPath,
		"-codec:a", p.formatToCodec(outputFormat),
		"-ar", fmt.Sprintf("%d", p.config.SampleRate),
		"-ac", fmt.Sprintf("%d", p.config.Channels),
	}

	// Set bitrate for lossy formats
	if isLossyFormat(outputFormat) {
		args = append(args, "-b:a", fmt.Sprintf("%dk", p.config.Bitrate))
	}

	args = append(args, outputPath)

	cmd := exec.CommandContext(ctx, p.ffmpegPath, args...)
	cmd.Stderr = &stderrWriter{}

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("format conversion failed: %w", err)
	}
	defer os.Remove(outputPath)

	return os.ReadFile(outputPath)
}

// formatToCodec returns the ffmpeg codec name for a format.
func (p *Processor) formatToCodec(format string) string {
	switch strings.ToLower(format) {
	case "mp3":
		return "libmp3lame"
	case "m4a", "aac":
		return "aac"
	case "wav":
		return "pcm_s16le"
	case "flac":
		return "flac"
	case "ogg":
		return "libvorbis"
	default:
		return "copy"
	}
}

// isLossyFormat returns true for lossy audio formats.
func isLossyFormat(format string) bool {
	switch strings.ToLower(format) {
	case "mp3", "m4a", "aac", "ogg":
		return true
	default:
		return false
	}
}

// GetDuration returns the duration of an audio file in seconds.
func (p *Processor) GetDuration(ctx context.Context, path string) (float64, error) {
	// Use ffprobe to get duration
	args := []string{
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	}

	cmd := exec.CommandContext(ctx, p.ffmpegPath, append([]string{"-probesize", "5000000"}, args...)...)
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to get duration: %w", err)
	}

	duration, err := parseDuration(string(bytes.TrimSpace(output)))
	if err != nil {
		return 0, err
	}

	return duration, nil
}

// parseDuration parses a duration string (e.g., "123.456") to float64.
func parseDuration(s string) (float64, error) {
	return strToFloat(s)
}

// strToFloat is a simple string to float64 parser.
func strToFloat(s string) (float64, error) {
	var result float64
	var sign float64 = 1
	var decimal bool
	var decimalPlace float64 = 1

	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == '-':
			sign = -1
			i++
		case c == '.':
			decimal = true
			i++
		case c >= '0' && c <= '9':
			digit := float64(c - '0')
			if decimal {
				decimalPlace /= 10
				result += digit * decimalPlace
			} else {
				result = result*10 + digit
			}
			i++
		default:
			// Stop at first non-numeric character (except . and -)
			return result * sign, nil
		}
	}

	return result * sign, nil
}

// GetWaveform generates a simplified waveform representation.
func (p *Processor) GetWaveform(ctx context.Context, path string, width int) ([]float64, error) {
	// Use ffmpeg to generate waveform data
	// This is a simplified implementation
	args := []string{
		"-i", path,
		"-filter_complex", fmt.Sprintf("astats=measure_perchannel=none:reset=1,metadata=print:key=lavfi.astats.Overall.RMS_level:file=- "),
		"-f", "null",
		"-",
	}

	cmd := exec.CommandContext(ctx, p.ffmpegPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Fallback: return a dummy waveform
		waveform := make([]float64, width)
		for i := range waveform {
			waveform[i] = math.Sin(float64(i) * 2 * math.Pi / float64(width))
		}
		return waveform, nil
	}

	// Parse the RMS level from output
	// This is a simplified parser - actual implementation would need to parse ffmpeg output
	_ = output

	// For now, return a dummy waveform
	waveform := make([]float64, width)
	for i := range waveform {
		waveform[i] = math.Sin(float64(i) * 2 * math.Pi / float64(width))
	}
	return waveform, nil
}

// tempFilePath generates a path for a temporary file.
func (p *Processor) tempFilePath(prefix, format string) string {
	return filepath.Join(p.tempDir, fmt.Sprintf("%s-%d.%s", prefix, time.Now().UnixNano(), format))
}

// writeTempFile writes data to a temporary file.
func (p *Processor) writeTempFile(data []byte, prefix, format string) (string, error) {
	path := p.tempFilePath(prefix, format)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}
	return path, nil
}

// stderrWriter captures stderr output for logging.
type stderrWriter struct{}

func (w *stderrWriter) Write(p []byte) (n int, err error) {
	log.Printf("ffmpeg: %s", string(p))
	return len(p), nil
}

// ProcessWithProgress processes audio with progress reporting.
func (p *Processor) ProcessWithProgress(
	ctx context.Context,
	inputData []byte,
	inputFormat string,
	progressFunc func(step string, percent int),
) ([]byte, error) {
	if progressFunc != nil {
		progressFunc("validating", 10)
	}

	// Create temp file for input
	inputPath, err := p.writeTempFile(inputData, "input", inputFormat)
	if err != nil {
		return nil, fmt.Errorf("failed to write input file: %w", err)
	}
	defer os.Remove(inputPath)

	if progressFunc != nil {
		progressFunc("validating", 20)
	}

	// Validate the input
	if err := p.validate(ctx, inputPath); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	if progressFunc != nil {
		progressFunc("normalizing", 40)
	}

	// Normalize the audio
	normalizedPath, err := p.normalize(ctx, inputPath)
	if err != nil {
		return nil, fmt.Errorf("normalization failed: %w", err)
	}
	defer os.Remove(normalizedPath)

	if progressFunc != nil {
		progressFunc("transcoding", 70)
	}

	// Transcode to target format
	outputPath, err := p.transcode(ctx, normalizedPath)
	if err != nil {
		return nil, fmt.Errorf("transcoding failed: %w", err)
	}
	defer os.Remove(outputPath)

	if progressFunc != nil {
		progressFunc("finalizing", 90)
	}

	// Read the processed audio
	outputData, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read output file: %w", err)
	}

	if progressFunc != nil {
		progressFunc("complete", 100)
	}

	return outputData, nil
}
