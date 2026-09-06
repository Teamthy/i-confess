// Package audio inspects audio bytes before anything trusts them.
//
// Two rules shape this package:
//
//  1. Duration is measured, never accepted. The session planner schedules
//     against duration_seconds, so a duration supplied by a client, an admin
//     form or a voice provider is a claim, not a fact. Everything downstream
//     uses the measured value.
//  2. Content is verified against its declared format. A file that says it is
//     m4a and is actually a truncated mp3 must be rejected at the boundary,
//     not discovered by a listener mid-session.
//
// Measurement is done by parsing container headers in-process. No ffmpeg, no
// external binary, no network (§47 keeps the deploy surface small). The formats
// are the ones audio_assets.format allows.
package audio

import (
	"errors"
	"fmt"
	"strings"
)

// Formats this package can inspect. Mirrors audio_assets.format.
const (
	FormatM4A  = "m4a"
	FormatMP3  = "mp3"
	FormatWAV  = "wav"
	FormatFLAC = "flac"
	FormatOGG  = "ogg"
)

// Errors returned by Inspect. Callers use errors.Is; none of these are
// retryable, because re-uploading the same bytes cannot make them valid.
var (
	// ErrUnsupported means the format is not one this package can inspect.
	ErrUnsupported = errors.New("audio: unsupported format")
	// ErrTooSmall means the payload is too short to hold a valid header.
	ErrTooSmall = errors.New("audio: payload too small to be valid audio")
	// ErrFormatMismatch means the bytes are a different format than declared.
	ErrFormatMismatch = errors.New("audio: content does not match declared format")
	// ErrNoDuration means the container carries no usable duration.
	ErrNoDuration = errors.New("audio: duration could not be determined")
	// ErrDurationImplausible means the measured duration is outside bounds that
	// any real confession audio must satisfy.
	ErrDurationImplausible = errors.New("audio: measured duration is implausible")
)

// MinDurationSeconds rejects silence-length or empty payloads. A confession
// variant is spoken text; anything shorter is a truncated upload.
const MinDurationSeconds = 1

// MaxDurationSeconds rejects corrupt containers that report absurd durations,
// which would otherwise poison the session planner.
const MaxDurationSeconds = 4 * 60 * 60

// MinPayloadBytes is the smallest byte count that can hold any supported
// header. Anything smaller is not audio.
const MinPayloadBytes = 12

// Report is what inspection produces.
type Report struct {
	// Format is the format the bytes actually are, sniffed from content.
	Format string
	// DeclaredFormat is what the caller said the bytes were. Empty if the
	// caller made no claim.
	DeclaredFormat string
	// DurationSeconds is measured from the container, never from a caller.
	DurationSeconds int
	// DurationMillis retains sub-second precision for queue planning.
	DurationMillis int
	// SampleRate in Hz, when the container reports it.
	SampleRate int
	// Channels, when the container reports it.
	Channels int
	// SizeBytes is len(payload).
	SizeBytes int
	// Exact is true when the container states a duration authoritatively
	// (wav, flac, m4a, ogg). False for mp3 without a Xing header, where the
	// value is derived from the bitrate and is accurate to within a frame.
	Exact bool
}

// Inspect measures and validates audio bytes.
//
// declaredFormat may be empty, in which case the sniffed format is accepted
// without comparison. When it is set and the content disagrees, inspection
// fails with ErrFormatMismatch rather than silently adopting the sniffed value.
func Inspect(payload []byte, declaredFormat string) (Report, error) {
	if len(payload) < MinPayloadBytes {
		return Report{}, fmt.Errorf("%w: got %d bytes", ErrTooSmall, len(payload))
	}

	format, err := sniff(payload)
	if err != nil {
		return Report{}, err
	}

	declared := strings.ToLower(strings.TrimSpace(declaredFormat))
	switch declared {
	case "":
		// No claim to check.
	case "m4a", "mp4":
		// mp4 and m4a are the same container; either may name an m4a file.
		if format != FormatM4A {
			return Report{}, fmt.Errorf("%w: declared %q but content is %s", ErrFormatMismatch, declaredFormat, format)
		}
	default:
		if declared != format {
			return Report{}, fmt.Errorf("%w: declared %q but content is %s", ErrFormatMismatch, declaredFormat, format)
		}
	}

	var r Report
	switch format {
	case FormatMP3:
		r, err = inspectMP3(payload)
	case FormatM4A:
		r, err = inspectMP4(payload)
	case FormatWAV:
		r, err = inspectWAV(payload)
	case FormatFLAC:
		r, err = inspectFLAC(payload)
	case FormatOGG:
		r, err = inspectOGG(payload)
	default:
		return Report{}, fmt.Errorf("%w: %s", ErrUnsupported, format)
	}
	if err != nil {
		return Report{}, err
	}

	r.Format = format
	r.DeclaredFormat = declared
	r.SizeBytes = len(payload)

	if r.DurationMillis <= 0 {
		return Report{}, fmt.Errorf("%w: %s reported no usable duration", ErrNoDuration, format)
	}

	if r.DurationMillis < MinDurationSeconds*1000 {
		return Report{}, fmt.Errorf("%w: %dms is below the %ds minimum",
			ErrDurationImplausible, r.DurationMillis, MinDurationSeconds)
	}
	if r.DurationMillis > MaxDurationSeconds*1000 {
		return Report{}, fmt.Errorf("%w: %dms exceeds the %ds ceiling",
			ErrDurationImplausible, r.DurationMillis, MaxDurationSeconds)
	}

	// DurationSeconds is the ceiling, not the floor: a 1.4s clip is 2s of
	// queue time, and under-counting would let a session overrun its plan.
	r.DurationSeconds = (r.DurationMillis + 999) / 1000
	return r, nil
}

// SupportedFormat reports whether this package can inspect the named format.
func SupportedFormat(format string) bool {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case FormatM4A, "mp4", FormatMP3, FormatWAV, FormatFLAC, FormatOGG:
		return true
	}
	return false
}

// sniff identifies a format from its magic bytes.
func sniff(b []byte) (string, error) {
	switch {
	case hasPrefix(b, "ID3"):
		// ID3v2 tag; an mp3 always follows it.
		return FormatMP3, nil
	case len(b) > 1 && b[0] == 0xFF && b[1]&0xE0 == 0xE0:
		return FormatMP3, nil
	case hasPrefix(b, "RIFF"):
		return FormatWAV, nil
	case hasPrefix(b, "fLaC"):
		return FormatFLAC, nil
	case hasPrefix(b, "OggS"):
		return FormatOGG, nil
	case hasPrefixAt(b, 4, "ftyp"):
		return FormatM4A, nil
	}
	return "", fmt.Errorf("%w: unrecognised magic bytes %.4q", ErrUnsupported, b[:min(len(b), 4)])
}

func hasPrefix(b []byte, s string) bool {
	return len(b) >= len(s) && string(b[:len(s)]) == s
}

func hasPrefixAt(b []byte, at int, s string) bool {
	return len(b) >= at+len(s) && string(b[at:at+len(s)]) == s
}
