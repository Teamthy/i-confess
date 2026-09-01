package media

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
)

const (
	sampleRate = 8000
	toneFreq   = 220.0 // A3 — calm, low tone
)

// ToneBytes generates a gentle sine-tone WAV (8 kHz, 8-bit, mono) of the given
// duration and returns it in memory.
//
// It exists only to provide placeholder audio for the demo/dev environment so
// the full playback loop — signed URLs, range requests, seeking, queueing —
// can be exercised end to end without a TTS bill. Real audio comes from the
// production voice pipeline.
func ToneBytes(seconds int) []byte {
	if seconds <= 0 {
		seconds = 1
	}
	n := sampleRate * seconds
	pcm := make([]byte, n)
	fade := sampleRate / 10
	for i := 0; i < n; i++ {
		t := float64(i) / sampleRate
		// Slow fade in/out avoids clicks at segment boundaries.
		env := 1.0
		if i < fade {
			env = float64(i) / float64(fade)
		} else if i > n-fade {
			env = float64(n-i) / float64(fade)
		}
		sample := math.Sin(2*math.Pi*toneFreq*t) * 0.35 * env
		pcm[i] = uint8(int8(sample * 127))
	}

	var buf bytes.Buffer
	write := func(v any) { _ = binary.Write(&buf, binary.LittleEndian, v) }
	// RIFF header
	write([]byte("RIFF"))
	write(uint32(36 + n)) // chunk size
	write([]byte("WAVE"))
	write([]byte("fmt "))
	write(uint32(16))         // fmt chunk size
	write(uint16(1))          // PCM
	write(uint16(1))          // mono
	write(uint32(sampleRate)) // sample rate
	write(uint32(sampleRate)) // byte rate
	write(uint16(1))          // block align
	write(uint16(8))          // bits per sample
	write([]byte("data"))
	write(uint32(n))
	buf.Write(pcm)
	return buf.Bytes()
}

// WriteTone writes a placeholder tone directly to a filesystem path. Retained
// for tooling; the seeder now writes through the storage layer instead so it
// exercises the same signed-delivery path as production.
func WriteTone(path string, seconds int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, ToneBytes(seconds), 0o644)
}
