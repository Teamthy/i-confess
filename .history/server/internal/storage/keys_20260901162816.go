package storage

import (
	"fmt"
)

// Keys contains utilities for generating deterministic storage keys.
// Key format is crucial for:
// 1. Identifying related assets (same content/voice/version)
// 2. Organizing CDN cache invalidation
// 3. Auditing and cleanup
// 4. Parallel processing without collisions

// AudioAssetKey generates a storage key for an audio asset.
// Format: audio/content/{contentID}/version/{version}/voice/{voiceID}/{assetType}/{quality}/audio.{format}
//
// Example:
//   audio/content/conf-123/version/1/voice/voice-456/master/standard/audio.m4a
//   audio/content/conf-123/version/1/voice/voice-456/stream/standard/audio.m4a
//   audio/content/conf-123/version/1/voice/voice-456/preview/standard/audio.m4a
//
// Parameters:
//   contentID: Unique ID of the confession/content
//   version: Version number (integer)
//   voiceID: Unique ID of the voice
//   assetType: "master" | "stream" | "preview" | "download"
//   quality: "standard" | "high" | "lossless"
//   format: "m4a" | "mp3" | "wav" | "flac"
func AudioAssetKey(contentID string, version int, voiceID, assetType, quality, format string) string {
	return fmt.Sprintf(
		"audio/content/%s/version/%d/voice/%s/%s/%s/audio.%s",
		contentID, version, voiceID, assetType, quality, format,
	)
}

// MasterAudioKey generates a key for the master/source audio (before variants).
func MasterAudioKey(contentID string, version int, voiceID string) string {
	return fmt.Sprintf(
		"audio/content/%s/version/%d/voice/%s/master/lossless/audio.wav",
		contentID, version, voiceID,
	)
}

// StreamAudioKey generates a key for streaming audio (CDN-optimized, lower bitrate).
func StreamAudioKey(contentID string, version int, voiceID, quality string) string {
	return fmt.Sprintf(
		"audio/content/%s/version/%d/voice/%s/stream/%s/audio.m4a",
		contentID, version, voiceID, quality,
	)
}

// PreviewAudioKey generates a key for preview audio (short sample for discovery).
func PreviewAudioKey(contentID string, version int, voiceID string) string {
	return fmt.Sprintf(
		"audio/content/%s/version/%d/voice/%s/preview/standard/audio.m4a",
		contentID, version, voiceID,
	)
}

// DownloadAudioKey generates a key for downloadable audio (user's device storage).
func DownloadAudioKey(contentID string, version int, voiceID, quality string) string {
	return fmt.Sprintf(
		"audio/content/%s/version/%d/voice/%s/download/%s/audio.m4a",
		contentID, version, voiceID, quality,
	)
}

// SourceAudioKey generates a key for raw/source audio before processing.
// Temporary location during generation pipeline.
func SourceAudioKey(jobID string) string {
	return fmt.Sprintf("audio/jobs/%s/source/raw.wav", jobID)
}

// ProcessingLogKey generates a key for storing processing pipeline logs for debugging.
func ProcessingLogKey(audioAssetID string) string {
	return fmt.Sprintf("audio/logs/%s/processing.json", audioAssetID)
}

// PrefixForContent generates a prefix to list all audio for a content/version.
func PrefixForContent(contentID string, version int) string {
	return fmt.Sprintf("audio/content/%s/version/%d/", contentID, version)
}

// PrefixForVoice generates a prefix to list all audio for a voice.
func PrefixForVoice(voiceID string) string {
	return fmt.Sprintf("audio/%/voice/%s/", voiceID)
}

// PrefixForJob generates a prefix to list all artifacts for a generation job.
func PrefixForJob(jobID string) string {
	return fmt.Sprintf("audio/jobs/%s/", jobID)
}

// ValidateFormat checks if format is supported.
func ValidateFormat(format string) bool {
	validFormats := map[string]bool{
		"m4a":  true,
		"mp3":  true,
		"wav":  true,
		"flac": true,
		"ogg":  true,
	}
	return validFormats[format]
}

// ValidateAssetType checks if asset type is valid.
func ValidateAssetType(assetType string) bool {
	validTypes := map[string]bool{
		"source":   true,
		"master":   true,
		"stream":   true,
		"preview":  true,
		"download": true,
	}
	return validTypes[assetType]
}

// ValidateQuality checks if quality tier is valid.
func ValidateQuality(quality string) bool {
	validQualities := map[string]bool{
		"standard": true,
		"high":     true,
		"lossless": true,
	}
	return validQualities[quality]
}

// FormatVariants defines available encoding variants by asset type.
var FormatVariants = map[string]map[string]map[string]interface{}{
	"stream": {
		"standard": {
			"format":  "m4a",
			"codec":   "aac",
			"bitrate": 128,
		},
		"high": {
			"format":  "m4a",
			"codec":   "aac",
			"bitrate": 256,
		},
		"lossless": {
			"format":  "m4a",
			"codec":   "aac",
			"bitrate": 320,
		},
	},
	"download": {
		"standard": {
			"format":  "m4a",
			"codec":   "aac",
			"bitrate": 128,
		},
		"high": {
			"format":  "m4a",
			"codec":   "aac",
			"bitrate": 256,
		},
		"lossless": {
			"format":  "flac",
			"codec":   "flac",
			"bitrate": 0, // Variable, typically 800-1200 kbps for speech
		},
	},
	"preview": {
		"standard": {
			"format":  "m4a",
			"codec":   "aac",
			"bitrate": 64,
		},
	},
	"master": {
		"lossless": {
			"format":  "wav",
			"codec":   "pcm",
			"bitrate": 0, // Uncompressed
		},
	},
}
