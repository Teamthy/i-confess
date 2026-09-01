package images

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

// Avatar processing (§10, §11).

func makeJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 120, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func makePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 10, G: 200, B: uint8(x % 256), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestProcessProducesAllVariants(t *testing.T) {
	res, err := Process(bytes.NewReader(makeJPEG(t, 800, 800)))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Variants) != len(VariantSpecs) {
		t.Fatalf("got %d variants, want %d", len(res.Variants), len(VariantSpecs))
	}
	for _, v := range res.Variants {
		if len(v.Data) == 0 {
			t.Fatalf("variant %s is empty", v.Name)
		}
		if v.Width != v.Height {
			t.Fatalf("variant %s is not square: %dx%d", v.Name, v.Width, v.Height)
		}
		// Each variant must actually decode as an image.
		if _, _, err := image.Decode(bytes.NewReader(v.Data)); err != nil {
			t.Fatalf("variant %s does not decode: %v", v.Name, err)
		}
	}
}

// Smaller variants must actually be smaller files, or serving them to mobile
// achieves nothing.
func TestSmallerVariantsAreSmallerFiles(t *testing.T) {
	res, err := Process(bytes.NewReader(makeJPEG(t, 1024, 1024)))
	if err != nil {
		t.Fatal(err)
	}
	var thumb, large Variant
	for _, v := range res.Variants {
		switch v.Name {
		case "thumbnail":
			thumb = v
		case "large":
			large = v
		}
	}
	if len(thumb.Data) >= len(large.Data) {
		t.Fatalf("thumbnail (%d bytes) is not smaller than large (%d bytes)",
			len(thumb.Data), len(large.Data))
	}
}

// The central privacy guarantee: re-encoding must strip EXIF, including GPS
// coordinates a user did not know their phone attached (§11).
func TestEXIFIsStripped(t *testing.T) {
	base := makeJPEG(t, 400, 400)

	// Splice a minimal APP1/Exif segment carrying a recognisable marker in
	// after the SOI, which is where a camera would put it.
	marker := []byte("GPSLatitudeSECRET")
	exif := append([]byte{0xFF, 0xE1, 0x00, byte(len(marker) + 8), 'E', 'x', 'i', 'f', 0, 0}, marker...)
	withEXIF := append(append(append([]byte{}, base[:2]...), exif...), base[2:]...)

	if !bytes.Contains(withEXIF, marker) {
		t.Fatal("test fixture does not actually contain the marker")
	}

	res, err := Process(bytes.NewReader(withEXIF))
	if err != nil {
		t.Fatalf("image with EXIF was rejected: %v", err)
	}
	for _, v := range res.Variants {
		if bytes.Contains(v.Data, marker) {
			t.Fatalf("EXIF payload survived into variant %s", v.Name)
		}
		if bytes.Contains(v.Data, []byte("Exif")) {
			t.Fatalf("an Exif segment survived into variant %s", v.Name)
		}
	}
}

// A non-image must be refused regardless of what the client calls it.
func TestNonImageIsRejected(t *testing.T) {
	payloads := map[string][]byte{
		"php":    []byte("<?php system($_GET['c']); ?>"),
		"html":   []byte("<html><script>alert(1)</script></html>"),
		"empty":  {},
		"random": bytes.Repeat([]byte{0x41}, 1024),
	}
	for name, data := range payloads {
		if _, err := Process(bytes.NewReader(data)); err == nil {
			t.Fatalf("non-image accepted: %s", name)
		}
	}
}

// A polyglot — valid image bytes with a script appended — must not survive.
// Re-encoding is what guarantees this.
func TestPolyglotPayloadDoesNotSurvive(t *testing.T) {
	payload := []byte("<?php echo 'pwned'; ?>")
	polyglot := append(makeJPEG(t, 300, 300), payload...)

	res, err := Process(bytes.NewReader(polyglot))
	if err != nil {
		t.Fatalf("polyglot rejected outright (acceptable, but check): %v", err)
	}
	for _, v := range res.Variants {
		if bytes.Contains(v.Data, payload) {
			t.Fatalf("appended payload survived into variant %s", v.Name)
		}
	}
}

