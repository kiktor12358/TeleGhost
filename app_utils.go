package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bytes"
	"image"
	"image/png"

	"github.com/nfnt/resize"
	"github.com/skratchdot/open-golang/open"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.design/x/clipboard"
)

// GetFileBase64 читает файл и возвращает base64 строку
func (a *App) GetFileBase64(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// SaveTempImage сохраняет base64 изображение во временный файл
func (a *App) SaveTempImage(base64Data string, name string) (string, error) {
	// Remove data URI prefix if present
	parts := strings.Split(base64Data, ",")
	if len(parts) > 1 {
		base64Data = parts[1]
	}

	data, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	// Create temp directory if not exists
	tempDir := filepath.Join(os.TempDir(), "teleghost_uploads")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	// Create unique filename
	filename := fmt.Sprintf("%d_%s", time.Now().UnixNano(), name)
	path := filepath.Join(tempDir, filename)

	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return path, nil
}

// SelectFiles открывает диалог выбора файлов
func (a *App) SelectFiles() ([]string, error) {
	return runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Выберите файлы",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "All Files",
				Pattern:     "*",
			},
			{
				DisplayName: "Images",
				Pattern:     "*.jpg;*.jpeg;*.png;*.webp;*.gif;*.bmp",
			},
		},
	})
}

// OpenFile opens a file using the system's default application
func (a *App) OpenFile(path string) error {
	return open.Run(path)
}

// ShowInFolder opens the file manager showing the file
func (a *App) ShowInFolder(path string) error {
	return open.Run(filepath.Dir(path))
}

// CopyToClipboard копирует текст в буфер обмена
func (a *App) CopyToClipboard(text string) {
	runtime.ClipboardSetText(a.ctx, text)
}

// CopyImageToClipboard copies image from path to clipboard
func (a *App) CopyImageToClipboard(path string) error {
	// Read file
	fileData, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Decode to image.Image to check format and convert if needed
	img, _, err := image.Decode(bytes.NewReader(fileData))
	if err != nil {
		return fmt.Errorf("failed to decode image: %w", err)
	}

	// Encode to PNG buffer (clipboard.FmtImage usually expects PNG)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return fmt.Errorf("failed to encode to png: %w", err)
	}

	// Write to clipboard
	clipboard.Write(clipboard.FmtImage, buf.Bytes())
	return nil
}

// GetImageThumbnail создает уменьшенную копию изображения и возвращает base64
func (a *App) GetImageThumbnail(path string, width, height uint) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return "", err
	}

	// Bilinear resampling for speed
	m := resize.Thumbnail(width, height, img, resize.Bilinear)

	var buf bytes.Buffer
	err = png.Encode(&buf, m)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
