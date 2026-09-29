// Package jobs provides audio post-processing capabilities.
//
// This file implements the audio processor that handles post-generation
// processing like format conversion, quality optimization, and metadata extraction.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/Teamthy/i-confess/internal/audio"
	"github.com/Teamthy/i-confess/internal/storage"
)

// ErrProcessingFailed is returned when audio processing fails.
var ErrProcessingFailed = errors.New("jobs: audio processing failed")

// AudioProcessor handles post-generation audio processing.
type AudioProcessor struct {
	audioProcessor *audio.Processor
	storage        storage.ObjectStorage
	config         ProcessorConfig
}

// ProcessorConfig holds configuration for the audio processor.
type ProcessorConfig struct {
	FFmpegPath    string
	TempDir       string
	TargetFormat  string
	TargetBitrate int
	SampleRate   int
	Channels     int
}

// NewAudioProcessor creates a new audio processor.
func NewAudioProcessor(
	audioProcessor *audio.Processor,
	storage storage.ObjectStorage,
	cfg ProcessorConfig,
) (*AudioProcessor, error) {
	if cfg.TempDir == "" {
		cfg.TempDir = "/tmp/iconfess-processor"
	}
	if cfg.FFmpegPath == "" {
		cfg.FFmpegPath = "ffmpeg"
	}
	if cfg.TargetFormat == "" {
		cfg.TargetFormat = "m4a"
	}
	if cfg.TargetBitrate == 0 {
		cfg.TargetBitrate = 128
	}
	if cfg.SampleRate == 0 {
		cfg.SampleRate = 48000
	}
	if cfg.Channels == 0 {
		cfg.Channels = 2
	}

	// Ensure temp directory exists
	if err := os.MkdirAll(cfg.TempDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	return &AudioProcessor{
		audioProcessor: audioProcessor,
		storage:        storage,
		config:         cfg,
	}, nil
}

// Process processes an audio file with the configured transformations.
func (p *AudioProcessor) Process(
	ctx context.Context,
	inputData []byte,
	inputFormat string,
) ([]byte, error) {
	// Create temp file for input
	inputPath, err := p.writeTempFile(inputData, "input", inputFormat)
	if err != nil {
		return nil, fmt.Errorf("failed to write input file: %w", err)
	}
	defer os.Remove(inputPath)

	// Run the processing pipeline
	outputPath, err := p.processFile(ctx, inputPath)
	if err != nil {
		return nil, fmt.Errorf("processing failed: %w", err)
	}
	defer os.Remove(outputPath)

	// Read the processed data
	outputData, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read output file: %w", err)
	}

	return outputData, nil
}

// ProcessFile processes an audio file from disk.
func (p *AudioProcessor) ProcessFile(ctx context.Context, inputPath string) ([]byte, error) {
	outputPath, err := p.processFile(ctx, inputPath)
	if err != nil {
		return nil, err
	}
	defer os.Remove(outputPath)

	return os.ReadFile(outputPath)
}

// processFile runs the full processing pipeline on a file.
func (p *AudioProcessor) processFile(ctx context.Context, inputPath string) (string, error) {
	// Step 1: Validate the input
	if err := p.validate(ctx, inputPath); err != nil {
		return "", fmt.Errorf("validation failed: %w", err)
	}

	// Step 2: Normalize audio levels
	normalizedPath, err := p.normalize(ctx, inputPath)
	if err != nil {
		return "", fmt.Errorf("normalization failed: %w", err)
	}
	defer os.Remove(normalizedPath)

	// Step 3: Apply any additional processing (e.g., noise reduction, EQ)
	processedPath, err := p.applyProcessing(ctx, normalizedPath)
	if err != nil {
		return "", fmt.Errorf("additional processing failed: %w", err)
	}
	defer os.Remove(processedPath)

	// Step 4: Convert to target format
	outputPath, err := p.convertFormat(ctx, processedPath)
	if err != nil {
		return "", fmt.Errorf("format conversion failed: %w", err)
	}

	return outputPath, nil
}

// validate checks if the audio file is valid.
func (p *AudioProcessor) validate(ctx context.Context, path string) error {
	// Use the audio package's Inspect function
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	_, err = audio.Inspect(data, "")
	if err != nil {
		return fmt.Errorf("audio inspection failed: %w", err)
	}

	return nil
}