func TestOversizeUploadRejected(t *testing.T) {
	huge := bytes.Repeat([]byte{0xFF}, MaxBytes+1024)
	_, err := Process(bytes.NewReader(huge))
	if err == nil {
		t.Fatal("oversize upload accepted")
	}
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("error = %v, want ErrTooLarge", err)
	}
}

// A decompression bomb is small on disk and enormous decoded. It must be
// rejected from the header, before any pixels are allocated.
func TestDecompressionBombRejected(t *testing.T) {
	// A 12000x12000 all-white PNG compresses tiny but decodes to ~576 MB.
	img := image.NewGray(image.Rect(0, 0, 12000, 12000))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Skipf("could not build fixture: %v", err)
	}
	if buf.Len() > MaxBytes {
		t.Skipf("fixture is %d bytes, above the byte limit; pixel guard not exercised", buf.Len())
	}

	_, err := Process(bytes.NewReader(buf.Bytes()))
	if err == nil {
		t.Fatal("decompression bomb accepted")
	}
	if !errors.Is(err, ErrTooManyPixels) {
		t.Fatalf("error = %v, want ErrTooManyPixels", err)
	}
}

func TestTooSmallRejected(t *testing.T) {
	_, err := Process(bytes.NewReader(makeJPEG(t, 32, 32)))
	if !errors.Is(err, ErrTooSmall) {
		t.Fatalf("error = %v, want ErrTooSmall", err)
	}
}

// A non-square upload must be centre-cropped, not stretched.
func TestNonSquareIsCroppedNotStretched(t *testing.T) {
	res, err := Process(bytes.NewReader(makeJPEG(t, 1200, 400)))
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range res.Variants {
		if v.Width != v.Height {
			t.Fatalf("variant %s is %dx%d, want square", v.Name, v.Width, v.Height)
		}
	}
	// The square side comes from the shorter edge, so variants cannot exceed it.
	for _, v := range res.Variants {
		if v.Width > 400 {
			t.Fatalf("variant %s upscaled beyond the source's short edge: %d", v.Name, v.Width)
		}
	}
}

// Upscaling a small source wastes bytes and looks worse.
func TestNoUpscaling(t *testing.T) {
	res, err := Process(bytes.NewReader(makeJPEG(t, 100, 100)))
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range res.Variants {
		if v.Width > 100 {
			t.Fatalf("variant %s upscaled to %d from a 100px source", v.Name, v.Width)
		}
	}
}

// PNG sources keep PNG so transparency is not flattened onto black.
func TestPNGStaysPNGAndJPEGStaysJPEG(t *testing.T) {
	pngRes, err := Process(bytes.NewReader(makePNG(t, 300, 300)))
	if err != nil {
		t.Fatal(err)
	}
	if pngRes.Format != "png" {
		t.Fatalf("png source produced %q", pngRes.Format)
	}

	jpegRes, err := Process(bytes.NewReader(makeJPEG(t, 300, 300)))
	if err != nil {
		t.Fatal(err)
	}
	if jpegRes.Format != "jpeg" {
		t.Fatalf("jpeg source produced %q", jpegRes.Format)
	}
}

func TestContentTypeAndExtension(t *testing.T) {
	if ContentType("png") != "image/png" || Extension("png") != "png" {
		t.Fatal("png mapping wrong")
	}
	if ContentType("jpeg") != "image/jpeg" || Extension("jpeg") != "jpg" {
		t.Fatal("jpeg mapping wrong")
	}
	// An unknown format must not produce an empty content type.
	if !strings.HasPrefix(ContentType("weird"), "image/") {
		t.Fatal("unknown format produced a non-image content type")
	}
}
