// Package audio provides audio processing capabilities.
//
// This file implements the audio processing pipeline for validation,
// normalization, and transcoding of audio files.
package audio

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	// ffprobePath is the path to the ffprobe binary. Metadata reads use
	// ffprobe, not ffmpeg: ffmpeg accepts no -show_entries and would fail,
	// so duration measurement must not be handed to it.
	ffprobePath string
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
	// FFprobePath to ffprobe binary. Defaults to ffmpeg's own path with the
	// "ffmpeg" component replaced by "ffprobe", which is how the two ship
	// together; set it explicitly when they do not.
	FFprobePath string
	// Timeout bounds a single ffmpeg/ffprobe invocation. It is per operation
	// rather than per pipeline: a normalise plus a transcode is two renders
	// and must not share one budget.
	Timeout time.Duration
}

// waveformSampleRate is the rate audio is decoded at to build a waveform.
// Peaks per second is far below the source rate, and decoding a 60-minute
// render at 48 kHz to draw a thousand bars would waste most of the work.
const waveformSampleRate = 8000

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
		FFprobePath:    "ffprobe",
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
	if cfg.FFprobePath == "" {
		cfg.FFprobePath = defaultFFprobePath(cfg.FFmpegPath)
	}

	// Ensure temp directory exists
	if err := os.MkdirAll(cfg.TempDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Verify both binaries are available. ffprobe is checked separately: it
	// ships with ffmpeg but not always, and a missing one must fail at
	// construction rather than on the first duration measurement.
	if err := verifyFFmpeg(cfg.FFmpegPath); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFFmpegNotAvailable, err)
	}
	if err := verifyFFmpeg(cfg.FFprobePath); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFFmpegNotAvailable, err)
	}

	return &Processor{
		config:      cfg,
		ffmpegPath:  cfg.FFmpegPath,
		ffprobePath: cfg.FFprobePath,
		tempDir:     cfg.TempDir,
	}, nil
}

// defaultFFprobePath derives the ffprobe path from the ffmpeg path.
func defaultFFprobePath(ffmpeg string) string {
	if i := strings.LastIndex(ffmpeg, "ffmpeg"); i >= 0 {
		return ffmpeg[:i] + "ffprobe" + ffmpeg[i+len("ffmpeg"):]
	}
	return "ffprobe"
}

// withTimeout bounds one subprocess invocation. A zero Timeout means the
// caller's context is the only limit, which is the behaviour this had before
// the field existed and is what the tests rely on.
func (p *Processor) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if p.config.Timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, p.config.Timeout)
}

// verifyFFmpeg checks that a media binary runs. It reports the bare reason
// only; the caller adds ErrFFmpegNotAvailable, so the sentinel is wrapped
// exactly once and errors.Is stays useful.
func verifyFFmpeg(path string) error {
	cmd := exec.Command(path, "-version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("could not run %q: %v", path, err)
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
	ctx, cancel := p.withTimeout(ctx)
	defer cancel()

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
	ctx, cancel := p.withTimeout(ctx)
	defer cancel()

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

	ctx, cancel := p.withTimeout(ctx)
	defer cancel()

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
//
// It shells out to ffprobe rather than ffmpeg. These arguments are ffprobe's
// (-show_entries/-of); ffmpeg rejects both, so this measurement silently
// failed for every caller until the binary was corrected.
func (p *Processor) GetDuration(ctx context.Context, path string) (float64, error) {
	if path == "" {
		return 0, fmt.Errorf("%w: no input path", ErrProcessingFailed)
	}

	ctx, cancel := p.withTimeout(ctx)
	defer cancel()

	args := []string{
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	}

	cmd := exec.CommandContext(ctx, p.ffprobePath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return 0, fmt.Errorf("%w: could not measure duration: %s", ErrProcessingFailed, detail)
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

// strToFloat parses a duration string (e.g., "123.456") to float64.
// Uses Go's standard library for robustness.
func strToFloat(s string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}

// GetWaveform returns `width` peak amplitudes in the range 0..1, normalised so
// the loudest bar is 1.
//
// It decodes the file to mono 16-bit PCM at a reduced rate and takes the peak
// of each bucket. The previous implementation rendered a sine wave and returned
// it even when ffmpeg had failed, so a caller drawing a waveform for a file
// that could not be decoded got a plausible-looking picture of nothing.
func (p *Processor) GetWaveform(ctx context.Context, path string, width int) ([]float64, error) {
	if path == "" {
		return nil, fmt.Errorf("%w: no input path", ErrProcessingFailed)
	}
	if width <= 0 {
		return nil, fmt.Errorf("%w: width must be positive, got %d", ErrProcessingFailed, width)
	}

	ctx, cancel := p.withTimeout(ctx)
	defer cancel()

	args := []string{
		"-v", "error",
		"-i", path,
		// First audio stream only: an input carrying artwork or a second
		// language track must not widen the buckets.
		"-map", "0:a:0",
		"-ac", "1",
		"-ar", strconv.Itoa(waveformSampleRate),
		"-f", "s16le",
		"-c:a", "pcm_s16le",
		"-",
	}

	cmd := exec.CommandContext(ctx, p.ffmpegPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	raw, err := cmd.Output()
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return nil, fmt.Errorf("%w: could not decode audio for a waveform: %s", ErrProcessingFailed, detail)
	}

	peaks, ok := pcmPeaks(raw, width)
	if !ok {
		return nil, fmt.Errorf("%w: decoded no audio samples", ErrProcessingFailed)
	}
	return peaks, nil
}

// pcmPeaks buckets little-endian 16-bit PCM into width peak amplitudes and
// normalises them to 0..1.
//
// It is separate from GetWaveform so the arithmetic can be tested without a
// media binary on the machine. The second return is false when there is no
// sample to draw, which is a decode failure rather than silence: silence is
// representable (all zeros) and must stay distinguishable from it.
func pcmPeaks(raw []byte, width int) ([]float64, bool) {
	// Two bytes per sample; a trailing odd byte is a truncated frame and is
	// dropped rather than allowed to shift every later sample.
	samples := len(raw) / 2
	if samples == 0 {
		return nil, false
	}

	peaks := make([]float64, width)
	perBucket := samples / width
	if perBucket < 1 {
		perBucket = 1
	}

	for i := 0; i < width; i++ {
		start := i * perBucket
		if start >= samples {
			// Fewer samples than buckets: the tail stays at zero, which is
			// honest - there is no audio there.
			break
		}
		end := start + perBucket
		if i == width-1 || end > samples {
			end = samples
		}

		var peak int
		for b := start; b < end; b++ {
			// int16 round-trips through uint16 so the sign is preserved.
			v := int(int16(binary.LittleEndian.Uint16(raw[b*2 : b*2+2])))
			if v < 0 {
				v = -v
			}
			if v > peak {
				peak = v
			}
		}
		peaks[i] = float64(peak) / 32768
	}

	// Normalise so a quiet render still fills the height it was given. Without
	// this a well-mastered-but-quiet file draws as a flat line.
	var max float64
	for _, v := range peaks {
		if v > max {
			max = v
		}
	}
	if max > 0 {
		for i := range peaks {
			peaks[i] /= max
		}
	}
	return peaks, true
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
