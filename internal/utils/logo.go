package utils

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"strings"

	// Don't remove the `image/jpeg` and `image/png` packages import unless
	// the `image` package is no longer necessary.
	// It registers the `Decoders` to handle the image decoding - `image.Decode`.
	// See https://pkg.go.dev/image#pkg-overview
	_ "image/jpeg"
	_ "image/png"
)

type LogoType string

const (
	PNGLogoType      LogoType = "png"
	JPEGLogoType     LogoType = "jpeg"
	MaxLogoDimension          = 2048
)

// ValidateLogoHeader checks a logo's format and dimensions from the header alone; it never
// allocates a pixel buffer.
func ValidateLogoHeader(logo []byte) error {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(logo))
	if errors.Is(err, image.ErrFormat) {
		return errors.New("invalid file type provided. Expected png or jpeg.")
	}
	if err != nil {
		return errors.New("invalid or corrupt image")
	}

	if !strings.Contains(fmt.Sprintf("%s %s", PNGLogoType, JPEGLogoType), format) {
		return errors.New("invalid file type provided. Expected png or jpeg.")
	}

	if cfg.Width > MaxLogoDimension || cfg.Height > MaxLogoDimension {
		return fmt.Errorf("image dimensions %dx%d exceed the %dpx per-side limit", cfg.Width, cfg.Height, MaxLogoDimension)
	}

	return nil
}

// ValidateLogo runs ValidateLogoHeader, then fully decodes the now-bounded image to confirm
// the pixel payload is intact.
func ValidateLogo(logo []byte) error {
	if err := ValidateLogoHeader(logo); err != nil {
		return err
	}

	if _, _, err := image.Decode(bytes.NewReader(logo)); err != nil {
		return errors.New("invalid or corrupt image")
	}

	return nil
}
