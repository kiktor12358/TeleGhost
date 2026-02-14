package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gen2brain/beeep"
	"github.com/nfnt/resize"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.design/x/clipboard"

	"teleghost/internal/appcore"
	"teleghost/internal/network/media"
)

// NetworkStatus статус подключения к I2P
type NetworkStatus string

const (
	NetworkStatusOffline    NetworkStatus = "offline"
	NetworkStatusConnecting NetworkStatus = "connecting"
	NetworkStatusOnline     NetworkStatus = "online"
	NetworkStatusError      NetworkStatus = "error"
)

// FolderInfo информация о папке
type FolderInfo struct {
	ID          string
	Name        string
	Icon        string
	ChatIDs     []string
	Position    int
	UnreadCount int
}

// ContactInfo информация о контакте для фронтенда
type ContactInfo struct {
	ID              string
	Nickname        string
	PublicKey       string
	Avatar          string
	I2PAddress      string
	LastMessage     string
	LastMessageTime int64
	LastSeen        string
	IsOnline        bool
	ChatID          string
	UnreadCount     int
}

// MessageInfo сообщение для фронтенда
type MessageInfo struct {
	ID           string
	Content      string
	Timestamp    int64
	IsOutgoing   bool
	Status       string
	ContentType  string
	FileCount    int
	TotalSize    int64
	Filenames    []string
	Attachments  []map[string]interface{}
	ReplyToID    string
	ReplyPreview *appcore.ReplyPreview
}

// UserInfo информация о текущем пользователе
type UserInfo struct {
	ID          string
	Nickname    string
	Avatar      string
	PublicKey   string
	Destination string
	Fingerprint string
	Mnemonic    string
}

// AppAboutInfo информация о приложении
type AppAboutInfo struct {
	AppVersion string `json:"app_version"`
	I2PVersion string `json:"i2p_version"`
	I2PPath    string `json:"i2p_path"`
	Author     string `json:"author"`
	License    string `json:"license"`
}

// RouterSettings настройки роутера
type RouterSettings struct {
	TunnelLength int // 1, 3, 5
	LogToFile    bool
}

// App основная структура приложения
type App struct {
	ctx  context.Context
	core *appcore.AppCore

	embeddedRouter interface {
		IsReady() bool
		Start(context.Context) error
		Stop() error
	}
	embeddedStop func() error
	trayManager  *TrayManager
	fileSelector FileSelector
}

// WailsEmitter — реализация appcore.EventEmitter через Wails runtime
type WailsEmitter struct {
	ctx context.Context
}

func (e *WailsEmitter) Emit(event string, data ...interface{}) {
	wailsRuntime.EventsEmit(e.ctx, event, data...)
}

// WailsPlatform — реализация appcore.PlatformServices через Wails runtime
type WailsPlatform struct {
	ctx context.Context
}

func (p *WailsPlatform) OpenFileDialog(title string, filters []string) (string, error) {
	return wailsRuntime.OpenFileDialog(p.ctx, wailsRuntime.OpenDialogOptions{
		Title: title,
	})
}

func (p *WailsPlatform) SaveFileDialog(title, defaultFilename string) (string, error) {
	return wailsRuntime.SaveFileDialog(p.ctx, wailsRuntime.SaveDialogOptions{
		Title:           title,
		DefaultFilename: defaultFilename,
	})
}

func (p *WailsPlatform) ClipboardSet(text string) {
	_ = wailsRuntime.ClipboardSetText(p.ctx, text)
}

func (p *WailsPlatform) ClipboardGet() (string, error) {
	return wailsRuntime.ClipboardGetText(p.ctx)
}

func (p *WailsPlatform) ShowWindow() {
	wailsRuntime.WindowShow(p.ctx)
}

func (p *WailsPlatform) HideWindow() {
	wailsRuntime.WindowHide(p.ctx)
}

func (p *WailsPlatform) Notify(title, message string) {
	// Десктопная реализация Notify через beeep
	appIcon := "" // Можно добавить путь к иконке
	if err := beeep.Notify(title, message, appIcon); err != nil {
		log.Printf("[App] Failed to send notification: %v", err)
	}
}

func (p *WailsPlatform) ShareFile(path string) error {
	// For desktop, just open the folder containing the file
	dir := filepath.Dir(path)
	return openFile(dir)
}

// FileSelector interface for platform-specific file selection
type FileSelector interface {
	SelectImage() (string, error)
	SelectFiles() ([]string, error)
}

// ClipboardSet sets text to clipboard
func (a *App) ClipboardSet(text string) {
	_ = wailsRuntime.ClipboardSetText(a.ctx, text)
}

// SetFileSelector sets the file selector implementation
func (a *App) SetFileSelector(selector FileSelector) {
	a.fileSelector = selector
}

