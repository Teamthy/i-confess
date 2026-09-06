package audio

import (
	"encoding/binary"
	"fmt"
)

// ===== MP3 =====

// mp3SampleRates is indexed by the frame header's version id, then its
// sampling-rate index. Version 1 is reserved and therefore all zero.
var mp3SampleRates = [4][3]int{
	{11025, 12000, 8000},  // 0 = MPEG 2.5
	{0, 0, 0},             // 1 = reserved
	{22050, 24000, 16000}, // 2 = MPEG 2
	{44100, 48000, 32000}, // 3 = MPEG 1
}

var (
	mp3BitratesLayerI  = [16]int{0, 32, 64, 96, 128, 160, 192, 224, 256, 288, 320, 352, 384, 416, 448, 0}
	mp3BitratesLayerII = [16]int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0}
	mp3BitratesV2      = [16]int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0}
)

func inspectMP3(b []byte) (Report, error) {
	off := 0

	// Step over a leading ID3v2 tag. Its size is synchsafe.
	if hasPrefix(b, "ID3") {
		if len(b) < 10 {
			return Report{}, fmt.Errorf("%w: truncated ID3v2 tag", ErrTooSmall)
		}
		size := int(b[6]&0x7f)<<21 | int(b[7]&0x7f)<<14 | int(b[8]&0x7f)<<7 | int(b[9]&0x7f)
		off = 10 + size
		if b[5]&0x10 != 0 { // footer present
			off += 10
		}
	}

	// Find the first frame sync. A scan is required because padding or an
	// unsynchronised tag can leave junk between the tag and the audio.
	frame := -1
	for i := off; i+4 <= len(b); i++ {
		if b[i] != 0xFF || b[i+1]&0xE0 != 0xE0 {
			continue
		}
		if _, err := parseMP3Header(b[i:]); err == nil {
			frame = i
			break
		}
	}
	if frame < 0 {
		return Report{}, fmt.Errorf("%w: no valid frame sync found", ErrNoDuration)
	}

	hdr, err := parseMP3Header(b[frame:])
	if err != nil {
		return Report{}, err
	}

	// A Xing/Info header states the frame count exactly, which matters for VBR.
	if frames, ok := mp3XingFrames(b, frame, hdr); ok {
		ms := int(int64(frames) * int64(hdr.samplesPerFrame) * 1000 / int64(hdr.sampleRate))
		return Report{DurationMillis: ms, SampleRate: hdr.sampleRate,
			Channels: hdr.channels, Exact: true}, nil
	}

	if hdr.bitrateKbps <= 0 {
		return Report{}, fmt.Errorf("%w: mp3 has no Xing header and a free bitrate", ErrNoDuration)
	}
	audioBytes := len(b) - frame
	ms := audioBytes * 8 * 1000 / (hdr.bitrateKbps * 1000)
	return Report{DurationMillis: ms, SampleRate: hdr.sampleRate,
		Channels: hdr.channels, Exact: false}, nil
}

type mp3Header struct {
	sampleRate      int
	bitrateKbps     int
	channels        int
	samplesPerFrame int
}

func parseMP3Header(b []byte) (mp3Header, error) {
	var h mp3Header
	if len(b) < 4 {
		return h, fmt.Errorf("%w: frame header", ErrTooSmall)
	}
	b1, b2, b3 := b[1], b[2], b[3]

	versionID := (b1 >> 3) & 3
	layer := (b1 >> 1) & 3
	if versionID == 1 || layer == 0 {
		return h, fmt.Errorf("%w: reserved version/layer", ErrUnsupported)
	}

	bitrateIdx := (b2 >> 4) & 0xF
	srateIdx := (b2 >> 2) & 3
	if bitrateIdx == 0xF || srateIdx == 3 {
		return h, fmt.Errorf("%w: bad bitrate or sample-rate index", ErrUnsupported)
	}

	h.sampleRate = mp3SampleRates[versionID][srateIdx]
	if h.sampleRate == 0 {
		return h, fmt.Errorf("%w: unsupported sample rate", ErrUnsupported)
	}

	switch layer {
	case 3: // Layer I
		h.bitrateKbps = mp3BitratesLayerI[bitrateIdx]
		h.samplesPerFrame = 384
	case 2: // Layer II
		h.bitrateKbps = mp3BitratesLayerII[bitrateIdx]
		h.samplesPerFrame = 1152
	default: // Layer III
		if versionID == 3 {
			h.bitrateKbps = mp3BitratesLayerII[bitrateIdx]
			h.samplesPerFrame = 1152
		} else {
			h.bitrateKbps = mp3BitratesV2[bitrateIdx]
			h.samplesPerFrame = 576
		}
	}

	if (b3>>6)&3 == 3 { // mono
		h.channels = 1
	} else {
		h.channels = 2
	}
	return h, nil
}

