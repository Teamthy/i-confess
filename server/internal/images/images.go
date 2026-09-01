// Package images validates and processes user-uploaded images (§10, §11).
//
// Three rules:
//
//  1. The client's declared content type is ignored. Format is determined by
//     decoding the bytes, because a Content-Type header is attacker-controlled
//     and "image/png" on a PHP payload is the oldest upload bug there is.
//  2. Images are re-encoded, never passed through. Re-encoding is what
//     guarantees EXIF is gone — including GPS coordinates a user did not know
//     their phone attached — and that no polyglot payload survives.
//  3. Decoding is bounded before it is attempted, so a "decompression bomb"
//     cannot exhaust memory.
package images

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"

	// Registered so DecodeConfig recognises the formats, even though we only
	// ever re-encode to JPEG/PNG.
	_ "image/gif"
)

// Limits on accepted uploads.
const (
	// MaxBytes caps the upload. Generous for a photo, far below what would
	// stress the server.
	MaxBytes = 8 << 20 // 8 MiB
	// MaxPixels caps total decoded area. A 20000x20000 PNG is only a few
	// hundred KB compressed but allocates ~1.6 GB decoded, so the pixel budget
	// — not the byte size — is what actually prevents the bomb.
	MaxPixels = 40_000_000 // 40 MP
	// MinDimension rejects images too small to be a usable avatar.
	MinDimension = 64
)

// Errors callers distinguish so they can return precise messages.
var (
	ErrTooLarge      = errors.New("image exceeds the maximum size")
	ErrTooManyPixels = errors.New("image dimensions are too large")
	ErrTooSmall      = errors.New("image is too small")
	ErrUnsupported   = errors.New("unsupported image format")
	ErrCorrupt       = errors.New("image could not be decoded")
)

// Variant is one rendered size (§11).
type Variant struct {
	Name   string
	Size   int
	Data   []byte
	Format string
	Width  int
	Height int
}

// VariantSpecs are the avatar sizes produced for every upload.
//
// Serving a 4 MB original to a 40 px list row wastes the user's data plan, so
// each surface gets a size that suits it.
var VariantSpecs = []struct {
	Name string
	Size int
}{
	{"thumbnail", 64},
	{"small", 128},
	{"medium", 256},
	{"large", 512},
}

// Result is the outcome of processing an upload.
type Result struct {
	Variants []Variant
	// Format is the encoding used for output.
	Format string
	// OriginalWidth and OriginalHeight describe the source, for audit.
	OriginalWidth  int
	OriginalHeight int
}

// Process validates and re-encodes an uploaded avatar into square variants.
func Process(r io.Reader) (*Result, error) {
	// Read one byte past the limit so an oversize upload is detected rather
	// than silently truncated into a "valid" image.
	data, err := io.ReadAll(io.LimitReader(r, MaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read upload: %w", err)
	}
	if len(data) > MaxBytes {
		return nil, ErrTooLarge
	}
	if len(data) == 0 {
		return nil, ErrCorrupt
	}

	// Inspect the header before decoding pixels: DecodeConfig reads only
	// dimensions, so a bomb is rejected without ever being allocated.
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, ErrUnsupported
	}
	if !supportedFormat(format) {
		return nil, ErrUnsupported
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, ErrCorrupt
	}
	if cfg.Width*cfg.Height > MaxPixels {
		return nil, ErrTooManyPixels
	}
	if cfg.Width < MinDimension || cfg.Height < MinDimension {
		return nil, ErrTooSmall
	}

	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, ErrCorrupt
	}

	// Alpha is preserved only for PNG sources; everything else becomes JPEG,
	// which is far smaller for photographs.
	outFormat := "jpeg"
	if format == "png" {
		outFormat = "png"
	}

	square := cropSquare(src)

	out := &Result{
		Format:         outFormat,
		OriginalWidth:  cfg.Width,
		OriginalHeight: cfg.Height,
	}
	for _, spec := range VariantSpecs {
		// Never upscale: enlarging a small source produces a blurry image and
		// a bigger file for no benefit.
		size := spec.Size
		if b := square.Bounds(); size > b.Dx() {
			size = b.Dx()
		}
		resized := resize(square, size)
		encoded, err := encode(resized, outFormat)
		if err != nil {
			return nil, fmt.Errorf("encode %s: %w", spec.Name, err)
		}
		out.Variants = append(out.Variants, Variant{
			Name: spec.Name, Size: spec.Size, Data: encoded,
			Format: outFormat, Width: size, Height: size,
		})
	}
	return out, nil
}

func supportedFormat(f string) bool {
	switch f {
	case "jpeg", "png", "gif":
		return true
	}
	return false
}

// cropSquare takes the largest centred square, so avatars are consistent
// without distorting the subject by stretching.
func cropSquare(src image.Image) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == h {
		return src
	}
	side := w
	if h < w {
		side = h
	}
	x0 := b.Min.X + (w-side)/2
	y0 := b.Min.Y + (h-side)/2
	rect := image.Rect(x0, y0, x0+side, y0+side)

	dst := image.NewRGBA(image.Rect(0, 0, side, side))
	draw.Draw(dst, dst.Bounds(), src, rect.Min, draw.Src)
	return dst
}

// resize scales a square image using box sampling.
//
// Box sampling (averaging the source pixels covering each destination pixel)
// rather than nearest-neighbour: nearest-neighbour aliases badly at avatar
// sizes, which is exactly where quality is most visible.
func resize(src image.Image, size int) image.Image {
	b := src.Bounds()
	srcSize := b.Dx()
	dst := image.NewRGBA(image.Rect(0, 0, size, size))

	if srcSize == size {
		draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
		return dst
	}

	scale := float64(srcSize) / float64(size)
	for y := 0; y < size; y++ {
		sy0 := b.Min.Y + int(float64(y)*scale)
		sy1 := b.Min.Y + int(float64(y+1)*scale)
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for x := 0; x < size; x++ {
			sx0 := b.Min.X + int(float64(x)*scale)
			sx1 := b.Min.X + int(float64(x+1)*scale)
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}

			var rs, gs, bs, as, n uint64
			for sy := sy0; sy < sy1 && sy < b.Max.Y; sy++ {
				for sx := sx0; sx < sx1 && sx < b.Max.X; sx++ {
					r, g, bl, a := src.At(sx, sy).RGBA()
					rs += uint64(r)
					gs += uint64(g)
					bs += uint64(bl)
					as += uint64(a)
					n++
				}
			}
			if n == 0 {
				continue
			}
			dst.Set(x, y, rgba16{
				r: uint16(rs / n), g: uint16(gs / n),
				b: uint16(bs / n), a: uint16(as / n),
			})
		}
	}
	return dst
}

// rgba16 carries the 16-bit averages RGBA() produces without truncating early.
type rgba16 struct{ r, g, b, a uint16 }

func (c rgba16) RGBA() (r, g, b, a uint32) {
	return uint32(c.r), uint32(c.g), uint32(c.b), uint32(c.a)
}

func encode(img image.Image, format string) ([]byte, error) {
	var buf bytes.Buffer
	switch format {
	case "png":
		enc := png.Encoder{CompressionLevel: png.BestCompression}
		if err := enc.Encode(&buf, img); err != nil {
			return nil, err
		}
	default:
		// Quality 85 is the usual point where further increases cost bytes
		// without a visible difference at avatar sizes.
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// ContentType returns the MIME type for an output format.
func ContentType(format string) string {
	if format == "png" {
		return "image/png"
	}
	return "image/jpeg"
}

// Extension returns the file extension for an output format.
func Extension(format string) string {
	if format == "png" {
		return "png"
	}
	return "jpg"
}