// ClipboardGet gets text from clipboard
func (a *App) ClipboardGet() (string, error) {
	return wailsRuntime.ClipboardGetText(a.ctx)
}

// NewApp creates a new App application struct
func NewApp(iconData []byte) *App {
	app := &App{}
	app.trayManager = NewTrayManager(app, iconData)
	return app
}

// startup вызывается при старте приложения
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	if err := clipboard.Init(); err != nil {
		log.Printf("[App] Failed to init clipboard: %v", err)
	}

	appDataDir, err := os.UserConfigDir()
	if err != nil {
		appDataDir = "."
	}
	dataDir := filepath.Join(appDataDir, "TeleGhost")

	emitter := &WailsEmitter{ctx: ctx}
	platform := &WailsPlatform{ctx: ctx}

	a.core = appcore.NewAppCore(dataDir, emitter, platform)
	if err := a.core.Init(); err != nil {
		log.Printf("[App] Failed to init app core: %v", err)
	}

	if err := a.initEmbeddedRouter(ctx); err != nil {
		log.Printf("[App] Failed to init embedded router: %v", err)
	}

	if a.trayManager != nil {
		a.trayManager.Start()
	}

	log.Printf("[App] Started. Data dir: %s", dataDir)
}

// shutdown вызывается при закрытии приложения
func (a *App) shutdown(ctx context.Context) {
	log.Printf("[App] Shutdown started...")

	// Failsafe: force exit if shutdown takes too long
	go func() {
		time.Sleep(1 * time.Second)
		log.Println("[App] Shutdown timed out, forcing exit!")
		os.Exit(0)
	}()

	if a.core != nil {
		log.Println("[App] Shutting down Core...")
		a.core.Shutdown()
	}
	if a.embeddedStop != nil {
		log.Println("[App] Stopping embedded I2P router...")
		_ = a.embeddedStop()
	}
	if a.trayManager != nil {
		log.Println("[App] Stopping Tray...")
		a.trayManager.Stop()
	}
	log.Println("[App] Shutdown complete. Exiting.")
	os.Exit(0)
}

// GetMediaHandler возвращает обработчик для медиафайлов
func (a *App) GetMediaHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.core == nil {
			http.Error(w, "Core not initialized", http.StatusInternalServerError)
			return
		}

		if strings.HasPrefix(r.URL.Path, "/avatars/") {
			// Формат: /avatars/<userID>/<filename>
			parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/avatars/"), "/")
			if len(parts) >= 2 {
				userID := parts[0]
				filename := parts[1]
				avatarPath := filepath.Join(a.core.DataDir, "users", userID, "avatars", filename)
				http.ServeFile(w, r, avatarPath)
				return
			}
		}

		if a.core.Identity == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// MediaCrypt инициализируется в AppCore при логине
		mediaDir := filepath.Join(a.core.DataDir, "users", a.core.Identity.Keys.UserID, "media")
		mc, _ := media.NewMediaCrypt(a.core.Identity.Keys.EncryptionKey)
		handler := mc.NewMediaHandler(mediaDir)
		handler.ServeHTTP(w, r)
	})
}

// ShowWindow показывает окно из трея
func (a *App) ShowWindow() {
	wailsRuntime.WindowShow(a.ctx)
}

// QuitApp закрывает приложение
func (a *App) QuitApp() {
	wailsRuntime.Quit(a.ctx)
}

// SelectImage открывает диалог выбора изображения
func (a *App) SelectImage() (string, error) {
	if a.fileSelector != nil {
		return a.fileSelector.SelectImage()
	}
	return wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Выберите изображение",
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "Изображения (*.png;*.jpg;*.jpeg;*.webp)", Pattern: "*.png;*.jpg;*.jpeg;*.webp"},
		},
	})
}

// SelectFiles открывает диалог выбора файлов
func (a *App) SelectFiles() ([]string, error) {
	if a.fileSelector != nil {
		return a.fileSelector.SelectFiles()
	}
	return wailsRuntime.OpenMultipleFilesDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Выберите файлы",
	})
}

// GetImageThumbnail возвращает уменьшенную копию изображения в base64
func (a *App) GetImageThumbnail(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return "", err
	}

	// Изменяем размер до 256px по одной из сторон
	thumb := resize.Thumbnail(256, 256, img, resize.Lanczos3)

	var buf bytes.Buffer
	if err := png.Encode(&buf, thumb); err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// SaveTempImage сохраняет изображение во временную папку для превью
func (a *App) SaveTempImage(base64Data, filename string) (string, error) {
	// Убираем префикс если есть
	if idx := strings.Index(base64Data, ","); idx != -1 {
		base64Data = base64Data[idx+1:]
	}

	data, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return "", err
	}

	tempDir := filepath.Join(a.core.DataDir, "temp")
	os.MkdirAll(tempDir, 0755)

	filePath := filepath.Join(tempDir, filename)
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return "", err
	}

	return filePath, nil
}