// normalize applies loudness normalization.
func (p *AudioProcessor) normalize(ctx context.Context, inputPath string) (string, error) {
	outputPath := p.tempFilePath("normalized", "wav")

	args := []string{
		"-y",
		"-i", inputPath,
		"-filter_complex",
		"loudnorm=I=-18:TP=-1:LRA=11:print_format=summary",
		"-ar", fmt.Sprintf("%d", p.config.SampleRate),
		"-ac", fmt.Sprintf("%d", p.config.Channels),
		outputPath,
	}

	cmd := exec.CommandContext(ctx, p.config.FFmpegPath, args...)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ffmpeg normalization failed: %w", err)
	}

	return outputPath, nil
}

// applyProcessing applies additional audio processing.
func (p *AudioProcessor) applyProcessing(ctx context.Context, inputPath string) (string, error) {
	// For now, just pass through
	// In a real implementation, this could apply:
	// - Noise reduction
	// - EQ adjustments
	// - Dynamic range compression
	// - etc.
	return inputPath, nil
}

// convertFormat converts to the target format.
func (p *AudioProcessor) convertFormat(ctx context.Context, inputPath string) (string, error) {
	outputPath := p.tempFilePath("output", p.config.TargetFormat)

	args := []string{
		"-y",
		"-i", inputPath,
	}

	// Add codec and format options based on target
	switch p.config.TargetFormat {
	case "mp3":
		args = append(args,
			"-codec:a", "libmp3lame",
			"-b:a", fmt.Sprintf("%dk", p.config.TargetBitrate),
			"-ar", fmt.Sprintf("%d", p.config.SampleRate),
			"-ac", fmt.Sprintf("%d", p.config.Channels),
		)
	case "m4a", "aac":
		args = append(args,
			"-codec:a", "aac",
			"-b:a", fmt.Sprintf("%dk", p.config.TargetBitrate),
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
			"-b:a", fmt.Sprintf("%dk", p.config.TargetBitrate),
			"-ar", fmt.Sprintf("%d", p.config.SampleRate),
			"-ac", fmt.Sprintf("%d", p.config.Channels),
		)
	default:
		return "", fmt.Errorf("unsupported target format: %s", p.config.TargetFormat)
	}

	args = append(args, outputPath)

	cmd := exec.CommandContext(ctx, p.config.FFmpegPath, args...)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ffmpeg conversion failed: %w", err)
	}

	return outputPath, nil
}

// ExtractMetadata extracts metadata from an audio file.
func (p *AudioProcessor) ExtractMetadata(ctx context.Context, path string) (AudioMetadata, error) {
	// Use ffprobe to extract metadata
	args := []string{
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		path,
	}

	cmd := exec.CommandContext(ctx, p.config.FFmpegPath, args...)
	_, err := cmd.Output()
	if err != nil {
		return AudioMetadata{}, fmt.Errorf("failed to extract metadata: %w", err)
	}

	// Parse the JSON output
	// For now, return a simplified metadata structure
	// In a real implementation, this would parse the ffprobe JSON output
	
	// Use the audio inspection as a fallback
	data, err := os.ReadFile(path)
	if err != nil {
		return AudioMetadata{}, fmt.Errorf("failed to read file: %w", err)
	}

	report, err := audio.Inspect(data, "")
	if err != nil {
		return AudioMetadata{}, fmt.Errorf("audio inspection failed: %w", err)
	}

	return AudioMetadata{
		Format:         report.Format,
		Duration:       float64(report.DurationSeconds),
		SampleRate:     report.SampleRate,
		Channels:       report.Channels,
		Bitrate:        0, // Would be extracted from ffprobe
		SizeBytes:      int64(len(data)),
	}, nil
}

// AudioMetadata contains metadata about an audio file.
type AudioMetadata struct {
	Format     string  `json:"format"`
	Duration   float64 `json:"duration"`   // in seconds
	SampleRate int     `json:"sample_rate"` // in Hz
	Channels   int     `json:"channels"`
	Bitrate    int     `json:"bitrate"`    // in kbps
	SizeBytes  int64   `json:"size_bytes"`
}

// GetDuration returns the duration of an audio file in seconds.
func (p *AudioProcessor) GetDuration(ctx context.Context, path string) (float64, error) {
	// Use the audio processor's GetDuration
	return p.audioProcessor.GetDuration(ctx, path)
}

