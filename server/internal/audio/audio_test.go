package audio

import (
	"encoding/binary"
	"errors"
	"testing"
)

// Every duration below is computed independently of the parser: the test
// builds a container around a known number of samples and asserts the parser
// recovers it. A parser that just echoed the input would not survive this.

func buildWAV(seconds int, sampleRate, channels, bits int) []byte {
	byteRate := uint32(sampleRate * channels * bits / 8)
	dataSize := byteRate * uint32(seconds)

	b := []byte("RIFF")
	b = binary.LittleEndian.AppendUint32(b, 4+24+8+dataSize)
	b = append(b, "WAVE"...)

	b = append(b, "fmt "...)
	b = binary.LittleEndian.AppendUint32(b, 16)
	b = binary.LittleEndian.AppendUint16(b, 1) // PCM
	b = binary.LittleEndian.AppendUint16(b, uint16(channels))
	b = binary.LittleEndian.AppendUint32(b, uint32(sampleRate))
	b = binary.LittleEndian.AppendUint32(b, byteRate)
	b = binary.LittleEndian.AppendUint16(b, uint16(channels*bits/8))
	b = binary.LittleEndian.AppendUint16(b, uint16(bits))

	b = append(b, "data"...)
	b = binary.LittleEndian.AppendUint32(b, dataSize)
	b = append(b, make([]byte, dataSize)...)
	return b
}

func buildFLAC(seconds, sampleRate, channels int) []byte {
	totalSamples := uint64(sampleRate * seconds)

	si := make([]byte, 34)
	binary.BigEndian.PutUint32(si[10:14], uint32(sampleRate)<<12)
	si[12] |= byte((channels-1)&7) << 1
	si[12] |= byte(((16 - 1) >> 4) & 1)
	si[13] = byte((16-1)&0x0F) << 4
	si[13] |= byte(totalSamples >> 32 & 0x0F)
	binary.BigEndian.PutUint32(si[14:18], uint32(totalSamples))

	b := []byte("fLaC")
	b = append(b, 0x00) // last block, type 0 = STREAMINFO
	b = append(b, 0, 0, 34)
	return append(b, si...)
}

func buildMP4(seconds, timescale int) []byte {
	mvhd := []byte{0, 0, 0, 0}                    // version 0 + flags
	mvhd = binary.BigEndian.AppendUint32(mvhd, 0) // creation
	mvhd = binary.BigEndian.AppendUint32(mvhd, 0) // modification
	mvhd = binary.BigEndian.AppendUint32(mvhd, uint32(timescale))
	mvhd = binary.BigEndian.AppendUint32(mvhd, uint32(timescale*seconds))

	mdhd := []byte{0, 0, 0, 0}
	mdhd = binary.BigEndian.AppendUint32(mdhd, 0)
	mdhd = binary.BigEndian.AppendUint32(mdhd, 0)
	mdhd = binary.BigEndian.AppendUint32(mdhd, uint32(timescale))
	mdhd = binary.BigEndian.AppendUint32(mdhd, uint32(timescale*seconds))

	box := func(typ string, payload []byte) []byte {
		out := make([]byte, 8, 8+len(payload))
		binary.BigEndian.PutUint32(out[0:4], uint32(8+len(payload)))
		copy(out[4:8], typ)
		return append(out, payload...)
	}

	trak := box("mdia", box("mdhd", mdhd))
	moov := box("mvhd", mvhd)
	moov = append(moov, box("trak", trak)...)

	b := box("ftyp", []byte("M4A "))
	return append(b, box("moov", moov)...)
}

func oggPage(granule int64, payload []byte) []byte {
	segCount := (len(payload) + 254) / 255
	if segCount == 0 {
		segCount = 1
	}
	b := []byte("OggS")
	b = append(b, 0, 0) // version, header type
	b = binary.LittleEndian.AppendUint64(b, uint64(granule))
	b = binary.LittleEndian.AppendUint32(b, 1) // serial
	b = binary.LittleEndian.AppendUint32(b, 0) // sequence
	b = binary.LittleEndian.AppendUint32(b, 0) // checksum
	b = append(b, byte(segCount))
	remaining := len(payload)
	for i := 0; i < segCount; i++ {
		n := remaining
		if n > 255 {
			n = 255
		}
		b = append(b, byte(n))
		remaining -= n
	}
	return append(b, payload...)
}

func buildOGG(seconds, sampleRate, channels int) []byte {
	ident := []byte{0x01}
	ident = append(ident, "vorbis"...)
	ident = binary.LittleEndian.AppendUint32(ident, 0) // version
	ident = append(ident, byte(channels))
	ident = binary.LittleEndian.AppendUint32(ident, uint32(sampleRate))
	ident = binary.LittleEndian.AppendUint32(ident, 0) // bitrate max
	ident = binary.LittleEndian.AppendUint32(ident, 128000)
	ident = binary.LittleEndian.AppendUint32(ident, 0)
	ident = append(ident, 0xB8) // blocksize 0/1
	ident = append(ident, 1)    // framing

	b := oggPage(0, ident)
	// An audio page, then the final page carrying the total granule position.
	b = append(b, oggPage(int64(sampleRate*seconds/2), make([]byte, 100))...)
	return append(b, oggPage(int64(sampleRate*seconds), make([]byte, 100))...)
}

// buildMP3 emits one MPEG1 Layer III frame header at 44100 Hz stereo,
// 128 kbps, optionally with a Xing frame-count header.
func buildMP3(xingFrames int) []byte {
	b := make([]byte, 32)
	b[0], b[1], b[2], b[3] = 0xFF, 0xFB, 0x90, 0x00

	if xingFrames > 0 {
		b = append(b, "Xing"...)
		b = binary.BigEndian.AppendUint32(b, 0x00000003) // frames + bytes present
		b = binary.BigEndian.AppendUint32(b, uint32(xingFrames))
		b = binary.BigEndian.AppendUint32(b, 1_000_000)
	}
	return append(b, make([]byte, 32000)...)
}

