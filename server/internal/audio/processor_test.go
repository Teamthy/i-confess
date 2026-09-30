package audio

import (
	"context"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
)

// These tests deliberately avoid a media binary. Everything that can be
// checked without ffmpeg is: the path derivation, the duration parsing and the
// waveform arithmetic. Tests that need the real binaries skip when they are
// absent, so the suite stays useful on a laptop without ffmpeg installed.

func TestDefaultFFprobePathSitsBesideFFmpeg(t *testing.T) {
	cases := []struct{ in, want string }{
		{"ffmpeg", "ffprobe"},
		{"/usr/bin/ffmpeg", "/usr/bin/ffprobe"},
		{"/opt/ffmpeg-6.0/bin/ffmpeg", "/opt/ffmpeg-6.0/bin/ffprobe"},
		// An unrelated path must not be mangled into something that does not
		// exist; falling back to PATH lookup is the safer guess.
		{"avconv", "ffprobe"},
	}
	for _, c := range cases {
		if got := defaultFFprobePath(c.in); got != c.want {
			t.Errorf("defaultFFprobePath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseDurationAcceptsWhatFFprobePrints(t *testing.T) {
	cases := []struct {
		in      string
		want    float64
		wantErr bool
	}{
		{"123.456", 123.456, false},
		{"123.456\n", 123.456, false},
		{"  12.5  ", 12.5, false},
		{"60", 60, false},
		// ffprobe prints "N/A" for a stream with no duration in the header.
		// That must be an error, not a silent zero that a session planner
		// would then schedule against.
		{"N/A", 0, true},
		{"", 0, true},
	}
	for _, c := range cases {
		got, err := parseDuration(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseDuration(%q) = %v, want error", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseDuration(%q) unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseDuration(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestNewProcessorRefusesAMissingBinary(t *testing.T) {
	// A bogus path is portable: it fails on every machine, with or without
	// ffmpeg installed.
	_, err := NewProcessor(ProcessorConfig{
		TempDir:     t.TempDir(),
		FFmpegPath:  "definitely-not-ffmpeg-on-this-system",
		FFprobePath: "definitely-not-ffprobe-on-this-system",
	})
	if !errors.Is(err, ErrFFmpegNotAvailable) {
		t.Fatalf("NewProcessor with a missing binary = %v, want ErrFFmpegNotAvailable", err)
	}
	// The message has to name the binary, or an operator chasing the failure
	// cannot tell ffmpeg from ffprobe.
	if !strings.Contains(err.Error(), "definitely-not-ffmpeg-on-this-system") {
		t.Errorf("error should name the missing binary, got %q", err.Error())
	}
}

// pcm is a helper that renders samples as little-endian 16-bit PCM.
func pcm(samples ...int16) []byte {
	b := make([]byte, 0, len(samples)*2)
	for _, s := range samples {
		b = binary.LittleEndian.AppendUint16(b, uint16(s))
	}
	return b
}

func TestPCMPeaksTakesTheLoudestSampleInEachBucket(t *testing.T) {
	// Four buckets of two samples. The second sample of each pair is the
	// louder one, so a parser that took the first sample - or the average -
	// would not produce 0.5/1.0.
	raw := pcm(100, -16384, 200, -32768, 50, 60, 10, 20)
	got, ok := pcmPeaks(raw, 4)
	if !ok {
		t.Fatal("pcmPeaks returned no peaks for valid input")
	}
	// Normalised against the loudest bucket (32768/32768 = 1).
	want := []float64{0.5, 1, 60.0 / 32768, 20.0 / 32768}
	for i := range want {
		if diff := got[i] - want[i]; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("bucket %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestPCMPeaksHandlesNegativePeaksAndTruncatedFrames(t *testing.T) {
	// A quiet negative sample must still beat zero: abs() before compare.
	// Output is normalised against the loudest bucket, so the pair is
	// expected as a ratio, not as raw amplitudes.
	got, ok := pcmPeaks(pcm(-5, -300), 2)
	if !ok {
		t.Fatal("pcmPeaks returned no peaks for valid input")
	}
	if got[1] != 1 {
		t.Errorf("loudest bucket = %v, want 1 after normalisation", got[1])
	}
	if got[0] <= 0 || got[0] >= 1 {
		t.Errorf("quieter bucket = %v, want a ratio in (0,1)", got[0])
	}
	const wantRatio = 5.0 / 300.0
	if diff := got[0] - wantRatio; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("quieter bucket = %v, want %v", got[0], wantRatio)
	}

	// An odd trailing byte is a truncated frame. Dropping it keeps every
	// earlier sample aligned instead of shifting them all by one byte, so the
	// two real samples still land one per bucket in the right order.
	raw := append(pcm(1000, 2000), 0x7f)
	got, ok = pcmPeaks(raw, 2)
	if !ok {
		t.Fatal("pcmPeaks returned no peaks for a truncated frame")
	}
	if got[1] != 1 {
		t.Errorf("second bucket = %v, want 1 after normalisation", got[1])
	}
	if diff := got[0] - 0.5; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("first bucket = %v, want 0.5: the truncated byte shifted the samples", got[0])
	}
}

func TestPCMPeaksDistinguishesSilenceFromNoSamples(t *testing.T) {
	// Silence is drawable: all zeros, but present.
	got, ok := pcmPeaks(pcm(0, 0, 0, 0), 2)
	if !ok {
		t.Fatal("silence must be representable, not reported as a decode failure")
	}
	for i, v := range got {
		if v != 0 {
			t.Errorf("bucket %d = %v, want 0", i, v)
		}
	}

	// Nothing decoded at all is a failure the caller must not draw.
	if _, ok := pcmPeaks(nil, 8); ok {
		t.Error("pcmPeaks(nil) should report no peaks")
	}
	if _, ok := pcmPeaks([]byte{0x01}, 8); ok {
		t.Error("a single byte is not a sample and should report no peaks")
	}
}

func TestPCMPeaksFillsFewerBucketsThanSamplesAndPadsMore(t *testing.T) {
	// More buckets than samples: the tail is zero, not a repeat of the last
	// sample. Inventing bars would draw audio that is not there.
	got, _ := pcmPeaks(pcm(30000), 4)
	if got[0] != 1 {
		t.Errorf("first bucket = %v, want 1 (normalised peak)", got[0])
	}
	for i := 1; i < len(got); i++ {
		if got[i] != 0 {
			t.Errorf("bucket %d = %v, want 0 for a bucket with no samples", i, got[i])
		}
	}

	// Fewer buckets than samples must not drop the final samples: the last
	// bucket absorbs the remainder rather than the tail being discarded.
	got, _ = pcmPeaks(pcm(1, 1, 1, 1, 1, 1, 1, 32767), 3)
	if got[2] != 1 {
		t.Errorf("last bucket = %v, want 1: the final samples were dropped", got[2])
	}
}

func TestGetWaveformRejectsBadArgumentsBeforeShellingOut(t *testing.T) {
	// These must fail on argument validation alone, so they hold on a machine
	// with no ffmpeg installed and prove the old "return a sine wave anyway"
	// fallback is gone.
	if _, err := (Processor{}).GetWaveform(context.Background(), "", 10); err == nil {
		t.Error("GetWaveform with an empty path should fail")
	}
	if _, err := (Processor{}).GetWaveform(context.Background(), "x.wav", 0); err == nil {
		t.Error("GetWaveform with width 0 should fail")
	}
	if _, err := (Processor{}).GetWaveform(context.Background(), "x.wav", -1); err == nil {
		t.Error("GetWaveform with a negative width should fail")
	}
	if _, err := (Processor{}).GetDuration(context.Background(), ""); err == nil {
		t.Error("GetDuration with an empty path should fail")
	}
}