// mp3XingFrames reads the frame count from a Xing/Info header when present.
func mp3XingFrames(b []byte, frame int, h mp3Header) (int, bool) {
	// The header sits just past the frame's side information, whose size
	// depends on version and channel mode. MPEG 1 carries more of it.
	offset := 9
	if h.channels != 1 {
		offset = 17
	}
	if h.samplesPerFrame == 1152 {
		if h.channels == 1 {
			offset = 17
		} else {
			offset = 32
		}
	}

	at := frame + offset
	if at+12 > len(b) {
		return 0, false
	}
	tag := string(b[at : at+4])
	if tag != "Xing" && tag != "Info" {
		return 0, false
	}
	flags := binary.BigEndian.Uint32(b[at+4 : at+8])
	if flags&0x01 == 0 { // no frame-count field
		return 0, false
	}
	frames := int(binary.BigEndian.Uint32(b[at+8 : at+12]))
	if frames <= 0 {
		return 0, false
	}
	return frames, true
}

// ===== MP4 / M4A =====

func inspectMP4(b []byte) (Report, error) {
	var timescale, duration uint64

	var walk func(off, end, depth int) error
	walk = func(off, end, depth int) error {
		if depth > 8 {
			return nil
		}
		for off+8 <= end {
			size := uint64(binary.BigEndian.Uint32(b[off : off+4]))
			typ := string(b[off+4 : off+8])
			hdr := 8

			switch size {
			case 1: // 64-bit size follows
				if off+16 > end {
					return nil
				}
				size = binary.BigEndian.Uint64(b[off+8 : off+16])
				hdr = 16
			case 0: // box extends to end of file
				size = uint64(end - off)
			}
			if size < uint64(hdr) {
				return nil
			}
			boxEnd := off + int(size)
			if boxEnd > end {
				boxEnd = end
			}

			switch typ {
			case "moov", "trak", "mdia", "minf", "stbl":
				if err := walk(off+hdr, boxEnd, depth+1); err != nil {
					return err
				}
			case "mvhd", "mdhd":
				// Prefer mdhd: it describes the audio track rather than the
				// whole movie, which can differ once editing is involved.
				if ts, dur, ok := parseMVHD(b, off+hdr, boxEnd); ok && (typ == "mdhd" || timescale == 0) {
					timescale, duration = ts, dur
				}
			}
			off = boxEnd
		}
		return nil
	}

	if err := walk(0, len(b), 0); err != nil {
		return Report{}, err
	}
	if timescale == 0 || duration == 0 {
		return Report{}, fmt.Errorf("%w: mp4 has no mvhd/mdhd duration", ErrNoDuration)
	}

	ms := int(duration * 1000 / timescale)
	return Report{DurationMillis: ms, Exact: true}, nil
}

func parseMVHD(b []byte, off, end int) (timescale, duration uint64, ok bool) {
	if off+4 > end {
		return 0, 0, false
	}
	version := b[off]
	off += 4 // version + flags

	if version == 1 {
		if off+28 > end {
			return 0, 0, false
		}
		timescale = uint64(binary.BigEndian.Uint32(b[off+16 : off+20]))
		duration = binary.BigEndian.Uint64(b[off+20 : off+28])
	} else {
		if off+16 > end {
			return 0, 0, false
		}
		timescale = uint64(binary.BigEndian.Uint32(b[off+8 : off+12]))
		duration = uint64(binary.BigEndian.Uint32(b[off+12 : off+16]))
	}
	if timescale == 0 {
		return 0, 0, false
	}
	return timescale, duration, true
}

// ===== WAV =====

