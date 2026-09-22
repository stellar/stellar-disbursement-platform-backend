package utils

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"strings"

	// Register the PNG and JPEG decoders that image.DecodeConfig relies on.
	_ "image/jpeg"
	_ "image/png"
)

type LogoType string

const (
	PNGLogoType      LogoType = "png"
	JPEGLogoType     LogoType = "jpeg"
	MaxLogoDimension          = 2048
)

// ValidateLogo checks a logo's header without decoding pixels: format, then dimensions,
// so a decompression bomb is rejected before anything allocates a pixel buffer.
func ValidateLogo(logo []byte) error {
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
