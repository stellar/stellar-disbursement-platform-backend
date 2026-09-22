package utils

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func encodeTestImage(t *testing.T, w, h int, enc func(*bytes.Buffer, image.Image) error) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	require.NoError(t, enc(&buf, img))
	return buf.Bytes()
}

func testPNG(t *testing.T, w, h int) []byte {
	return encodeTestImage(t, w, h, func(b *bytes.Buffer, i image.Image) error { return png.Encode(b, i) })
}

func testJPEG(t *testing.T, w, h int) []byte {
	return encodeTestImage(t, w, h, func(b *bytes.Buffer, i image.Image) error { return jpeg.Encode(b, i, nil) })
}

func testGIF(t *testing.T, w, h int) []byte {
	return encodeTestImage(t, w, h, func(b *bytes.Buffer, i image.Image) error { return gif.Encode(b, i, nil) })
}

var corruptHeaderPNG = append([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, []byte("not a valid IHDR chunk")...)

func Test_ValidateLogoHeader(t *testing.T) {
	testCases := []struct {
		name    string
		logo    []byte
		wantErr string
	}{
		{name: "real png within the limit", logo: testPNG(t, 300, 300)},
		{name: "real jpeg within the limit", logo: testJPEG(t, 300, 300)},
		{name: "real png exactly at the limit", logo: testPNG(t, MaxLogoDimension, 1)},
		// Header-only: a valid IHDR with no pixel payload passes, proving no full decode happens here.
		{name: "header-only png within the limit", logo: CreatePNGHeaderWithDimensions(t, 100, 100)},
		{name: "not an image", logo: []byte("not-an-image"), wantErr: "invalid file type provided. Expected png or jpeg."},
		// image/gif is registered in this test binary, so this exercises the explicit format allowlist.
		{name: "gif is not an accepted format", logo: testGIF(t, 1, 1), wantErr: "invalid file type provided. Expected png or jpeg."},
		{name: "png signature with a corrupt header", logo: corruptHeaderPNG, wantErr: "invalid or corrupt image"},
		{name: "decompression bomb", logo: CreatePNGHeaderWithDimensions(t, 22000, 22000), wantErr: fmt.Sprintf("image dimensions 22000x22000 exceed the %dpx per-side limit", MaxLogoDimension)},
		{name: "width just over the limit", logo: CreatePNGHeaderWithDimensions(t, MaxLogoDimension+1, 10), wantErr: fmt.Sprintf("image dimensions %dx10 exceed the %dpx per-side limit", MaxLogoDimension+1, MaxLogoDimension)},
		{name: "height just over the limit", logo: CreatePNGHeaderWithDimensions(t, 10, MaxLogoDimension+1), wantErr: fmt.Sprintf("image dimensions 10x%d exceed the %dpx per-side limit", MaxLogoDimension+1, MaxLogoDimension)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateLogoHeader(tc.logo)
			if tc.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tc.wantErr)
			}
		})
	}
}

func Test_ValidateLogo(t *testing.T) {
	testCases := []struct {
		name    string
		logo    []byte
		wantErr string
	}{
		{name: "real png", logo: testPNG(t, 300, 300)},
		{name: "real jpeg", logo: testJPEG(t, 300, 300)},
		// The same header-only logo that ValidateLogoHeader accepts is rejected here by the full decode.
		{name: "header-only png within the limit has no pixel payload", logo: CreatePNGHeaderWithDimensions(t, 100, 100), wantErr: "invalid or corrupt image"},
		// Header checks run first, so a bomb is rejected by its dimensions before anything decodes.
		{name: "decompression bomb", logo: CreatePNGHeaderWithDimensions(t, 22000, 22000), wantErr: fmt.Sprintf("image dimensions 22000x22000 exceed the %dpx per-side limit", MaxLogoDimension)},
		{name: "not an image", logo: []byte("not-an-image"), wantErr: "invalid file type provided. Expected png or jpeg."},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateLogo(tc.logo)
			if tc.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tc.wantErr)
			}
		})
	}
}
