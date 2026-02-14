package utils

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"os"

	"github.com/nfnt/resize"
)

// CompressImage resizes and compresses an image for sending
func CompressImage(path string, maxWidth, maxHeight uint) ([]byte, string, int, int, error) {
	// #nosec G304
	file, err := os.Open(path)
	if err != nil {
		return nil, "", 0, 0, err
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return nil, "", 0, 0, fmt.Errorf("failed to decode image: %w", err)
	}

	// Calculate new dimensions
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	if (width > 0 && uint(width) > maxWidth) || (height > 0 && uint(height) > maxHeight) {
		img = resize.Thumbnail(maxWidth, maxHeight, img, resize.Lanczos3)
		width = img.Bounds().Dx()
		height = img.Bounds().Dy()
	}

	var buf bytes.Buffer
	mimeType := "image/jpeg"

	// Re-encode to JPEG with quality 80
	err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80})
	if err != nil {
		return nil, "", 0, 0, err
	}

	return buf.Bytes(), mimeType, width, height, nil
}

// GetImageDimensions returns width and height of an image file
func GetImageDimensions(path string) (int, int, error) {
	// #nosec G304
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()

	cfg, _, err := image.DecodeConfig(file)
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}