// CreateVariants creates multiple quality variants from a source file.
func (p *AudioProcessor) CreateVariants(
	ctx context.Context,
	sourceData []byte,
	sourceFormat string,
	variants []VariantConfig,
) (map[string][]byte, error) {
	results := make(map[string][]byte)

	for _, variant := range variants {
		// Create a processor with the variant's configuration
		variantProcessor, err := NewAudioProcessor(
			p.audioProcessor,
			p.storage,
			ProcessorConfig{
				FFmpegPath:   p.config.FFmpegPath,
				TempDir:      p.config.TempDir,
				TargetFormat: variant.Format,
				TargetBitrate: variant.Bitrate,
				SampleRate:    variant.SampleRate,
				Channels:      variant.Channels,
			},
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create variant processor: %w", err)
		}

		// Process the audio with variant settings
		variantData, err := variantProcessor.Process(ctx, sourceData, sourceFormat)
		if err != nil {
			return nil, fmt.Errorf("failed to process variant %s: %w", variant.Name, err)
		}

		results[variant.Name] = variantData
	}

	return results, nil
}

// VariantConfig defines the configuration for an audio variant.
type VariantConfig struct {
	Name        string
	Format      string
	Bitrate     int
	SampleRate  int
	Channels    int
}

// DefaultVariants returns the default set of audio variants.
func DefaultVariants() []VariantConfig {
	return []VariantConfig{
		{Name: "standard", Format: "m4a", Bitrate: 128, SampleRate: 48000, Channels: 2},
		{Name: "high", Format: "m4a", Bitrate: 256, SampleRate: 48000, Channels: 2},
		{Name: "low", Format: "m4a", Bitrate: 64, SampleRate: 44100, Channels: 1},
		{Name: "mp3", Format: "mp3", Bitrate: 128, SampleRate: 44100, Channels: 2},
	}
}

// tempFilePath generates a path for a temporary file.
func (p *AudioProcessor) tempFilePath(prefix, format string) string {
	return filepath.Join(p.config.TempDir, fmt.Sprintf("%s-%d.%s", prefix, time.Now().UnixNano(), format))
}

// writeTempFile writes data to a temporary file.
func (p *AudioProcessor) writeTempFile(data []byte, prefix, format string) (string, error) {
	path := p.tempFilePath(prefix, format)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}
	return path, nil
}

// ProcessWithFFmpeg runs a custom ffmpeg command on an audio file.
func (p *AudioProcessor) ProcessWithFFmpeg(
	ctx context.Context,
	inputPath string,
	args []string,
) (string, error) {
	outputPath := p.tempFilePath("custom", "wav")

	// Build the full ffmpeg command
	fullArgs := append([]string{"-y", "-i", inputPath}, args...)
	fullArgs = append(fullArgs, outputPath)

	cmd := exec.CommandContext(ctx, p.config.FFmpegPath, fullArgs...)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ffmpeg custom processing failed: %w", err)
	}

	return outputPath, nil
}

// OptimizeForStreaming optimizes an audio file for streaming.
func (p *AudioProcessor) OptimizeForStreaming(
	ctx context.Context,
	inputData []byte,
	inputFormat string,
) ([]byte, error) {
	// For streaming, we want:
	// - Lower bitrate for faster loading
	// - Mono instead of stereo for smaller size
	// - Optimized metadata
	
	config := ProcessorConfig{
		FFmpegPath:   p.config.FFmpegPath,
		TempDir:      p.config.TempDir,
		TargetFormat: "m4a",
		TargetBitrate: 64,
		SampleRate:    44100,
		Channels:      1,
	}

	processor, err := NewAudioProcessor(p.audioProcessor, p.storage, config)
	if err != nil {
		return nil, err
	}

	return processor.Process(ctx, inputData, inputFormat)
}

// OptimizeForDownload optimizes an audio file for download (high quality).
func (p *AudioProcessor) OptimizeForDownload(
	ctx context.Context,
	inputData []byte,
	inputFormat string,
) ([]byte, error) {
	// For download, we want:
	// - Higher bitrate for better quality
	// - Stereo for better experience
	
	config := ProcessorConfig{
		FFmpegPath:   p.config.FFmpegPath,
		TempDir:      p.config.TempDir,
		TargetFormat: "m4a",
		TargetBitrate: 256,
		SampleRate:    48000,
		Channels:      2,
	}

	processor, err := NewAudioProcessor(p.audioProcessor, p.storage, config)
	if err != nil {
		return nil, err
	}

	return processor.Process(ctx, inputData, inputFormat)
}