func inspectWAV(b []byte) (Report, error) {
	if !hasPrefix(b, "RIFF") || !hasPrefixAt(b, 8, "WAVE") {
		return Report{}, fmt.Errorf("%w: not a RIFF/WAVE file", ErrFormatMismatch)
	}

	var byteRate, dataSize, channels, sampleRate uint32
	off := 12
	for off+8 <= len(b) {
		id := string(b[off : off+4])
		size := binary.LittleEndian.Uint32(b[off+4 : off+8])
		body := off + 8

		switch id {
		case "fmt ":
			if body+16 <= len(b) {
				channels = uint32(binary.LittleEndian.Uint16(b[body+2 : body+4]))
				sampleRate = binary.LittleEndian.Uint32(b[body+4 : body+8])
				byteRate = binary.LittleEndian.Uint32(b[body+8 : body+12])
			}
		case "data":
			dataSize = size
			// A truncated upload declares more data than it holds. Trust the
			// bytes actually present, or the duration would be overstated.
			if avail := uint32(len(b) - body); dataSize > avail {
				dataSize = avail
			}
		}

		off = body + int(size)
		if size%2 == 1 {
			off++ // chunks are word-aligned
		}
	}

	if byteRate == 0 || dataSize == 0 {
		return Report{}, fmt.Errorf("%w: wav has no byte rate or data", ErrNoDuration)
	}

	ms := int(uint64(dataSize) * 1000 / uint64(byteRate))
	return Report{
		DurationMillis: ms,
		SampleRate:     int(sampleRate),
		Channels:       int(channels),
		Exact:          true,
	}, nil
}

// ===== FLAC =====

func inspectFLAC(b []byte) (Report, error) {
	if !hasPrefix(b, "fLaC") {
		return Report{}, fmt.Errorf("%w: not a FLAC file", ErrFormatMismatch)
	}

	off := 4
	for off+4 <= len(b) {
		last := b[off]&0x80 != 0
		blockType := b[off] & 0x7F
		length := int(b[off+1])<<16 | int(b[off+2])<<8 | int(b[off+3])
		body := off + 4

		if blockType == 0 { // STREAMINFO
			if body+18 > len(b) {
				return Report{}, fmt.Errorf("%w: truncated STREAMINFO", ErrTooSmall)
			}
			s := b[body:]
			sampleRate := int(binary.BigEndian.Uint32(s[10:14]) >> 12)
			channels := int((s[12]>>1)&0x07) + 1
			totalSamples := (uint64(s[13]&0x0F) << 32) | uint64(binary.BigEndian.Uint32(s[14:18]))

			if sampleRate == 0 || totalSamples == 0 {
				return Report{}, fmt.Errorf("%w: FLAC reports no samples", ErrNoDuration)
			}
			return Report{
				DurationMillis: int(totalSamples * 1000 / uint64(sampleRate)),
				SampleRate:     sampleRate,
				Channels:       channels,
				Exact:          true,
			}, nil
		}

		off = body + length
		if last {
			break
		}
	}
	return Report{}, fmt.Errorf("%w: FLAC has no STREAMINFO block", ErrNoDuration)
}

// ===== OGG =====

func inspectOGG(b []byte) (Report, error) {
	var sampleRate, channels int
	var lastGranule int64 = -1

	off := 0
	for off+27 <= len(b) {
		if string(b[off:off+4]) != "OggS" {
			break
		}
		granule := int64(binary.LittleEndian.Uint64(b[off+6 : off+14]))
		segCount := int(b[off+26])
		if off+27+segCount > len(b) {
			break
		}
		segments := b[off+27 : off+27+segCount]

		dataSize := 0
		for _, s := range segments {
			dataSize += int(s)
		}
		body := off + 27 + segCount
		if body+dataSize > len(b) {
			dataSize = len(b) - body
		}

		// The first page carries the Vorbis identification header.
		if sampleRate == 0 && dataSize >= 16 && b[body] == 0x01 &&
			string(b[body+1:body+7]) == "vorbis" {
			channels = int(b[body+11])
			sampleRate = int(binary.LittleEndian.Uint32(b[body+12 : body+16]))
		}

		if granule > lastGranule {
			lastGranule = granule
		}
		off = body + dataSize
	}

	if sampleRate == 0 {
		return Report{}, fmt.Errorf("%w: ogg has no Vorbis identification header", ErrNoDuration)
	}
	if lastGranule <= 0 {
		return Report{}, fmt.Errorf("%w: ogg reports no granule position", ErrNoDuration)
	}

	return Report{
		DurationMillis: int(lastGranule * 1000 / int64(sampleRate)),
		SampleRate:     sampleRate,
		Channels:       channels,
		Exact:          true,
	}, nil
}
