package media

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
)

// WriteTone generates a gentle sine-tone WAV file (8 kHz, 8-bit, mono) of the
// given duration. It exists only to provide local placeholder audio for the
// demo/dev environment so the full playback loop can be exercised end-to-end.
// Real audio comes from the production TTS/recording pipeline.
func WriteTone(path string, seconds int) error {
	const sampleRate = 8000
	const freq = 220.0 // A3 — calm, low tone

	n := sampleRate * seconds
	data := make([]byte, n)
	for i := 0; i < n; i++ {
		t := float64(i) / sampleRate
		// Soft sine with a slow fade-in/out to avoid clicks.
		env := 1.0
		if i < sampleRate/10 {
			env = float64(i) / (sampleRate / 10)
		} else if i > n-sampleRate/10 {
			env = float64(n-i) / (sampleRate / 10)
		}
		sample := math.Sin(2*math.Pi*freq*t) * 0.35 * env
		data[i] = uint8(int8(sample * 127))
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// RIFF header
	write := func(v any) { _ = binary.Write(f, binary.LittleEndian, v) }
	write([]byte("RIFF"))
	write(uint32(36 + n)) // chunk size
	write([]byte("WAVE"))
	write([]byte("fmt "))
	write(uint32(16))            // fmt chunk size
	write(uint16(1))             // PCM
	write(uint16(1))             // mono
	write(uint32(sampleRate))    // sample rate
	write(uint32(sampleRate))    // byte rate
	write(uint16(1))             // block align
	write(uint16(8))             // bits per sample
	write([]byte("data"))
	write(uint32(n))
	_, err = f.Write(data)
	return err
}