// OpenFile открывает файл системным приложением
func (a *App) OpenFile(path string) error {
	return openFile(path)
}

// ShowInFolder открывает папку с файлом
func (a *App) ShowInFolder(path string) error {
	return openFile(filepath.Dir(path))
}

// SaveFileToLocation сохраняет файл в выбранное место
func (a *App) SaveFileToLocation(path, filename string) (string, error) {
	dest, err := wailsRuntime.SaveFileDialog(a.ctx, wailsRuntime.SaveDialogOptions{
		Title:           "Сохранить файл",
		DefaultFilename: filename,
	})
	if err != nil || dest == "" {
		return "", err
	}

	// Копируем файл
	input, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(dest, input, 0644); err != nil {
		return "", err
	}

	return dest, nil
}

// SetActiveChat устанавливает ID активного чата
func (a *App) SetActiveChat(chatID string) {
	if a.core != nil {
		a.core.SetActiveChat(chatID)
	}
}

// SetAppFocus устанавливает фокус приложения
func (a *App) SetAppFocus(focused bool) {
	if a.core != nil {
		a.core.IsFocused = focused
	}
}

// RequestProfile запрашивает обновление профиля у контакта
func (a *App) RequestProfile(address string) error {
	if a.core != nil {
		return a.core.RequestProfile(address)
	}
	return fmt.Errorf("core not initialized")
}

// ExportReseed wraps AppCore.ExportReseed
func (a *App) ExportReseed() (string, error) {
	if a.core == nil {
		return "", fmt.Errorf("core not initialized")
	}
	// 1. Создание временного архива
	tempPath, err := a.core.ExportReseed()
	if err != nil {
		return "", err
	}

	// 2. Предлагаем пользователю сохранить файл
	defaultName := filepath.Base(tempPath)
	destPath, err := wailsRuntime.SaveFileDialog(a.ctx, wailsRuntime.SaveDialogOptions{
		Title:           "Сохранить файл сети",
		DefaultFilename: defaultName,
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "ZIP Archive (*.zip)", Pattern: "*.zip"},
		},
	})

	if err != nil {
		// Ошибка диалога
		return "", err
	}

	if destPath == "" {
		// Пользователь отменил сохранение
		return "", fmt.Errorf("export canceled")
	}

	// 3. Перемещаем файл из temp в выбранное место
	// Rename может не сработать между дисками, поэтому лучше Read/Write
	input, err := os.ReadFile(tempPath)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(destPath, input, 0644); err != nil {
		return "", err
	}
	os.Remove(tempPath) // Удаляем временный файл

	// 4. Открываем папку с сохраненным файлом
	a.ShareFile(destPath)

	return destPath, nil
}

// ImportReseed wraps AppCore.ImportReseed
func (a *App) ImportReseed(path string) error {
	if a.core == nil {
		return fmt.Errorf("core not initialized")
	}
	return a.core.ImportReseed(path)
}

// ExportAccount wraps AppCore.ExportAccount
func (a *App) ExportAccount() (string, error) {
	if a.core == nil {
		return "", fmt.Errorf("core not initialized")
	}

	tempPath, err := a.core.ExportAccount()
	if err != nil {
		return "", err
	}

	defaultName := filepath.Base(tempPath)
	destPath, err := wailsRuntime.SaveFileDialog(a.ctx, wailsRuntime.SaveDialogOptions{
		Title:           "Сохранить резервную копию аккаунта",
		DefaultFilename: defaultName,
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "ZIP Archive (*.zip)", Pattern: "*.zip"},
		},
	})

	if err != nil {
		return "", err
	}

	if destPath == "" {
		return "", fmt.Errorf("export canceled")
	}

	input, err := os.ReadFile(tempPath)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(destPath, input, 0644); err != nil {
		return "", err
	}
	os.Remove(tempPath)

	a.ShareFile(destPath)

	return destPath, nil
}

// ImportAccount wraps AppCore.ImportAccount
func (a *App) ImportAccount(path string) error {
	if a.core == nil {
		return fmt.Errorf("core not initialized")
	}
	return a.core.ImportAccount(path)
}

// ShareFile calls WailsPlatform.ShareFile
func (a *App) ShareFile(path string) error {
	// For desktop, just open the folder containing the file
	dir := filepath.Dir(path)
	return openFile(dir)
}

// openFile opens a file or directory using the system default application.
// This avoids Wails "Invalid URL scheme" error for file:// URLs.
func openFile(path string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "explorer"
		args = []string{path}
	case "darwin":
		cmd = "open"
		args = []string{path}
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
		args = []string{path}
	}
	return exec.Command(cmd, args...).Start()
}
