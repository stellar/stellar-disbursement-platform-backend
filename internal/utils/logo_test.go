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

func Test_ValidateLogo(t *testing.T) {
	encode := func(w, h int, enc func(*bytes.Buffer, image.Image) error) []byte {
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		img.Set(0, 0, color.RGBA{R: 255, A: 255})
		var buf bytes.Buffer
		require.NoError(t, enc(&buf, img))
		return buf.Bytes()
	}
	pngOf := func(w, h int) []byte {
		return encode(w, h, func(b *bytes.Buffer, i image.Image) error { return png.Encode(b, i) })
	}

	testCases := []struct {
		name    string
		logo    []byte
		wantErr string
	}{
		{name: "real png within the limit", logo: pngOf(300, 300)},
		{name: "real jpeg within the limit", logo: encode(300, 300, func(b *bytes.Buffer, i image.Image) error { return jpeg.Encode(b, i, nil) })},
		{name: "real png exactly at the limit", logo: pngOf(MaxLogoDimension, 1)},
		// The check is header-only: a valid IHDR with no pixel payload passes, proving no full decode happens here.
		{name: "header-only png within the limit", logo: CreatePNGHeaderWithDimensions(t, 100, 100)},
		{name: "not an image", logo: []byte("not-an-image"), wantErr: "invalid file type provided. Expected png or jpeg."},
		// image/gif is registered in this test binary, so this exercises the explicit format allowlist.
		{name: "gif is not an accepted format", logo: encode(1, 1, func(b *bytes.Buffer, i image.Image) error { return gif.Encode(b, i, nil) }), wantErr: "invalid file type provided. Expected png or jpeg."},
		{name: "png signature with a corrupt header", logo: append([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, []byte("not a valid IHDR chunk")...), wantErr: "invalid or corrupt image"},
		{name: "decompression bomb", logo: CreatePNGHeaderWithDimensions(t, 22000, 22000), wantErr: fmt.Sprintf("image dimensions 22000x22000 exceed the %dpx per-side limit", MaxLogoDimension)},
		{name: "width just over the limit", logo: CreatePNGHeaderWithDimensions(t, MaxLogoDimension+1, 10), wantErr: fmt.Sprintf("image dimensions %dx10 exceed the %dpx per-side limit", MaxLogoDimension+1, MaxLogoDimension)},
		{name: "height just over the limit", logo: CreatePNGHeaderWithDimensions(t, 10, MaxLogoDimension+1), wantErr: fmt.Sprintf("image dimensions 10x%d exceed the %dpx per-side limit", MaxLogoDimension+1, MaxLogoDimension)},
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