func TestMeasuresEverySupportedFormat(t *testing.T) {
	cases := []struct {
		name    string
		payload []byte
		format  string
		wantMS  int
	}{
		{"wav 3s @44.1k stereo", buildWAV(3, 44100, 2, 16), FormatWAV, 3000},
		{"wav 12s @48k mono", buildWAV(12, 48000, 1, 16), FormatWAV, 12000},
		{"flac 5s @44.1k", buildFLAC(5, 44100, 2), FormatFLAC, 5000},
		{"flac 90s @22.05k", buildFLAC(90, 22050, 1), FormatFLAC, 90000},
		{"m4a 7s @44.1k timescale", buildMP4(7, 44100), FormatM4A, 7000},
		{"m4a 180s @1000 timescale", buildMP4(180, 1000), FormatM4A, 180000},
		{"ogg 4s @48k", buildOGG(4, 48000, 2), FormatOGG, 4000},
		{"mp3 with Xing, 1000 frames", buildMP3(1000), FormatMP3, 26122},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := Inspect(tc.payload, tc.format)
			if err != nil {
				t.Fatalf("Inspect: %v", err)
			}
			if r.DurationMillis != tc.wantMS {
				t.Errorf("DurationMillis = %d, want %d", r.DurationMillis, tc.wantMS)
			}
			if !r.Exact {
				t.Errorf("expected an exact duration for %s", tc.format)
			}
			// Ceiling, so a session never under-plans.
			if want := (tc.wantMS + 999) / 1000; r.DurationSeconds != want {
				t.Errorf("DurationSeconds = %d, want %d", r.DurationSeconds, want)
			}
		})
	}
}

func TestMP3WithoutXingFallsBackToBitrate(t *testing.T) {
	payload := buildMP3(0) // no Xing header

	r, err := Inspect(payload, FormatMP3)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if r.Exact {
		t.Error("a bitrate-derived duration must not claim to be exact")
	}
	// 32032 bytes at 128 kbps: 32032*8*1000/128000 = 2002 ms.
	if r.DurationMillis != 2002 {
		t.Errorf("DurationMillis = %d, want 2002", r.DurationMillis)
	}
}

func TestID3TagIsSteppedOver(t *testing.T) {
	mp3 := buildMP3(1000)
	tag := make([]byte, 200, 200+len(mp3))
	copy(tag, "ID3")
	tag[3], tag[4] = 3, 0
	// Synchsafe size of the tag body (190 bytes).
	tag[6], tag[7], tag[8], tag[9] = 0, 0, 1, 0x2E

	payload := append(tag, mp3...)

	r, err := Inspect(payload, FormatMP3)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if r.DurationMillis != 26122 {
		t.Errorf("DurationMillis = %d, want 26122 (the tag must not shift measurement)", r.DurationMillis)
	}
}

func TestRejectsContentThatContradictsItsDeclaredFormat(t *testing.T) {
	wav := buildWAV(3, 44100, 2, 16)

	_, err := Inspect(wav, FormatMP3)
	if !errors.Is(err, ErrFormatMismatch) {
		t.Fatalf("err = %v, want ErrFormatMismatch", err)
	}

	// mp4 and m4a are the same container, so either name must be accepted.
	m4a := buildMP4(7, 44100)
	if _, err := Inspect(m4a, "mp4"); err != nil {
		t.Errorf("Inspect(m4a, \"mp4\") = %v, want nil", err)
	}
}

func TestRejectsPayloadsThatAreNotAudio(t *testing.T) {
	_, err := Inspect([]byte("not audio at all"), FormatMP3)
	if !errors.Is(err, ErrUnsupported) {
		t.Errorf("err = %v, want ErrUnsupported", err)
	}

	_, err = Inspect([]byte("tiny"), FormatMP3)
	if !errors.Is(err, ErrTooSmall) {
		t.Errorf("err = %v, want ErrTooSmall", err)
	}
}

func TestRejectsTruncatedUploads(t *testing.T) {
	// A WAV whose data chunk promises 3s but holds only 0.5s of bytes.
	wav := buildWAV(3, 44100, 2, 16)
	wav = wav[:len(wav)-len(wav)/2]

	r, err := Inspect(wav, FormatWAV)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	// Must reflect the bytes present, not the claim in the header.
	if r.DurationMillis > 1600 {
		t.Errorf("truncated wav measured %dms; must reflect actual bytes, not the header claim",
			r.DurationMillis)
	}
}

func TestRejectsImplausibleDurations(t *testing.T) {
	// A corrupt mp4 claiming 200 years at a 44.1 kHz timescale.
	absurd := buildMP4(1, 44100)
	binary.BigEndian.PutUint32(absurd[len(absurd)-4:], 0xFFFFFFFF)

	_, err := Inspect(absurd, FormatM4A)
	if !errors.Is(err, ErrDurationImplausible) {
		t.Errorf("err = %v, want ErrDurationImplausible", err)
	}
}

func TestSupportedFormatMatchesWhatInspectAccepts(t *testing.T) {
	for _, f := range []string{"m4a", "mp4", "mp3", "wav", "flac", "ogg", "M4A"} {
		if !SupportedFormat(f) {
			t.Errorf("SupportedFormat(%q) = false, want true", f)
		}
	}
	for _, f := range []string{"aac", "wma", "", "opus"} {
		if SupportedFormat(f) {
			t.Errorf("SupportedFormat(%q) = true, want false", f)
		}
	}
}
